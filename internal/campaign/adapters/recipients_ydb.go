package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// ydbRecipientChunk is how many recipients one transaction writes or one
// query reads. V5 found no documented transaction-size limit, and a query
// returns at most 1000 rows.
const ydbRecipientChunk = 1000

// YDBRecipients is the YDB-backed implementation of domain.RecipientRepository,
// for the serverless runtime. A campaign's recipients are one primary-key
// range, in id order. A recipient is found from its id alone through
// idx_campaign_recipients_id.
type YDBRecipients struct {
	driver *ydb.Driver
}

var _ domain.RecipientRepository = (*YDBRecipients)(nil)

// NewYDBRecipients builds a YDBRecipients repository over the driver.
func NewYDBRecipients(d *ydb.Driver) *YDBRecipients {
	return &YDBRecipients{driver: d}
}

// BulkInsert persists one recipient row per unique (campaign_id, email) and
// returns how many rows were newly inserted. An INSERT that hits an existing
// key aborts the whole transaction on YDB (spec "Plan B7 planning spike"), so
// each chunk of up to 1000 recipients reads the emails the campaign already
// has and inserts only the others, in one transaction. A chunk is idempotent
// on retry, so the SDK may re-run it. A first attempt that committed without
// the response arriving makes the retry count its rows as already present, so
// the returned count can then fall short.
func (r *YDBRecipients) BulkInsert(ctx context.Context, tenantID, campaignID string,
	rs []*domain.Recipient) (int, error) {

	// The first occurrence of an email wins, as ON CONFLICT DO NOTHING keeps
	// the first row. YDB rejects a batch that repeats a key.
	seen := make(map[string]bool, len(rs))
	unique := make([]*domain.Recipient, 0, len(rs))
	for _, rec := range rs {
		if !seen[rec.Email()] {
			seen[rec.Email()] = true
			unique = append(unique, rec)
		}
	}

	inserted := 0
	for start := 0; start < len(unique); start += ydbRecipientChunk {
		chunk := unique[start:min(start+ydbRecipientChunk, len(unique))]
		n, err := r.insertChunk(ctx, tenantID, campaignID, chunk)
		if err != nil {
			return inserted, err
		}
		inserted += n
	}
	return inserted, nil
}

// insertChunk inserts the recipients of chunk whose email the campaign does
// not have yet, and returns how many it inserted.
func (r *YDBRecipients) insertChunk(ctx context.Context, tenantID, campaignID string,
	chunk []*domain.Recipient) (int, error) {

	// Ids are fixed before the transaction, so a retry writes the same rows.
	ids := make([]string, len(chunk))
	emails := make([]types.Value, len(chunk))
	for i, rec := range chunk {
		ids[i] = uuid.NewString()
		emails[i] = types.TextValue(rec.Email())
	}
	idOpt := query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$campaign_id").Text(campaignID).
		Param("$emails").Any(types.ListValue(emails...)).
		Build())

	var inserted int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $emails AS List<Utf8>;
SELECT email FROM campaign_recipients VIEW idx_campaign_recipients_email
WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id AND email IN $emails;`, idOpt)
		if err != nil {
			return fmt.Errorf("reading existing recipients: %w", err)
		}
		existing := map[string]bool{}
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("reading existing recipients: %w", err)
			}
			var email string
			if err := row.Scan(&email); err != nil {
				return fmt.Errorf("scanning existing recipient: %w", err)
			}
			existing[email] = true
		}

		var rows []types.Value
		for i, rec := range chunk {
			if existing[rec.Email()] {
				continue
			}
			rows = append(rows, types.StructValue(
				types.StructFieldValue("id", types.TextValue(ids[i])),
				types.StructFieldValue("subscriber_id", types.TextValue(rec.SubscriberID())),
				types.StructFieldValue("email", types.TextValue(rec.Email()))))
		}
		if len(rows) > 0 {
			err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $rows AS List<Struct<id: Utf8, subscriber_id: Utf8, email: Utf8>>;
INSERT INTO campaign_recipients
    (tenant_id, campaign_id, id, subscriber_id, email, status, failure_reason)
SELECT $tenant_id, $campaign_id, id, subscriber_id, email, "pending"u, ""u
FROM AS_TABLE($rows);`, query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Param("$rows").Any(types.ListValue(rows...)).
				Build()))
			if err != nil {
				return fmt.Errorf("inserting recipients: %w", err)
			}
		}
		// Assigned at the end: a retry must not add to a failed run's count.
		inserted = len(rows)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

// Pending returns the still-unsent recipients within the stable [offset,
// offset+limit) window of the campaign's id-ordered recipient set, as the
// Postgres adapter does: the window covers every recipient, so a batch's slice
// never shifts as rows are sent.
// ponytail: OFFSET reads every skipped row, so batch N of a campaign reads N
// windows of rows. Fine for the recipient counts of one organisation; store a
// per-campaign ordinal to seek on if a campaign outgrows it (spec "Ceilings").
func (r *YDBRecipients) Pending(ctx context.Context, tenantID, campaignID string,
	offset, limit int) ([]*domain.Recipient, error) {

	var out []*domain.Recipient
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		// Collected into a local and assigned at the end (V6).
		var local []*domain.Recipient
		for done := 0; done < limit; done += ydbRecipientChunk {
			rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT id, subscriber_id, email, status, failure_reason, sent_at
FROM campaign_recipients
WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id
ORDER BY id
LIMIT $limit OFFSET $offset;`, query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Param("$limit").Uint64(uint64(min(ydbRecipientChunk, limit-done))).
				Param("$offset").Uint64(uint64(offset+done)).
				Build()))
			if err != nil {
				return fmt.Errorf("listing pending recipients: %w", err)
			}
			read := 0
			for row, err := range rs.Rows(ctx) {
				if err != nil {
					return fmt.Errorf("listing pending recipients: %w", err)
				}
				read++
				var id, subscriberID, email, status, failureReason string
				var sentAt *time.Time
				if err := row.Scan(&id, &subscriberID, &email, &status, &failureReason, &sentAt); err != nil {
					return fmt.Errorf("scanning recipient: %w", err)
				}
				if domain.RecipientStatus(status) != domain.RecipientPending {
					continue
				}
				local = append(local, domain.HydrateRecipient(id, tenantID, campaignID, subscriberID,
					email, domain.RecipientPending, failureReason, sentAt))
			}
			if read < min(ydbRecipientChunk, limit-done) {
				break // the campaign has no rows past this window
			}
		}
		out = local
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MarkSent records a successful send for one recipient, persisting the
// provider message id so a later bounce or complaint can be attributed to it.
// A recipient id that does not exist records nothing, as the Postgres UPDATE
// matches no row.
func (r *YDBRecipients) MarkSent(ctx context.Context, tenantID, recipientID, providerMessageID string,
	at time.Time) error {

	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $sent_at AS Timestamp;
DECLARE $provider_message_id AS Optional<Utf8>;
UPDATE campaign_recipients ON
SELECT tenant_id, campaign_id, id, "sent"u AS status, ""u AS failure_reason,
    $sent_at AS sent_at, $provider_message_id AS provider_message_id
FROM campaign_recipients VIEW idx_campaign_recipients_id
WHERE tenant_id = $tenant_id AND id = $id;`, query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(recipientID).
		Param("$sent_at").Timestamp(at.UTC()).
		Param("$provider_message_id").BeginOptional().Text(nullableString(providerMessageID)).EndOptional().
		Build()))
	if err != nil {
		return fmt.Errorf("marking recipient sent: %w", err)
	}
	return nil
}

// MarkSkipped records a recipient skipped by the pre-send suppression check,
// storing the suppression reason in failure_reason.
func (r *YDBRecipients) MarkSkipped(ctx context.Context, tenantID, recipientID, reason string) error {
	return r.markStatus(ctx, tenantID, recipientID, domain.RecipientSkipped, reason)
}

// MarkFailed records a failed send for one recipient.
func (r *YDBRecipients) MarkFailed(ctx context.Context, tenantID, recipientID, reason string) error {
	return r.markStatus(ctx, tenantID, recipientID, domain.RecipientFailed, reason)
}

func (r *YDBRecipients) markStatus(ctx context.Context, tenantID, recipientID string,
	status domain.RecipientStatus, reason string) error {

	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $status AS Utf8;
DECLARE $failure_reason AS Utf8;
UPDATE campaign_recipients ON
SELECT tenant_id, campaign_id, id, $status AS status, $failure_reason AS failure_reason
FROM campaign_recipients VIEW idx_campaign_recipients_id
WHERE tenant_id = $tenant_id AND id = $id;`, query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(recipientID).
		Param("$status").Text(string(status)).
		Param("$failure_reason").Text(reason).
		Build()))
	if err != nil {
		return fmt.Errorf("marking recipient %s: %w", status, err)
	}
	return nil
}

// Counts returns the campaign's sent, failed, and pending recipient counts.
// ponytail: it reads every recipient row of the campaign on each call.
func (r *YDBRecipients) Counts(ctx context.Context, tenantID, campaignID string) (sent, failed, pending int, err error) {
	err = r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
SELECT status, COUNT(*) FROM campaign_recipients
WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id
GROUP BY status;`, query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$campaign_id").Text(campaignID).
			Build()))
		if err != nil {
			return fmt.Errorf("counting recipients: %w", err)
		}
		// Assigned at the end of a complete read: a retry starts from zero.
		var s, f, p int
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("counting recipients: %w", err)
			}
			var status string
			var n uint64
			if err := row.Scan(&status, &n); err != nil {
				return fmt.Errorf("scanning recipient count: %w", err)
			}
			switch domain.RecipientStatus(status) {
			case domain.RecipientSent:
				s = int(n)
			case domain.RecipientFailed:
				f = int(n)
			case domain.RecipientPending:
				p = int(n)
			}
		}
		sent, failed, pending = s, f, p
		return nil
	})
	return sent, failed, pending, err
}

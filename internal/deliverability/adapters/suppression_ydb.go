package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"

	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// YDBSuppressions is the YDB-backed implementation of
// domain.SuppressionRepository, for the serverless runtime. The address is
// unique per tenant through idx_suppression_email.
type YDBSuppressions struct {
	driver *ydb.Driver
}

var _ domain.SuppressionRepository = (*YDBSuppressions)(nil)

// NewYDBSuppressions builds a YDBSuppressions repository over the driver.
func NewYDBSuppressions(d *ydb.Driver) *YDBSuppressions {
	return &YDBSuppressions{driver: d}
}

// ydbSuppressionIDByEmail reads the id of the tenant's entry for an address.
func ydbSuppressionIDByEmail(ctx context.Context, tx query.TxActor, tenantID, email string) (string, bool, error) {
	row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $email AS Utf8;
SELECT id FROM suppression_list VIEW idx_suppression_email
WHERE tenant_id = $tenant_id AND email = $email;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$email").Text(email).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading suppression entry: %w", err)
	}
	var id string
	if err := row.Scan(&id); err != nil {
		return "", false, fmt.Errorf("scanning suppression entry: %w", err)
	}
	return id, true, nil
}

// Upsert adds a suppression entry; an address already suppressed for the
// tenant is a no-op. The transaction reads the address first, because a
// conflicting INSERT would abort it.
func (r *YDBSuppressions) Upsert(ctx context.Context, e *domain.SuppressionEntry) error {
	id := uuid.NewString()
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if _, found, err := ydbSuppressionIDByEmail(ctx, tx, e.TenantID(), e.Email()); err != nil || found {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $reason AS Utf8;
DECLARE $source_event_id AS Optional<Utf8>;
DECLARE $note AS Utf8;
INSERT INTO suppression_list (tenant_id, id, email, reason, source_event_id, suppressed_at, note)
VALUES ($tenant_id, $id, $email, $reason, $source_event_id, CurrentUtcTimestamp(), $note);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(e.TenantID()).
				Param("$id").Text(id).
				Param("$email").Text(e.Email()).
				Param("$reason").Text(string(e.Reason())).
				Param("$source_event_id").BeginOptional().Text(nullString(e.SourceEventID())).EndOptional().
				Param("$note").Text(e.Note()).
				Build())); err != nil {
			return fmt.Errorf("upserting suppression entry: %w", err)
		}
		return nil
	})
}

// Remove deletes a suppression entry, returning ErrSuppressionNotFound when no
// entry matches the address.
func (r *YDBSuppressions) Remove(ctx context.Context, tenantID, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		id, found, err := ydbSuppressionIDByEmail(ctx, tx, tenantID, email)
		if err != nil {
			return err
		}
		if !found {
			return domain.ErrSuppressionNotFound
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM suppression_list WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Build())); err != nil {
			return fmt.Errorf("removing suppression entry: %w", err)
		}
		return nil
	})
}

// List returns a page of the tenant's suppression entries, ordered by id, and
// the cursor for the next page (empty when the page is the last).
func (r *YDBSuppressions) List(ctx context.Context, tenantID string, f domain.SuppressionFilter) (
	[]*domain.SuppressionEntry, string, error) {

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	decl := "DECLARE $tenant_id AS Utf8;\nDECLARE $limit AS Uint64;\n"
	where := ""
	params := ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$limit").Uint64(uint64(limit + 1))
	if f.Reason != "" {
		decl += "DECLARE $reason AS Utf8;\n"
		where += " AND reason = $reason"
		params = params.Param("$reason").Text(string(f.Reason))
	}
	if f.EmailLike != "" {
		decl += "DECLARE $email_like AS Utf8;\n"
		where += ` AND email ILIKE "%"u || $email_like || "%"u`
		params = params.Param("$email_like").Text(f.EmailLike)
	}
	if f.Cursor != "" {
		decl += "DECLARE $cursor AS Utf8;\n"
		where += " AND id > $cursor"
		params = params.Param("$cursor").Text(f.Cursor)
	}
	rs, err := r.driver.Query().QueryResultSet(ctx, decl+
		`SELECT id, tenant_id, email, reason, source_event_id, suppressed_at, note
FROM suppression_list
WHERE tenant_id = $tenant_id`+where+`
ORDER BY id LIMIT $limit;`, query.WithParameters(params.Build()))
	if err != nil {
		return nil, "", fmt.Errorf("listing suppression entries: %w", err)
	}
	var out []*domain.SuppressionEntry
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, "", fmt.Errorf("listing suppression entries: %w", err)
		}
		var id, tID, email, reason, note string
		var sourceEventID *string
		var suppressedAt time.Time
		if err := row.Scan(&id, &tID, &email, &reason, &sourceEventID, &suppressedAt, &note); err != nil {
			return nil, "", fmt.Errorf("scanning suppression entry: %w", err)
		}
		out = append(out, domain.HydrateSuppressionEntry(id, tID, email,
			domain.SuppressionReason(reason), deref(sourceEventID), suppressedAt, note))
	}

	nextCursor := ""
	if len(out) > limit {
		nextCursor = out[limit-1].ID()
		out = out[:limit]
	}
	return out, nextCursor, nil
}

// YDBSuppressionChecker is the YDB-backed pre-send gate consumed by the
// campaign send paths. It implements the campaign context's SuppressionChecker
// port over idx_suppression_email.
type YDBSuppressionChecker struct {
	driver *ydb.Driver
}

var _ campaigndomain.SuppressionChecker = (*YDBSuppressionChecker)(nil)

// NewYDBSuppressionChecker builds a YDBSuppressionChecker over the driver.
func NewYDBSuppressionChecker(d *ydb.Driver) *YDBSuppressionChecker {
	return &YDBSuppressionChecker{driver: d}
}

// Suppressed returns the subset of emails on the tenant's suppression list,
// mapped to the reason each was suppressed.
func (c *YDBSuppressionChecker) Suppressed(ctx context.Context, tenantID string,
	emails []string) (map[string]string, error) {

	if len(emails) == 0 {
		return map[string]string{}, nil
	}
	values := make([]types.Value, len(emails))
	for i, e := range emails {
		values[i] = types.TextValue(strings.ToLower(strings.TrimSpace(e)))
	}
	rs, err := c.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $emails AS List<Utf8>;
SELECT email, reason FROM suppression_list VIEW idx_suppression_email
WHERE tenant_id = $tenant_id AND email IN $emails;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$emails").Any(types.ListValue(values...)).
			Build()))
	if err != nil {
		return nil, fmt.Errorf("checking suppression list: %w", err)
	}
	out := map[string]string{}
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("checking suppression list: %w", err)
		}
		var email, reason string
		if err := row.Scan(&email, &reason); err != nil {
			return nil, fmt.Errorf("scanning suppression entry: %w", err)
		}
		out[email] = reason
	}
	return out, nil
}

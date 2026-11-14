package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// YDBSendingDomains is the YDB-backed implementation of
// domain.SendingDomainRepository, for the serverless runtime. YDB has no
// row-level security, so every statement filters by tenant_id explicitly; the
// primary key (tenant_id, id) makes that filter a key-prefix lookup.
type YDBSendingDomains struct {
	driver *ydb.Driver
}

var _ domain.SendingDomainRepository = (*YDBSendingDomains)(nil)

// NewYDBSendingDomains builds a YDBSendingDomains repository over the driver.
func NewYDBSendingDomains(d *ydb.Driver) *YDBSendingDomains {
	return &YDBSendingDomains{driver: d}
}

const ydbSendingDomainColumns = `id, tenant_id, domain, status, dkim_records, spf_record,
    dmarc_record, postbox_identity_ref, failure_reason, created_at, verified_at, last_checked_at`

const ydbSelectSendingDomain = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbSendingDomainColumns + `
FROM sending_domains
WHERE tenant_id = $tenant_id AND id = $id;`

// Add persists a new sending domain under a fresh UUID and returns the id. It
// returns domain.ErrDomainAlreadyExists when the tenant already has the domain.
func (r *YDBSendingDomains) Add(ctx context.Context, tenantID string, d *domain.SendingDomain) (string, error) {
	dkimJSON, err := json.Marshal(d.DKIMRecords())
	if err != nil {
		return "", fmt.Errorf("encoding dkim records: %w", err)
	}
	id := uuid.NewString()
	err = r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $domain AS Utf8;
DECLARE $status AS Utf8;
DECLARE $dkim_records AS Utf8;
DECLARE $spf_record AS Utf8;
DECLARE $dmarc_record AS Utf8;
DECLARE $postbox_identity_ref AS Utf8;
DECLARE $failure_reason AS Utf8;
INSERT INTO sending_domains
    (tenant_id, id, domain, status, dkim_records, spf_record, dmarc_record,
     postbox_identity_ref, failure_reason, created_at)
VALUES ($tenant_id, $id, $domain, $status, $dkim_records, $spf_record, $dmarc_record,
     $postbox_identity_ref, $failure_reason, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$domain").Text(d.Domain()).
			Param("$status").Text(string(d.Status())).
			Param("$dkim_records").Text(string(dkimJSON)).
			Param("$spf_record").Text(d.SPFRecord()).
			Param("$dmarc_record").Text(d.DMARCRecord()).
			Param("$postbox_identity_ref").Text(d.IdentityRef()).
			Param("$failure_reason").Text(d.FailureReason()).
			Build()))
	if db.IsYDBConflict(err) {
		return "", domain.ErrDomainAlreadyExists
	}
	if err != nil {
		return "", fmt.Errorf("inserting sending domain: %w", err)
	}
	return id, nil
}

// Get returns the domain, or domain.ErrDomainNotFound.
func (r *YDBSendingDomains) Get(ctx context.Context, tenantID, id string) (*domain.SendingDomain, error) {
	return ydbSendingDomainFromRow(r.driver.Query().QueryRow(ctx, ydbSelectSendingDomain, ydbTenantAndID(tenantID, id)))
}

// Update loads the domain, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so fn
// may run more than once and must be retry-safe.
func (r *YDBSendingDomains) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.SendingDomain) (*domain.SendingDomain, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbSendingDomainFromRow(tx.QueryRow(ctx, ydbSelectSendingDomain, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		dkimJSON, err := json.Marshal(updated.DKIMRecords())
		if err != nil {
			return fmt.Errorf("encoding dkim records: %w", err)
		}
		err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $status AS Utf8;
DECLARE $dkim_records AS Utf8;
DECLARE $spf_record AS Utf8;
DECLARE $dmarc_record AS Utf8;
DECLARE $postbox_identity_ref AS Utf8;
DECLARE $failure_reason AS Utf8;
DECLARE $verified_at AS Optional<Timestamp>;
DECLARE $last_checked_at AS Optional<Timestamp>;
UPDATE sending_domains SET
    status = $status, dkim_records = $dkim_records, spf_record = $spf_record,
    dmarc_record = $dmarc_record, postbox_identity_ref = $postbox_identity_ref,
    failure_reason = $failure_reason,
    verified_at = $verified_at, last_checked_at = $last_checked_at
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$status").Text(string(updated.Status())).
				Param("$dkim_records").Text(string(dkimJSON)).
				Param("$spf_record").Text(updated.SPFRecord()).
				Param("$dmarc_record").Text(updated.DMARCRecord()).
				Param("$postbox_identity_ref").Text(updated.IdentityRef()).
				Param("$failure_reason").Text(updated.FailureReason()).
				Param("$verified_at").BeginOptional().Timestamp(updated.VerifiedAt()).EndOptional().
				Param("$last_checked_at").BeginOptional().Timestamp(updated.LastCheckedAt()).EndOptional().
				Build()))
		if err != nil {
			return fmt.Errorf("updating sending domain: %w", err)
		}
		return nil
	})
}

// All returns every sending domain of the tenant, ordered by domain name.
func (r *YDBSendingDomains) All(ctx context.Context, tenantID string) ([]*domain.SendingDomain, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbSendingDomainColumns+`
FROM sending_domains
WHERE tenant_id = $tenant_id
ORDER BY domain;`, ydbTenant(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing sending domains: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var domains []*domain.SendingDomain
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing sending domains: %w", err)
		}
		d, err := scanYDBSendingDomain(row)
		if err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, nil
}

// PendingIDs lists the ids of the tenant's domains still awaiting
// verification.
func (r *YDBSendingDomains) PendingIDs(ctx context.Context, tenantID string) ([]string, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT id FROM sending_domains
WHERE tenant_id = $tenant_id AND status = "pending"u;`, ydbTenant(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing pending domains: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var ids []string
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing pending domains: %w", err)
		}
		var id string
		if err := row.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning pending domain: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func ydbTenant(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

func ydbTenantAndID(tenantID, id string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Build())
}

// ydbSendingDomainFromRow maps a single-row read to the domain, translating
// "no row" into domain.ErrDomainNotFound.
func ydbSendingDomainFromRow(row query.Row, err error) (*domain.SendingDomain, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading sending domain: %w", err)
	}
	return scanYDBSendingDomain(row)
}

// scanYDBSendingDomain reads one row in ydbSendingDomainColumns order.
func scanYDBSendingDomain(row query.Row) (*domain.SendingDomain, error) {
	var id, tenantID, name, status, dkimJSON, spf, dmarc, identityRef, failureReason string
	var createdAt time.Time
	var verifiedAt, lastCheckedAt *time.Time
	if err := row.Scan(&id, &tenantID, &name, &status, &dkimJSON, &spf,
		&dmarc, &identityRef, &failureReason, &createdAt, &verifiedAt, &lastCheckedAt); err != nil {
		return nil, fmt.Errorf("scanning sending domain: %w", err)
	}
	var dkim []domain.DNSRecord
	if err := json.Unmarshal([]byte(dkimJSON), &dkim); err != nil {
		return nil, fmt.Errorf("decoding dkim records: %w", err)
	}
	return domain.HydrateSendingDomain(id, tenantID, name, domain.Status(status), dkim,
		spf, dmarc, identityRef, failureReason, createdAt, verifiedAt, lastCheckedAt), nil
}

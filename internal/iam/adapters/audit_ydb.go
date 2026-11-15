package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// YDBAudit is the YDB-backed implementation of domain.AuditRepository, for the
// serverless runtime. The primary key (tenant_id, created_at, id) is the order
// All pages through.
type YDBAudit struct {
	driver *ydb.Driver
}

var _ domain.AuditRepository = (*YDBAudit)(nil)

// NewYDBAudit builds a YDBAudit repository over the driver.
func NewYDBAudit(d *ydb.Driver) *YDBAudit {
	return &YDBAudit{driver: d}
}

// Record appends one audit record under a fresh UUID. The database stamps
// created_at.
func (r *YDBAudit) Record(ctx context.Context, tenantID string, rec domain.AuditRecord) error {
	metadata, err := json.Marshal(rec.Metadata)
	if err != nil {
		return fmt.Errorf("encoding audit metadata: %w", err)
	}
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $actor_id AS Utf8;
DECLARE $actor_kind AS Utf8;
DECLARE $action AS Utf8;
DECLARE $target AS Utf8;
DECLARE $metadata AS Utf8;
INSERT INTO audit_log (tenant_id, created_at, id, actor_id, actor_kind, action, target, metadata)
VALUES ($tenant_id, CurrentUtcTimestamp(), $id, $actor_id, $actor_kind, $action, $target, $metadata);`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(uuid.NewString()).
			Param("$actor_id").Text(rec.ActorID).
			Param("$actor_kind").Text(string(rec.ActorKind)).
			Param("$action").Text(rec.Action).
			Param("$target").Text(rec.Target).
			Param("$metadata").Text(string(metadata)).
			Build())); err != nil {
		return fmt.Errorf("inserting audit record: %w", err)
	}
	return nil
}

// All returns a page of the tenant's audit records, newest first, and the
// tenant's total record count. The count and the page are read in one
// transaction, so they agree.
func (r *YDBAudit) All(ctx context.Context, tenantID string, page domain.Page) ([]domain.AuditRecord, int, error) {
	page = page.Normalize()
	var records []domain.AuditRecord
	var total int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM audit_log WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("counting audit records: %w", err)
		}
		var count uint64
		if err := row.Scan(&count); err != nil {
			return fmt.Errorf("scanning audit count: %w", err)
		}
		// ponytail: OFFSET paging reads every skipped row; keyset paging on
		// (created_at, id) if the audit trail grows large.
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT id, tenant_id, actor_id, actor_kind, action, target, metadata, created_at
FROM audit_log
WHERE tenant_id = $tenant_id
ORDER BY created_at DESC
LIMIT $limit OFFSET $offset;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$limit").Uint64(uint64(page.Limit)).
				Param("$offset").Uint64(uint64(page.Offset)).
				Build()))
		if err != nil {
			return fmt.Errorf("listing audit records: %w", err)
		}
		// Collected into a local and assigned at the end: the SDK may re-run
		// the closure, and a retry must not append to rows a failed run read.
		var local []domain.AuditRecord
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing audit records: %w", err)
			}
			var id, tID, actorID, actorKind, action, target, metadataJSON string
			var createdAt time.Time
			if err := row.Scan(&id, &tID, &actorID, &actorKind, &action, &target,
				&metadataJSON, &createdAt); err != nil {
				return fmt.Errorf("scanning audit record: %w", err)
			}
			metadata := map[string]any{}
			if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
				return fmt.Errorf("decoding audit metadata: %w", err)
			}
			local = append(local, domain.AuditRecord{
				ID: id, TenantID: tID, ActorID: actorID,
				ActorKind: domain.PrincipalKind(actorKind), Action: action,
				Target: target, Metadata: metadata, CreatedAt: createdAt,
			})
		}
		records, total = local, int(count)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

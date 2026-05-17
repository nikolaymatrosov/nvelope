package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Audit is the pgx-backed implementation of domain.AuditRepository.
type Audit struct {
	pool *pgxpool.Pool
}

var _ domain.AuditRepository = (*Audit)(nil)

// NewAudit builds an Audit repository over the given pool.
func NewAudit(pool *pgxpool.Pool) *Audit {
	return &Audit{pool: pool}
}

// Record appends one audit record.
func (r *Audit) Record(ctx context.Context, tenantID string, rec domain.AuditRecord) error {
	metadata, err := json.Marshal(rec.Metadata)
	if err != nil {
		return fmt.Errorf("encoding audit metadata: %w", err)
	}
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO audit_log (tenant_id, actor_id, actor_kind, action, target, metadata)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			tenantID, rec.ActorID, string(rec.ActorKind), rec.Action, rec.Target, metadata)
		if err != nil {
			return fmt.Errorf("inserting audit record: %w", err)
		}
		return nil
	})
}

// All returns a page of the tenant's audit records, newest first.
func (r *Audit) All(ctx context.Context, tenantID string, page domain.Page) ([]domain.AuditRecord, int, error) {
	page = page.Normalize()
	var records []domain.AuditRecord
	var total int
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM audit_log").Scan(&total); err != nil {
			return fmt.Errorf("counting audit records: %w", err)
		}
		rows, err := tx.Query(ctx,
			`SELECT id, tenant_id, actor_id, actor_kind, action, target, metadata, created_at
			 FROM audit_log ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
			page.Limit, page.Offset)
		if err != nil {
			return fmt.Errorf("listing audit records: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, tID, actorID, actorKind, action, target string
			var metadataBytes []byte
			var createdAt time.Time
			if err := rows.Scan(&id, &tID, &actorID, &actorKind, &action, &target,
				&metadataBytes, &createdAt); err != nil {
				return fmt.Errorf("scanning audit record: %w", err)
			}
			metadata := map[string]any{}
			if len(metadataBytes) > 0 {
				_ = json.Unmarshal(metadataBytes, &metadata)
			}
			records = append(records, domain.AuditRecord{
				ID: id, TenantID: tID, ActorID: actorID,
				ActorKind: domain.PrincipalKind(actorKind), Action: action,
				Target: target, Metadata: metadata, CreatedAt: createdAt,
			})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

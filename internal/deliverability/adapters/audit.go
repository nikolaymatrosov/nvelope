package adapters

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// AuditLog records privileged deliverability actions in the shared audit_log
// table. It implements the command layer's AuditWriter port.
type AuditLog struct {
	pool *pgxpool.Pool
}

// NewAuditLog builds an AuditLog writer over the given pool.
func NewAuditLog(pool *pgxpool.Pool) *AuditLog {
	return &AuditLog{pool: pool}
}

// Record appends one audit entry inside the tenant-bound transaction. Manual
// suppression and bounce-setting changes are always operator actions made
// through a workspace session.
func (a *AuditLog) Record(ctx context.Context, tenantID, actorID, action, target string) error {
	return tenantdb.WithTenant(ctx, a.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO audit_log (tenant_id, actor_id, actor_kind, action, target, metadata)
			 VALUES (@tenant_id, @actor_id, 'session', @action, @target, '{}'::jsonb)`,
			pgx.NamedArgs{
				"tenant_id": tenantID,
				"actor_id":  actorID,
				"action":    action,
				"target":    target,
			})
		if err != nil {
			return fmt.Errorf("recording audit entry: %w", err)
		}
		return nil
	})
}

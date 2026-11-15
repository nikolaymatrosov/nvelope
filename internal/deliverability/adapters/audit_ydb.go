package adapters

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// YDBAuditLog records privileged deliverability actions in the shared
// audit_log table (created by the iam migration). It implements the command
// layer's AuditWriter port.
type YDBAuditLog struct {
	driver *ydb.Driver
}

// NewYDBAuditLog builds a YDBAuditLog writer over the driver.
func NewYDBAuditLog(d *ydb.Driver) *YDBAuditLog {
	return &YDBAuditLog{driver: d}
}

// Record appends one audit entry. Manual suppression and bounce-setting
// changes are always operator actions made through a workspace session.
func (a *YDBAuditLog) Record(ctx context.Context, tenantID, actorID, action, target string) error {
	if err := a.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $actor_id AS Utf8;
DECLARE $action AS Utf8;
DECLARE $target AS Utf8;
INSERT INTO audit_log (tenant_id, created_at, id, actor_id, actor_kind, action, target, metadata)
VALUES ($tenant_id, CurrentUtcTimestamp(), $id, $actor_id, "session"u, $action, $target, "{}"u);`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(uuid.NewString()).
			Param("$actor_id").Text(actorID).
			Param("$action").Text(action).
			Param("$target").Text(target).
			Build())); err != nil {
		return fmt.Errorf("recording audit entry: %w", err)
	}
	return nil
}

package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// backend is one storage implementation of the deliverability repositories
// under contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name         string
	suppressions func(t *testing.T) domain.SuppressionRepository
	settings     func(t *testing.T) domain.SettingsRepository
	checker      func(t *testing.T) campaigndomain.SuppressionChecker
	audit        func(t *testing.T) command.AuditWriter
	// The seeds below create rows owned by other contexts. On Postgres they
	// are real rows, because the delivery tables carry foreign keys to them.
	// YDB has no foreign keys, so a bare UUID stands in for a tenant.
	newTenant func(t *testing.T) string
	// The probes read what no port exposes.
	auditCount func(t *testing.T, tenantID string) int
}

var backends = []backend{
	{
		name: "pg",
		suppressions: func(t *testing.T) domain.SuppressionRepository {
			return adapters.NewSuppressions(dbtest.AppPool(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewSettings(dbtest.AppPool(t))
		},
		checker: func(t *testing.T) campaigndomain.SuppressionChecker {
			return adapters.NewSuppressionChecker(dbtest.AppPool(t))
		},
		audit: func(t *testing.T) command.AuditWriter {
			return adapters.NewAuditLog(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
		auditCount: func(t *testing.T, tenantID string) int {
			return pgCount(t, dbtest.AppPool(t), tenantID, "SELECT count(*) FROM audit_log")
		},
	},
	{
		name: "ydb",
		suppressions: func(t *testing.T) domain.SuppressionRepository {
			return adapters.NewYDBSuppressions(dbtest.YDBMigrated(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewYDBSettings(dbtest.YDBMigrated(t))
		},
		checker: func(t *testing.T) campaigndomain.SuppressionChecker {
			return adapters.NewYDBSuppressionChecker(dbtest.YDBMigrated(t))
		},
		audit: func(t *testing.T) command.AuditWriter {
			return adapters.NewYDBAuditLog(dbtest.YDBMigrated(t))
		},
		newTenant: func(*testing.T) string { return uuid.NewString() },
		auditCount: func(t *testing.T, tenantID string) int {
			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM audit_log WHERE tenant_id = $tenant_id;`, tenantID)
		},
	},
}

// eachBackend runs body once per backend, as parallel subtests named after it.
func eachBackend(t *testing.T, body func(t *testing.T, b backend)) {
	t.Helper()
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			body(t, b)
		})
	}
}

// pgCount reads one count inside the tenant-bound transaction.
func pgCount(t *testing.T, pool *pgxpool.Pool, tenantID, sql string) int {
	t.Helper()
	var n int
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, sql).Scan(&n)
		}))
	return n
}

// ydbCount runs a single-row COUNT(*) query that takes $tenant_id.
func ydbCount(t *testing.T, q, tenantID string) int {
	t.Helper()
	row, err := dbtest.YDBMigrated(t).Query().QueryRow(context.Background(), q,
		query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build()))
	require.NoError(t, err)
	var n uint64
	require.NoError(t, row.Scan(&n))
	return int(n)
}

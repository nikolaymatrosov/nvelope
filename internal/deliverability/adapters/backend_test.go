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
	events       func(t *testing.T) domain.EventRepository
	suppressions func(t *testing.T) domain.SuppressionRepository
	settings     func(t *testing.T) domain.SettingsRepository
	checker      func(t *testing.T) campaigndomain.SuppressionChecker
	audit        func(t *testing.T) command.AuditWriter
	// The seeds below create rows owned by other contexts. On Postgres they
	// are real rows, because the delivery tables carry foreign keys to them.
	// YDB has no foreign keys, so a bare UUID stands in for a tenant.
	newTenant        func(t *testing.T) string
	newCampaign      func(t *testing.T, tenantID, status string) string
	newRecipient     func(t *testing.T, tenantID, campaignID, status, providerMsgID string) string
	newTransactional func(t *testing.T, tenantID, providerMsgID string) string
	// The probes read what no port exposes.
	eventCount func(t *testing.T, tenantID string) int
	auditCount func(t *testing.T, tenantID string) int
}

var backends = []backend{
	{
		name: "pg",
		events: func(t *testing.T) domain.EventRepository {
			return adapters.NewEvents(dbtest.AppPool(t))
		},
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
		newCampaign: func(t *testing.T, tenantID, status string) string {
			var id string
			require.NoError(t, tenantdb.WithTenant(context.Background(), dbtest.AppPool(t), tenantID,
				func(ctx context.Context, tx pgx.Tx) error {
					return tx.QueryRow(ctx,
						`INSERT INTO campaigns
						   (tenant_id, name, subject, body_html, from_name, from_local_part, status)
						 VALUES ($1, $2, 'Subject', '<p>hi</p>', 'Acme', 'news', $3) RETURNING id`,
						tenantID, "C-"+dbtest.RandString(), status).Scan(&id)
				}))
			return id
		},
		newRecipient: func(t *testing.T, tenantID, campaignID, status, providerMsgID string) string {
			var id string
			require.NoError(t, tenantdb.WithTenant(context.Background(), dbtest.AppPool(t), tenantID,
				func(ctx context.Context, tx pgx.Tx) error {
					var subscriberID string
					if err := tx.QueryRow(ctx,
						`INSERT INTO subscribers (tenant_id, email, name) VALUES ($1, $2, 'Sub')
						 RETURNING id`, tenantID, dbtest.RandString()+"@acme.com").Scan(&subscriberID); err != nil {
						return err
					}
					return tx.QueryRow(ctx,
						`INSERT INTO campaign_recipients
						   (tenant_id, campaign_id, subscriber_id, email, status, sent_at, provider_message_id)
						 VALUES (@tenant_id, @campaign_id, @subscriber_id, @email, @status, now(), @provider_message_id)
						 RETURNING id`,
						pgx.NamedArgs{
							"tenant_id":           tenantID,
							"campaign_id":         campaignID,
							"subscriber_id":       subscriberID,
							"email":               dbtest.RandString() + "@acme.com",
							"status":              status,
							"provider_message_id": providerMsgID,
						}).Scan(&id)
				}))
			return id
		},
		newTransactional: func(t *testing.T, tenantID, providerMsgID string) string {
			var id string
			require.NoError(t, tenantdb.WithTenant(context.Background(), dbtest.AppPool(t), tenantID,
				func(ctx context.Context, tx pgx.Tx) error {
					return tx.QueryRow(ctx,
						`INSERT INTO transactional_messages (tenant_id, provider_message_id, recipient_email)
						 VALUES ($1, $2, $3) RETURNING id`,
						tenantID, providerMsgID, "tx@acme.com").Scan(&id)
				}))
			return id
		},
		eventCount: func(t *testing.T, tenantID string) int {
			return pgCount(t, dbtest.AppPool(t), tenantID, "SELECT count(*) FROM delivery_events")
		},
		auditCount: func(t *testing.T, tenantID string) int {
			return pgCount(t, dbtest.AppPool(t), tenantID, "SELECT count(*) FROM audit_log")
		},
	},
	{
		name: "ydb",
		events: func(t *testing.T) domain.EventRepository {
			return adapters.NewYDBEvents(dbtest.YDBMigrated(t))
		},
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
		newCampaign: func(t *testing.T, tenantID, status string) string {
			id := uuid.NewString()
			ydbExec(t, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $status AS Utf8;
INSERT INTO campaigns
  (tenant_id, id, name, subject, body_html, body_text, from_name, from_local_part, status,
   max_send_errors, sent_count, failed_count, recipient_count, created_at, updated_at, archive_visible)
VALUES ($tenant_id, $id, $name, "Subject"u, "<p>hi</p>"u, ""u, "Acme"u, "news"u, $status,
        0, 0, 0, 0, CurrentUtcTimestamp(), CurrentUtcTimestamp(), false);`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$id").Text(id).
					Param("$name").Text("C-"+dbtest.RandString()).
					Param("$status").Text(status).Build()))
			return id
		},
		newRecipient: func(t *testing.T, tenantID, campaignID, status, providerMsgID string) string {
			id := uuid.NewString()
			ydbExec(t, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $status AS Utf8;
DECLARE $provider_message_id AS Utf8;
INSERT INTO campaign_recipients
  (tenant_id, campaign_id, id, subscriber_id, email, status, failure_reason, sent_at, provider_message_id)
VALUES ($tenant_id, $campaign_id, $id, $id, $email, $status, ""u, CurrentUtcTimestamp(), $provider_message_id);`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$campaign_id").Text(campaignID).
					Param("$id").Text(id).
					Param("$email").Text(dbtest.RandString()+"@acme.com").
					Param("$status").Text(status).
					Param("$provider_message_id").Text(providerMsgID).Build()))
			return id
		},
		newTransactional: func(t *testing.T, tenantID, providerMsgID string) string {
			id := uuid.NewString()
			ydbExec(t, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $provider_message_id AS Utf8;
INSERT INTO transactional_messages
  (tenant_id, id, template_id, provider_message_id, recipient_email, sent_at)
VALUES ($tenant_id, $id, NULL, $provider_message_id, "tx@acme.com"u, CurrentUtcTimestamp());`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$id").Text(id).
					Param("$provider_message_id").Text(providerMsgID).Build()))
			return id
		},
		eventCount: func(t *testing.T, tenantID string) int {
			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM delivery_events WHERE tenant_id = $tenant_id;`, tenantID)
		},
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

// seedSend creates a finished campaign with one sent recipient carrying
// providerMsgID, returning the campaign and recipient ids.
func seedSend(t *testing.T, b backend, tenantID, providerMsgID string) (campaignID, recipientID string) {
	t.Helper()
	campaignID = b.newCampaign(t, tenantID, "finished")
	return campaignID, b.newRecipient(t, tenantID, campaignID, "sent", providerMsgID)
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

// ydbExec runs one write statement with the given parameters.
func ydbExec(t *testing.T, q string, params query.ExecuteOption) {
	t.Helper()
	require.NoError(t, dbtest.YDBMigrated(t).Query().Exec(context.Background(), q, params))
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

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

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// backend is one storage implementation of the campaign repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name          string
	templates     func(t *testing.T) domain.TemplateRepository
	campaigns     func(t *testing.T) domain.CampaignRepository
	recipients    func(t *testing.T) domain.RecipientRepository
	tracking      func(t *testing.T) domain.TrackingRepository
	transactional func(t *testing.T) domain.TransactionalMessageRepository
	// The seeds below create rows owned by other contexts. On Postgres they
	// are real rows, because the campaign tables carry foreign keys to them.
	// YDB has no foreign keys and no campaign query reads these tables, so a
	// bare UUID stands in.
	newTenant        func(t *testing.T) string
	newSendingDomain func(t *testing.T, tenantID string) string
	newList          func(t *testing.T, tenantID string) string
	newSubscriber    func(t *testing.T, tenantID, email string) string
	// The probes read what no port exposes.
	providerMessageID  func(t *testing.T, tenantID, recipientID string) string
	transactionalCount func(t *testing.T, tenantID string) int
	clickCount         func(t *testing.T, tenantID, linkID string) int
	viewCount          func(t *testing.T, tenantID, campaignID string) int
}

var backends = []backend{
	{
		name: "pg",
		templates: func(t *testing.T) domain.TemplateRepository {
			return adapters.NewTemplates(dbtest.AppPool(t))
		},
		campaigns: func(t *testing.T) domain.CampaignRepository {
			return adapters.NewCampaigns(dbtest.AppPool(t))
		},
		recipients: func(t *testing.T) domain.RecipientRepository {
			return adapters.NewRecipients(dbtest.AppPool(t))
		},
		tracking: func(t *testing.T) domain.TrackingRepository {
			return adapters.NewTracking(dbtest.AppPool(t))
		},
		transactional: func(t *testing.T) domain.TransactionalMessageRepository {
			return adapters.NewTransactionalMessages(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
		newSendingDomain: func(t *testing.T, tenantID string) string {
			return seedSendingDomain(t, dbtest.AppPool(t), tenantID, "mail."+dbtest.RandString()+".com")
		},
		newList: func(t *testing.T, tenantID string) string {
			return seedList(t, dbtest.AppPool(t), tenantID, "L-"+dbtest.RandString())
		},
		newSubscriber: func(t *testing.T, tenantID, email string) string {
			return seedSubscriber(t, dbtest.AppPool(t), tenantID, email)
		},
		providerMessageID: func(t *testing.T, tenantID, recipientID string) string {
			return pgScalar(t, tenantID, "SELECT coalesce(provider_message_id, '') FROM campaign_recipients WHERE id = $1", recipientID)
		},
		transactionalCount: func(t *testing.T, tenantID string) int {
			return pgCount(t, tenantID, "SELECT count(*) FROM transactional_messages", nil)
		},
		clickCount: func(t *testing.T, tenantID, linkID string) int {
			return pgCount(t, tenantID, "SELECT count(*) FROM link_clicks WHERE link_id = $1", linkID)
		},
		viewCount: func(t *testing.T, tenantID, campaignID string) int {
			return pgCount(t, tenantID, "SELECT count(*) FROM campaign_views WHERE campaign_id = $1", campaignID)
		},
	},
	{
		name: "ydb",
		templates: func(t *testing.T) domain.TemplateRepository {
			return adapters.NewYDBTemplates(dbtest.YDBMigrated(t))
		},
		campaigns: func(t *testing.T) domain.CampaignRepository {
			return adapters.NewYDBCampaigns(dbtest.YDBMigrated(t))
		},
		recipients: func(t *testing.T) domain.RecipientRepository {
			return adapters.NewYDBRecipients(dbtest.YDBMigrated(t))
		},
		tracking: func(t *testing.T) domain.TrackingRepository {
			return adapters.NewYDBTracking(dbtest.YDBMigrated(t))
		},
		transactional: func(t *testing.T) domain.TransactionalMessageRepository {
			return adapters.NewYDBTransactionalMessages(dbtest.YDBMigrated(t))
		},
		newTenant:        func(*testing.T) string { return uuid.NewString() },
		newSendingDomain: func(*testing.T, string) string { return uuid.NewString() },
		newList:          func(*testing.T, string) string { return uuid.NewString() },
		newSubscriber:    func(*testing.T, string, string) string { return uuid.NewString() },
		providerMessageID: func(t *testing.T, tenantID, recipientID string) string {
			return ydbScalar(t, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT provider_message_id FROM campaign_recipients VIEW idx_campaign_recipients_id
WHERE tenant_id = $tenant_id AND id = $id;`, tenantID, "$id", recipientID)
		},
		transactionalCount: func(t *testing.T, tenantID string) int {
			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM transactional_messages WHERE tenant_id = $tenant_id;`, tenantID, "", "")
		},
		clickCount: func(t *testing.T, tenantID, linkID string) int {
			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
DECLARE $k AS Utf8;
SELECT COUNT(*) FROM link_clicks WHERE tenant_id = $tenant_id AND link_id = $k;`, tenantID, "$k", linkID)
		},
		viewCount: func(t *testing.T, tenantID, campaignID string) int {
			return ydbCount(t, `DECLARE $tenant_id AS Utf8;
DECLARE $k AS Utf8;
SELECT COUNT(*) FROM campaign_views WHERE tenant_id = $tenant_id AND campaign_id = $k;`, tenantID, "$k", campaignID)
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

// pgScalar reads one text value inside the tenant-bound transaction.
func pgScalar(t *testing.T, tenantID, sql, arg string) string {
	t.Helper()
	var out string
	require.NoError(t, tenantdb.WithTenant(context.Background(), dbtest.AppPool(t), tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, sql, arg).Scan(&out)
		}))
	return out
}

// pgCount reads one count inside the tenant-bound transaction. A nil arg runs
// the statement with no argument.
func pgCount(t *testing.T, tenantID, sql string, arg any) int {
	t.Helper()
	var n int
	require.NoError(t, tenantdb.WithTenant(context.Background(), dbtest.AppPool(t), tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			if arg == nil {
				return tx.QueryRow(ctx, sql).Scan(&n)
			}
			return tx.QueryRow(ctx, sql, arg).Scan(&n)
		}))
	return n
}

// ydbParams binds $tenant_id and, when name is set, one more Utf8 parameter.
func ydbParams(tenantID, name, value string) query.ExecuteOption {
	b := ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID)
	if name != "" {
		b = b.Param(name).Text(value)
	}
	return query.WithParameters(b.Build())
}

// ydbScalar reads one nullable text value, "" for NULL.
func ydbScalar(t *testing.T, q, tenantID, name, value string) string {
	t.Helper()
	row, err := dbtest.YDBMigrated(t).Query().QueryRow(context.Background(), q, ydbParams(tenantID, name, value))
	require.NoError(t, err)
	var out *string
	require.NoError(t, row.Scan(&out))
	if out == nil {
		return ""
	}
	return *out
}

// ydbCount reads one COUNT(*).
func ydbCount(t *testing.T, q, tenantID, name, value string) int {
	t.Helper()
	row, err := dbtest.YDBMigrated(t).Query().QueryRow(context.Background(), q, ydbParams(tenantID, name, value))
	require.NoError(t, err)
	var n uint64
	require.NoError(t, row.Scan(&n))
	return int(n)
}

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"cmp-"+dbtest.RandString()).Scan(&id))
	return id
}

// seedSubscriber inserts a subscriber inside the tenant-bound transaction and
// returns its id.
func seedSubscriber(t *testing.T, pool *pgxpool.Pool, tenantID, email string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO subscribers (tenant_id, email, name) VALUES ($1, $2, 'Sub')
				 RETURNING id`, tenantID, email).Scan(&id)
		}))
	return id
}

// seedSendingDomain inserts a verified sending domain and returns its id.
func seedSendingDomain(t *testing.T, pool *pgxpool.Pool, tenantID, name string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO sending_domains (tenant_id, domain, status, verified_at)
				 VALUES ($1, $2, 'verified', now()) RETURNING id`, tenantID, name).Scan(&id)
		}))
	return id
}

// seedList inserts a list inside the tenant-bound transaction and returns its
// id.
func seedList(t *testing.T, pool *pgxpool.Pool, tenantID, name string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO lists (tenant_id, name) VALUES ($1, $2) RETURNING id`,
				tenantID, name).Scan(&id)
		}))
	return id
}

// newCampaign builds a draft campaign for the given tenant.
func newCampaign(t *testing.T, tenantID, name, sendingDomainID string) *domain.Campaign {
	t.Helper()
	c, err := domain.NewCampaign(tenantID, name, "Subject", `<p>hi</p>`, "Hi",
		"Acme", "news", sendingDomainID, "", 100)
	require.NoError(t, err)
	return c
}

func newTemplate(t *testing.T, tenantID, name string) *domain.Template {
	t.Helper()
	tpl, err := domain.NewTemplate(tenantID, name, domain.KindCampaign, "Subject", "<p>b</p>", "")
	require.NoError(t, err)
	return tpl
}

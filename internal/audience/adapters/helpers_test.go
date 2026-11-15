package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// timeZeroAdapters is a fixed timestamp for hydrating entities in tests.
func timeZeroAdapters() time.Time { return time.Unix(0, 0).UTC() }

// backend is one storage implementation of the audience repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name        string
	lists       func(t *testing.T) domain.ListRepository
	subscribers func(t *testing.T) domain.SubscriberRepository
	memberships func(t *testing.T) domain.MembershipRepository
	fields      func(t *testing.T) domain.FieldRepository
	pages       func(t *testing.T) domain.SubscriptionPageRepository
	pending     func(t *testing.T) domain.PendingSubscriptionRepository
	// newTenant returns the id of a tenant row, which the tenant context
	// owns. On Postgres the audience tables carry a foreign key to it, so the
	// pg backend inserts a real row. YDB has no foreign keys and no audience
	// query reads the tenants table, so a bare UUID stands in.
	newTenant func(t *testing.T) string
	// newSendingDomain returns the id of a sending domain of the tenant,
	// which the sending context owns. On Postgres a subscription page carries
	// a foreign key to it, so the pg backend inserts a real row. No audience
	// query reads sending_domains, so a bare UUID stands in on YDB.
	newSendingDomain func(t *testing.T, tenantID string) string
}

var backends = []backend{
	{
		name: "pg",
		lists: func(t *testing.T) domain.ListRepository {
			return adapters.NewLists(dbtest.AppPool(t))
		},
		subscribers: func(t *testing.T) domain.SubscriberRepository {
			return adapters.NewSubscribers(dbtest.AppPool(t))
		},
		memberships: func(t *testing.T) domain.MembershipRepository {
			return adapters.NewMemberships(dbtest.AppPool(t))
		},
		fields: func(t *testing.T) domain.FieldRepository {
			return adapters.NewFields(dbtest.AppPool(t))
		},
		pages: func(t *testing.T) domain.SubscriptionPageRepository {
			return adapters.NewSubscriptionPages(dbtest.AppPool(t))
		},
		pending: func(t *testing.T) domain.PendingSubscriptionRepository {
			return adapters.NewPendingSubscriptions(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
		newSendingDomain: func(t *testing.T, tenantID string) string {
			return seedSendingDomain(t, dbtest.AppPool(t), tenantID)
		},
	},
	{
		name: "ydb",
		lists: func(t *testing.T) domain.ListRepository {
			return adapters.NewYDBLists(dbtest.YDBMigrated(t))
		},
		subscribers: func(t *testing.T) domain.SubscriberRepository {
			return adapters.NewYDBSubscribers(dbtest.YDBMigrated(t))
		},
		memberships: func(t *testing.T) domain.MembershipRepository {
			return adapters.NewYDBMemberships(dbtest.YDBMigrated(t))
		},
		fields: func(t *testing.T) domain.FieldRepository {
			return adapters.NewYDBFields(dbtest.YDBMigrated(t))
		},
		pages: func(t *testing.T) domain.SubscriptionPageRepository {
			return adapters.NewYDBSubscriptionPages(dbtest.YDBMigrated(t))
		},
		pending: func(t *testing.T) domain.PendingSubscriptionRepository {
			return adapters.NewYDBPendingSubscriptions(dbtest.YDBMigrated(t))
		},
		newTenant:        func(*testing.T) string { return uuid.NewString() },
		newSendingDomain: func(*testing.T, string) string { return uuid.NewString() },
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

// seedTenant inserts a tenant row directly and returns its id. The audience
// adapters need a real tenant to satisfy the tenant_id foreign key and the RLS
// policy. The row is inserted with raw SQL rather than through the tenant
// context's adapter so the audience context's tests import no other context.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"aud-"+dbtest.RandString()).Scan(&id))
	return id
}

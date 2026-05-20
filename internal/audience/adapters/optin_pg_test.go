package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// fakeSuppression is a test double for domain.SuppressionLookup: it reports the
// addresses it was built with as suppressed.
type fakeSuppression struct {
	suppressed map[string]string
}

func (f fakeSuppression) Suppressed(_ context.Context, _ string, emails []string) (map[string]string, error) {
	out := map[string]string{}
	for _, e := range emails {
		if reason, ok := f.suppressed[e]; ok {
			out[e] = reason
		}
	}
	return out, nil
}

// seedSendingDomain inserts a verified sending domain for the tenant so a
// subscription page can satisfy its foreign key.
func seedSendingDomain(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID)
	require.NoError(t, err)
	var id string
	require.NoError(t, tx.QueryRow(ctx,
		`INSERT INTO sending_domains (tenant_id, domain, status)
		 VALUES ($1, 'mail.example.com', 'verified') RETURNING id`, tenantID).Scan(&id))
	require.NoError(t, tx.Commit(ctx))
	return id
}

func newSubscriptionPage(t *testing.T, tenantID, slug, listID, domainID string) *domain.SubscriptionPage {
	t.Helper()
	p, err := domain.NewSubscriptionPage(tenantID, slug, "Join us", []string{listID},
		[]domain.FormField{{Key: "name", Label: "Name", Required: false}},
		domainID, "Newsletter", "hello")
	require.NoError(t, err)
	return p
}

func TestSubscriptionPagesAddGetUpdate(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	listID := addList(t, adapters.NewLists(pool), tenantID, "Newsletter")
	domainID := seedSendingDomain(t, pool, tenantID)
	repo := adapters.NewSubscriptionPages(pool)

	id, err := repo.Add(ctx, tenantID, newSubscriptionPage(t, tenantID, "join", listID, domainID))
	require.NoError(t, err)
	require.NotEmpty(t, id)

	got, err := repo.GetBySlug(ctx, tenantID, "join")
	require.NoError(t, err)
	require.Equal(t, "join", got.Slug())
	require.Equal(t, []string{listID}, got.TargetListIDs())
	require.True(t, got.Active())

	// A duplicate slug within the tenant is a conflict.
	_, err = repo.Add(ctx, tenantID, newSubscriptionPage(t, tenantID, "join", listID, domainID))
	require.ErrorIs(t, err, domain.ErrSubscriptionPageSlugTaken)

	// Update deactivates the page.
	require.NoError(t, repo.Update(ctx, tenantID, id,
		func(p *domain.SubscriptionPage) (*domain.SubscriptionPage, error) {
			return p, p.Reconfigure("join", "Join us", []string{listID}, p.Fields(),
				domainID, "Newsletter", "hello", false)
		}))
	got, err = repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.False(t, got.Active())
}

func TestPendingSubscriptionsUpsertGetRefreshDelete(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	listID := addList(t, adapters.NewLists(pool), tenantID, "Newsletter")
	domainID := seedSendingDomain(t, pool, tenantID)
	pages := adapters.NewSubscriptionPages(pool)
	pageID, err := pages.Add(ctx, tenantID, newSubscriptionPage(t, tenantID, "join", listID, domainID))
	require.NoError(t, err)

	repo := adapters.NewPendingSubscriptions(pool)
	rawToken, _ := token.New()
	pending, err := domain.NewPendingSubscription(tenantID, pageID, "p@example.com",
		domain.HydrateAttributes(nil), []string{listID}, token.Hash(rawToken),
		time.Now().Add(time.Hour))
	require.NoError(t, err)

	id, err := repo.Upsert(ctx, tenantID, pending)
	require.NoError(t, err)

	got, err := repo.GetByTokenHash(ctx, tenantID, token.Hash(rawToken))
	require.NoError(t, err)
	require.Equal(t, "p@example.com", got.Email())

	// Upserting the same address/page refreshes the existing row rather than
	// stacking a duplicate.
	newRaw, _ := token.New()
	refreshed, err := domain.NewPendingSubscription(tenantID, pageID, "p@example.com",
		domain.HydrateAttributes(nil), []string{listID}, token.Hash(newRaw),
		time.Now().Add(time.Hour))
	require.NoError(t, err)
	id2, err := repo.Upsert(ctx, tenantID, refreshed)
	require.NoError(t, err)
	require.Equal(t, id, id2, "a repeat submission refreshes the same pending row")

	// RefreshToken swaps the token; the old hash no longer resolves.
	rotated, _ := token.New()
	require.NoError(t, repo.RefreshToken(ctx, tenantID, id, token.Hash(rotated),
		time.Now().Add(2*time.Hour)))
	_, err = repo.GetByTokenHash(ctx, tenantID, token.Hash(newRaw))
	require.ErrorIs(t, err, domain.ErrPendingSubscriptionNotFound)
	_, err = repo.GetByTokenHash(ctx, tenantID, token.Hash(rotated))
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, tenantID, id))
	_, err = repo.Get(ctx, tenantID, id)
	require.ErrorIs(t, err, domain.ErrPendingSubscriptionNotFound)
	// Deleting a missing row is not an error — confirmation is idempotent.
	require.NoError(t, repo.Delete(ctx, tenantID, id))
}

func TestConfirmSubscriptionLifecycle(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	listID := addList(t, adapters.NewLists(pool), tenantID, "Newsletter")
	domainID := seedSendingDomain(t, pool, tenantID)

	pages := adapters.NewSubscriptionPages(pool)
	pending := adapters.NewPendingSubscriptions(pool)
	subscribers := adapters.NewSubscribers(pool)
	memberships := adapters.NewMemberships(pool)
	pageID, err := pages.Add(ctx, tenantID, newSubscriptionPage(t, tenantID, "join", listID, domainID))
	require.NoError(t, err)

	confirm := audiencecommand.NewConfirmSubscriptionHandler(pending, subscribers, memberships,
		fakeSuppression{})

	// Submit → pending row, then confirm → subscriber with a confirmed membership.
	rawToken, _ := token.New()
	ps, err := domain.NewPendingSubscription(tenantID, pageID, "alice@example.com",
		domain.HydrateAttributes(map[string]any{"name": "Alice"}), []string{listID},
		token.Hash(rawToken), time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = pending.Upsert(ctx, tenantID, ps)
	require.NoError(t, err)

	res, err := confirm.Handle(ctx, audiencecommand.ConfirmSubscription{TenantID: tenantID, Token: rawToken})
	require.NoError(t, err)
	require.False(t, res.AlreadyConfirmed)

	subs, total, err := subscribers.Search(ctx, tenantID, "alice@example.com", domain.Page{Limit: 1})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	mships, err := memberships.ForSubscriber(ctx, tenantID, subs[0].ID())
	require.NoError(t, err)
	require.Len(t, mships, 1)
	require.Equal(t, domain.SubscriptionConfirmed, mships[0].Status())

	// The pending row is consumed; a repeat visit is benign.
	res, err = confirm.Handle(ctx, audiencecommand.ConfirmSubscription{TenantID: tenantID, Token: rawToken})
	require.NoError(t, err)
	require.True(t, res.AlreadyConfirmed)
}

func TestConfirmSubscriptionRejectsExpiredAndSuppressed(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	listID := addList(t, adapters.NewLists(pool), tenantID, "Newsletter")
	domainID := seedSendingDomain(t, pool, tenantID)
	pages := adapters.NewSubscriptionPages(pool)
	pending := adapters.NewPendingSubscriptions(pool)
	pageID, err := pages.Add(ctx, tenantID, newSubscriptionPage(t, tenantID, "join", listID, domainID))
	require.NoError(t, err)

	// An expired confirmation link is rejected.
	expiredRaw, _ := token.New()
	expired, err := domain.NewPendingSubscription(tenantID, pageID, "old@example.com",
		domain.HydrateAttributes(nil), []string{listID}, token.Hash(expiredRaw),
		time.Now().Add(-time.Hour))
	require.NoError(t, err)
	_, err = pending.Upsert(ctx, tenantID, expired)
	require.NoError(t, err)
	confirm := audiencecommand.NewConfirmSubscriptionHandler(pending,
		adapters.NewSubscribers(pool), adapters.NewMemberships(pool), fakeSuppression{})
	_, err = confirm.Handle(ctx, audiencecommand.ConfirmSubscription{TenantID: tenantID, Token: expiredRaw})
	require.ErrorIs(t, err, domain.ErrConfirmationExpired)

	// A suppressed address is not silently re-subscribed.
	suppRaw, _ := token.New()
	supp, err := domain.NewPendingSubscription(tenantID, pageID, "blocked@example.com",
		domain.HydrateAttributes(nil), []string{listID}, token.Hash(suppRaw),
		time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = pending.Upsert(ctx, tenantID, supp)
	require.NoError(t, err)
	confirmSupp := audiencecommand.NewConfirmSubscriptionHandler(pending,
		adapters.NewSubscribers(pool), adapters.NewMemberships(pool),
		fakeSuppression{suppressed: map[string]string{"blocked@example.com": "complaint"}})
	_, err = confirmSupp.Handle(ctx, audiencecommand.ConfirmSubscription{TenantID: tenantID, Token: suppRaw})
	require.ErrorIs(t, err, domain.ErrAddressSuppressed)
}

package adapters_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
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
// subscription page can satisfy its Postgres foreign key.
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

// addSubscriptionPage persists a page with the given slug, targeting one new
// list, and returns the page id and the list id.
func addSubscriptionPage(t *testing.T, b backend, tenantID, slug string) (pageID, listID string) {
	t.Helper()
	listID = addList(t, b.lists(t), tenantID, "List "+slug)
	pageID, err := b.pages(t).Add(context.Background(), tenantID,
		newSubscriptionPage(t, tenantID, slug, listID, b.newSendingDomain(t, tenantID)))
	require.NoError(t, err)
	return pageID, listID
}

// newPending builds a pending subscription of email to the page, targeting
// listID, confirmed by rawToken.
func newPending(t *testing.T, tenantID, pageID, email, listID, rawToken string,
	expiresAt time.Time) *domain.PendingSubscription {
	t.Helper()
	p, err := domain.NewPendingSubscription(tenantID, pageID, email,
		domain.HydrateAttributes(map[string]any{"name": "Pat"}), []string{listID},
		token.Hash(rawToken), expiresAt)
	require.NoError(t, err)
	return p
}

func TestSubscriptionPagesAddGetUpdate(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.pages(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID := addList(t, b.lists(t), tenantID, "Newsletter")
		domainID := b.newSendingDomain(t, tenantID)

		id, err := repo.Add(ctx, tenantID, newSubscriptionPage(t, tenantID, "join", listID, domainID))
		require.NoError(t, err)
		require.NotEmpty(t, id)

		got, err := repo.GetBySlug(ctx, tenantID, "join")
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, "Join us", got.Title())
		require.Equal(t, []string{listID}, got.TargetListIDs())
		require.Equal(t, []domain.FormField{{Key: "name", Label: "Name"}}, got.Fields())
		require.Equal(t, domainID, got.SendingDomainID())
		require.Equal(t, "hello", got.FromLocalPart())
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

		// Renaming another page onto the taken slug is a conflict too.
		otherID, err := repo.Add(ctx, tenantID, newSubscriptionPage(t, tenantID, "other", listID, domainID))
		require.NoError(t, err)
		err = repo.Update(ctx, tenantID, otherID,
			func(p *domain.SubscriptionPage) (*domain.SubscriptionPage, error) {
				return p, p.Reconfigure("join", "Join us", []string{listID}, p.Fields(),
					domainID, "Newsletter", "hello", true)
			})
		require.ErrorIs(t, err, domain.ErrSubscriptionPageSlugTaken)

		all, err := repo.All(ctx, tenantID)
		require.NoError(t, err)
		require.Len(t, all, 2)
		require.Equal(t, otherID, all[0].ID(), "newest first")
		require.Equal(t, "other", all[0].Slug(), "the failed rename changed nothing")

		_, err = repo.Get(ctx, tenantID, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, err, domain.ErrSubscriptionPageNotFound)
		_, err = repo.GetBySlug(ctx, tenantID, "missing")
		require.ErrorIs(t, err, domain.ErrSubscriptionPageNotFound)
		err = repo.Update(ctx, tenantID, "00000000-0000-0000-0000-000000000000",
			func(p *domain.SubscriptionPage) (*domain.SubscriptionPage, error) { return p, nil })
		require.ErrorIs(t, err, domain.ErrSubscriptionPageNotFound)
	})
}

func TestSubscriptionPagesAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.pages(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)
		idA, _ := addSubscriptionPage(t, b, tenantA, "join")

		// Another tenant may use the same slug and sees nothing of tenant A.
		addSubscriptionPage(t, b, tenantB, "join")
		_, err := repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrSubscriptionPageNotFound)
		gotB, err := repo.GetBySlug(ctx, tenantB, "join")
		require.NoError(t, err)
		require.NotEqual(t, idA, gotB.ID())
		all, err := repo.All(ctx, tenantB)
		require.NoError(t, err)
		require.Len(t, all, 1)
	})
}

func TestPendingSubscriptionsUpsertGetRefreshDelete(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.pending(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pageID, listID := addSubscriptionPage(t, b, tenantID, "join")

		rawToken, _ := token.New()
		expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
		id, err := repo.Upsert(ctx, tenantID,
			newPending(t, tenantID, pageID, "P@Example.com", listID, rawToken, expires))
		require.NoError(t, err)

		got, err := repo.GetByTokenHash(ctx, tenantID, token.Hash(rawToken))
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, "p@example.com", got.Email())
		require.Equal(t, pageID, got.SubscriptionPageID())
		require.Equal(t, []string{listID}, got.TargetListIDs())
		require.Equal(t, map[string]any{"name": "Pat"}, got.Attributes().Values())
		require.True(t, expires.Equal(got.ExpiresAt()))

		// Upserting the same address/page refreshes the existing row rather
		// than stacking a duplicate, and the old token stops resolving.
		newRaw, _ := token.New()
		id2, err := repo.Upsert(ctx, tenantID,
			newPending(t, tenantID, pageID, "p@example.com", listID, newRaw, expires))
		require.NoError(t, err)
		require.Equal(t, id, id2, "a repeat submission refreshes the same pending row")
		_, err = repo.GetByTokenHash(ctx, tenantID, token.Hash(rawToken))
		require.ErrorIs(t, err, domain.ErrPendingSubscriptionNotFound)

		// RefreshToken swaps the token; the old hash no longer resolves.
		rotated, _ := token.New()
		require.NoError(t, repo.RefreshToken(ctx, tenantID, id, token.Hash(rotated),
			time.Now().Add(2*time.Hour)))
		_, err = repo.GetByTokenHash(ctx, tenantID, token.Hash(newRaw))
		require.ErrorIs(t, err, domain.ErrPendingSubscriptionNotFound)
		_, err = repo.GetByTokenHash(ctx, tenantID, token.Hash(rotated))
		require.NoError(t, err)
		require.ErrorIs(t, repo.RefreshToken(ctx, tenantID, "00000000-0000-0000-0000-000000000000",
			token.Hash(rotated), time.Now()), domain.ErrPendingSubscriptionNotFound)

		require.NoError(t, repo.Delete(ctx, tenantID, id))
		_, err = repo.Get(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrPendingSubscriptionNotFound)
		// Deleting a missing row is not an error — confirmation is idempotent.
		require.NoError(t, repo.Delete(ctx, tenantID, id))
	})
}

// TestPendingSubscriptionsUpsertIsRaceFree pins the atomicity of Postgres
// ON CONFLICT DO UPDATE: concurrent submissions of one address to one page
// (a double click) all return the one pending row.
func TestPendingSubscriptionsUpsertIsRaceFree(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.pending(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pageID, listID := addSubscriptionPage(t, b, tenantID, "join")

		const n = 8
		subs := make([]*domain.PendingSubscription, n)
		for i := range n {
			raw, _ := token.New()
			subs[i] = newPending(t, tenantID, pageID, "race@example.com", listID, raw, time.Now().Add(time.Hour))
		}
		ids := make([]string, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() { ids[i], errs[i] = repo.Upsert(ctx, tenantID, subs[i]) })
		}
		wg.Wait()
		for i := range n {
			require.NoError(t, errs[i])
			require.Equal(t, ids[0], ids[i], "every upsert returns the one pending row")
		}
	})
}

func TestConfirmSubscriptionLifecycle(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pageID, listID := addSubscriptionPage(t, b, tenantID, "join")
		pending, subscribers, memberships := b.pending(t), b.subscribers(t), b.memberships(t)

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
		require.Equal(t, "Alice", subs[0].Name())
		mships, err := memberships.ForSubscriber(ctx, tenantID, subs[0].ID())
		require.NoError(t, err)
		require.Len(t, mships, 1)
		require.Equal(t, domain.SubscriptionConfirmed, mships[0].Status())

		// The pending row is consumed; a repeat visit is benign.
		res, err = confirm.Handle(ctx, audiencecommand.ConfirmSubscription{TenantID: tenantID, Token: rawToken})
		require.NoError(t, err)
		require.True(t, res.AlreadyConfirmed)
	})
}

func TestConfirmSubscriptionRejectsExpiredAndSuppressed(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pageID, listID := addSubscriptionPage(t, b, tenantID, "join")
		pending := b.pending(t)

		// An expired confirmation link is rejected.
		expiredRaw, _ := token.New()
		_, err := pending.Upsert(ctx, tenantID, newPending(t, tenantID, pageID, "old@example.com",
			listID, expiredRaw, time.Now().Add(-time.Hour)))
		require.NoError(t, err)
		confirm := audiencecommand.NewConfirmSubscriptionHandler(pending,
			b.subscribers(t), b.memberships(t), fakeSuppression{})
		_, err = confirm.Handle(ctx, audiencecommand.ConfirmSubscription{TenantID: tenantID, Token: expiredRaw})
		require.ErrorIs(t, err, domain.ErrConfirmationExpired)

		// A suppressed address is not silently re-subscribed.
		suppRaw, _ := token.New()
		_, err = pending.Upsert(ctx, tenantID, newPending(t, tenantID, pageID, "blocked@example.com",
			listID, suppRaw, time.Now().Add(time.Hour)))
		require.NoError(t, err)
		confirmSupp := audiencecommand.NewConfirmSubscriptionHandler(pending,
			b.subscribers(t), b.memberships(t),
			fakeSuppression{suppressed: map[string]string{"blocked@example.com": "complaint"}})
		_, err = confirmSupp.Handle(ctx, audiencecommand.ConfirmSubscription{TenantID: tenantID, Token: suppRaw})
		require.ErrorIs(t, err, domain.ErrAddressSuppressed)
	})
}

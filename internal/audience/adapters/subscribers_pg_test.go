package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func newSubscriber(t *testing.T, tenantID, email string) *domain.Subscriber {
	t.Helper()
	attrs, err := domain.NewAttributes(map[string]any{"plan": "pro"})
	require.NoError(t, err)
	s, err := domain.NewSubscriber(tenantID, email, "Pat", attrs)
	require.NoError(t, err)
	return s
}

// addSubscriber persists a subscriber and returns its id.
func addSubscriber(t *testing.T, repo *adapters.Subscribers, tenantID, email string) string {
	t.Helper()
	id, err := repo.Add(context.Background(), tenantID, newSubscriber(t, tenantID, email))
	require.NoError(t, err)
	return id
}

func TestSubscribersAddGet(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscribers(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	addSubscriber(t, repo, tenantID, "a@example.com")
	subs, total, err := repo.Search(ctx, tenantID, "", domain.DefaultPage)
	require.NoError(t, err)
	require.Equal(t, 1, total)

	got, err := repo.Get(ctx, tenantID, subs[0].ID())
	require.NoError(t, err)
	require.Equal(t, "a@example.com", got.Email())
	v, ok := got.Attributes().Get("plan")
	require.True(t, ok)
	require.Equal(t, "pro", v)
}

func TestSubscribersDuplicateEmailConflicts(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscribers(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	addSubscriber(t, repo, tenantID, "dup@example.com")
	_, err := repo.Add(ctx, tenantID, newSubscriber(t, tenantID, "dup@example.com"))
	require.ErrorIs(t, err, domain.ErrSubscriberEmailTaken)
}

func TestSubscribersUpsertByEmail(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscribers(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	created, err := repo.UpsertByEmail(ctx, tenantID, newSubscriber(t, tenantID, "up@example.com"))
	require.NoError(t, err)
	require.True(t, created, "a new email inserts")

	updated := newSubscriber(t, tenantID, "up@example.com")
	updated.Rename("Changed")
	created, err = repo.UpsertByEmail(ctx, tenantID, updated)
	require.NoError(t, err)
	require.False(t, created, "an existing email updates")

	subs, total, err := repo.Search(ctx, tenantID, "up@", domain.DefaultPage)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "Changed", subs[0].Name())
}

func TestSubscribersUpdateStateAndDelete(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscribers(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	addSubscriber(t, repo, tenantID, "s@example.com")
	subs, _, err := repo.Search(ctx, tenantID, "", domain.DefaultPage)
	require.NoError(t, err)
	id := subs[0].ID()

	require.NoError(t, repo.Update(ctx, tenantID, id, func(s *domain.Subscriber) (*domain.Subscriber, error) {
		s.Blocklist()
		return s, nil
	}))
	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, domain.StateBlocklisted, got.State())

	require.NoError(t, repo.Delete(ctx, tenantID, id))
	_, err = repo.Get(ctx, tenantID, id)
	require.ErrorIs(t, err, domain.ErrSubscriberNotFound)
}

func TestSubscribersSearch(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscribers(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	addSubscriber(t, repo, tenantID, "alice@example.com")
	addSubscriber(t, repo, tenantID, "bob@example.com")

	subs, total, err := repo.Search(ctx, tenantID, "alice", domain.DefaultPage)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "alice@example.com", subs[0].Email())
}

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

// addSubscriberWith persists a subscriber with the given email and attributes.
func addSubscriberWith(t *testing.T, repo *adapters.Subscribers, tenantID, email string,
	attrs map[string]any) string {
	t.Helper()
	a, err := domain.NewAttributes(attrs)
	require.NoError(t, err)
	s, err := domain.NewSubscriber(tenantID, email, "Pat", a)
	require.NoError(t, err)
	id, err := repo.Add(context.Background(), tenantID, s)
	require.NoError(t, err)
	return id
}

func mustSegment(t *testing.T, node domain.Node) domain.Segment {
	t.Helper()
	seg, err := domain.NewSegment(node)
	require.NoError(t, err)
	return *seg
}

func TestSubscribersRunSegmentAttribute(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscribers(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	addSubscriberWith(t, repo, tenantID, "pro@example.com", map[string]any{"plan": "pro"})
	addSubscriberWith(t, repo, tenantID, "free@example.com", map[string]any{"plan": "free"})

	seg := mustSegment(t, domain.Node{
		Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "pro"},
	})
	subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "pro@example.com", subs[0].Email())
}

func TestSubscribersRunSegmentMembership(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscribers(pool)
	lists := adapters.NewLists(pool)
	memberships := adapters.NewMemberships(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	list, err := domain.NewList(tenantID, "Newsletter", "", domain.VisibilityPrivate,
		domain.OptInSingle, nil)
	require.NoError(t, err)
	listID, err := lists.Add(ctx, tenantID, list)
	require.NoError(t, err)

	memberID := addSubscriberWith(t, repo, tenantID, "in@example.com", nil)
	addSubscriberWith(t, repo, tenantID, "out@example.com", nil)
	require.NoError(t, memberships.Attach(ctx, tenantID, memberID, listID, domain.SubscriptionConfirmed))

	seg := mustSegment(t, domain.Node{
		Member: &domain.MemberCondition{ListID: listID},
	})
	subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "in@example.com", subs[0].Email())
}

func TestSubscribersRunSegmentCombined(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscribers(pool)
	lists := adapters.NewLists(pool)
	memberships := adapters.NewMemberships(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	list, err := domain.NewList(tenantID, "Newsletter", "", domain.VisibilityPrivate,
		domain.OptInSingle, nil)
	require.NoError(t, err)
	listID, err := lists.Add(ctx, tenantID, list)
	require.NoError(t, err)

	match := addSubscriberWith(t, repo, tenantID, "match@example.com", map[string]any{"plan": "pro"})
	wrongAttr := addSubscriberWith(t, repo, tenantID, "wrong@example.com", map[string]any{"plan": "free"})
	addSubscriberWith(t, repo, tenantID, "nomember@example.com", map[string]any{"plan": "pro"})
	require.NoError(t, memberships.Attach(ctx, tenantID, match, listID, domain.SubscriptionConfirmed))
	require.NoError(t, memberships.Attach(ctx, tenantID, wrongAttr, listID, domain.SubscriptionConfirmed))

	seg := mustSegment(t, domain.Node{
		Conj: domain.ConjAnd,
		Children: []domain.Node{
			{Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "pro"}},
			{Member: &domain.MemberCondition{ListID: listID}},
		},
	})
	subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "match@example.com", subs[0].Email())

	count, err := repo.CountSegment(ctx, tenantID, seg)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestSubscribersRunSegmentEmptyResult(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscribers(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	addSubscriberWith(t, repo, tenantID, "free@example.com", map[string]any{"plan": "free"})

	seg := mustSegment(t, domain.Node{
		Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "enterprise"},
	})
	subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
	require.NoError(t, err)
	require.Equal(t, 0, total)
	require.Empty(t, subs)
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

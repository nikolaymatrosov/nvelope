package adapters_test

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
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
func addSubscriber(t *testing.T, repo domain.SubscriberRepository, tenantID, email string) string {
	t.Helper()
	id, err := repo.Add(context.Background(), tenantID, newSubscriber(t, tenantID, email))
	require.NoError(t, err)
	return id
}

// addSubscriberNamed persists a subscriber with the given email, name and
// attributes and returns its id.
func addSubscriberNamed(t *testing.T, repo domain.SubscriberRepository, tenantID, email, name string,
	attrs map[string]any) string {
	t.Helper()
	a, err := domain.NewAttributes(attrs)
	require.NoError(t, err)
	s, err := domain.NewSubscriber(tenantID, email, name, a)
	require.NoError(t, err)
	id, err := repo.Add(context.Background(), tenantID, s)
	require.NoError(t, err)
	return id
}

// addSubscriberWith persists a subscriber with the given email and attributes.
func addSubscriberWith(t *testing.T, repo domain.SubscriberRepository, tenantID, email string,
	attrs map[string]any) string {
	t.Helper()
	return addSubscriberNamed(t, repo, tenantID, email, "Pat", attrs)
}

func mustSegment(t *testing.T, node domain.Node) domain.Segment {
	t.Helper()
	seg, err := domain.NewSegment(node)
	require.NoError(t, err)
	return *seg
}

// emails returns the subscribers' emails, sorted.
func emails(subs []*domain.Subscriber) []string {
	out := make([]string, len(subs))
	for i, s := range subs {
		out[i] = s.Email()
	}
	sort.Strings(out)
	return out
}

func TestSubscribersAddGet(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id := addSubscriberWith(t, repo, tenantID, "a@example.com",
			map[string]any{"plan": "pro", "seats": 3, "tags": []any{"x"}, "nested": map[string]any{"k": "v"}})
		subs, total, err := repo.Search(ctx, tenantID, "", domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, id, subs[0].ID())

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, "a@example.com", got.Email())
		require.Equal(t, "Pat", got.Name())
		require.Equal(t, domain.StateEnabled, got.State())
		require.Equal(t, map[string]any{
			"plan": "pro", "seats": float64(3), "tags": []any{"x"}, "nested": map[string]any{"k": "v"},
		}, got.Attributes().Values(), "attributes round-trip as decoded JSON")
		require.False(t, got.CreatedAt().IsZero())

		_, err = repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrSubscriberNotFound)
	})
}

func TestSubscribersDuplicateEmailConflicts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addSubscriber(t, repo, tenantID, "dup@example.com")
		_, err := repo.Add(ctx, tenantID, newSubscriber(t, tenantID, "DUP@example.com"))
		require.ErrorIs(t, err, domain.ErrSubscriberEmailTaken, "emails are unique case-insensitively")

		// Another tenant may reuse the email.
		other := b.newTenant(t)
		addSubscriber(t, repo, other, "dup@example.com")
	})
}

func TestSubscribersUpsertByEmail(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		created, err := repo.UpsertByEmail(ctx, tenantID, newSubscriber(t, tenantID, "up@example.com"))
		require.NoError(t, err)
		require.True(t, created, "a new email inserts")

		updated := newSubscriber(t, tenantID, "UP@example.com")
		updated.Rename("Changed")
		created, err = repo.UpsertByEmail(ctx, tenantID, updated)
		require.NoError(t, err)
		require.False(t, created, "an existing email updates")

		subs, total, err := repo.Search(ctx, tenantID, "up@", domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, "Changed", subs[0].Name())
	})
}

func TestSubscribersUpsertByEmailKeepsState(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id := addSubscriber(t, repo, tenantID, "blocked@example.com")
		require.NoError(t, repo.Update(ctx, tenantID, id, func(s *domain.Subscriber) (*domain.Subscriber, error) {
			s.Blocklist()
			return s, nil
		}))

		created, err := repo.UpsertByEmail(ctx, tenantID, newSubscriber(t, tenantID, "blocked@example.com"))
		require.NoError(t, err)
		require.False(t, created)
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.StateBlocklisted, got.State(), "an upsert never re-enables a subscriber")
	})
}

func TestSubscribersUpsertByEmailIsRaceFree(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		const n = 8
		var wg sync.WaitGroup
		results := make([]bool, n)
		errs := make([]error, n)
		for i := range n {
			wg.Go(func() {
				results[i], errs[i] = repo.UpsertByEmail(ctx, tenantID, newSubscriber(t, tenantID, "race@example.com"))
			})
		}
		wg.Wait()

		createdCount := 0
		for i := range n {
			require.NoError(t, errs[i])
			if results[i] {
				createdCount++
			}
		}
		require.Equal(t, 1, createdCount, "exactly one concurrent upsert creates the subscriber")
		_, total, err := repo.Search(ctx, tenantID, "race@", domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
	})
}

func TestSubscribersUpdateStateAndDelete(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id := addSubscriber(t, repo, tenantID, "s@example.com")

		require.NoError(t, repo.Update(ctx, tenantID, id, func(s *domain.Subscriber) (*domain.Subscriber, error) {
			s.Blocklist()
			s.Rename("Renamed")
			return s, nil
		}))
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.StateBlocklisted, got.State())
		require.Equal(t, "Renamed", got.Name())

		require.ErrorIs(t, repo.Update(ctx, tenantID, "not-a-uuid",
			func(s *domain.Subscriber) (*domain.Subscriber, error) { return s, nil }), domain.ErrSubscriberNotFound)

		require.NoError(t, repo.Delete(ctx, tenantID, id))
		_, err = repo.Get(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrSubscriberNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, id), domain.ErrSubscriberNotFound)
	})
}

func TestSubscribersDeleteCascadesMemberships(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		subs, lists, memberships := b.subscribers(t), b.lists(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		listID := addList(t, lists, tenantID, "Newsletter")
		id := addSubscriber(t, subs, tenantID, "gone@example.com")
		require.NoError(t, memberships.Attach(ctx, tenantID, id, listID, domain.SubscriptionConfirmed))

		require.NoError(t, subs.Delete(ctx, tenantID, id))
		got, err := memberships.ForSubscriber(ctx, tenantID, id)
		require.NoError(t, err)
		require.Empty(t, got, "the subscriber's memberships go with it")
		_, total, err := subs.InList(ctx, tenantID, listID, domain.DefaultPage)
		require.NoError(t, err)
		require.Zero(t, total)
	})
}

func TestSubscribersSearch(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addSubscriberNamed(t, repo, tenantID, "alice@example.com", "Алиса Петрова", nil)
		addSubscriberNamed(t, repo, tenantID, "bob@example.com", "Bob", nil)

		for _, tc := range []struct {
			q    string
			want []string
		}{
			{"alice", []string{"alice@example.com"}},
			{"ALICE@EXAMPLE", []string{"alice@example.com"}},
			{"петрова", []string{"alice@example.com"}},
			{"АЛИСА", []string{"alice@example.com"}},
			{"  bob ", []string{"bob@example.com"}},
			{"example.com", []string{"alice@example.com", "bob@example.com"}},
			{"", []string{"alice@example.com", "bob@example.com"}},
			{"nobody", []string{}},
		} {
			subs, total, err := repo.Search(ctx, tenantID, tc.q, domain.DefaultPage)
			require.NoError(t, err, tc.q)
			require.Equal(t, tc.want, emails(subs), tc.q)
			require.Equal(t, len(tc.want), total, tc.q)
		}

		page, total, err := repo.Search(ctx, tenantID, "", domain.Page{Offset: 0, Limit: 1})
		require.NoError(t, err)
		require.Equal(t, 2, total, "the total ignores the page")
		require.Len(t, page, 1)
	})
}

func TestSubscribersTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)

		idA := addSubscriber(t, repo, tenantA, "mine@example.com")

		subs, total, err := repo.Search(ctx, tenantB, "", domain.DefaultPage)
		require.NoError(t, err)
		require.Zero(t, total)
		require.Empty(t, subs)
		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrSubscriberNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantB, idA), domain.ErrSubscriberNotFound)
		count, err := repo.CountSegment(ctx, tenantB, mustSegment(t, domain.Node{
			Field: &domain.FieldCondition{Field: "email", Op: domain.OpExists},
		}))
		require.NoError(t, err)
		require.Zero(t, count)
		_, err = repo.Get(ctx, tenantA, idA)
		require.NoError(t, err)
	})
}

func TestSubscribersRunSegmentAttribute(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addSubscriberWith(t, repo, tenantID, "pro@example.com", map[string]any{"plan": "pro"})
		addSubscriberWith(t, repo, tenantID, "free@example.com", map[string]any{"plan": "free"})

		seg := mustSegment(t, domain.Node{
			Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "pro"},
		})
		subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, "pro@example.com", subs[0].Email())
	})
}

func TestSubscribersRunSegmentMembership(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo, lists, memberships := b.subscribers(t), b.lists(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		listID := addList(t, lists, tenantID, "Newsletter")
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
	})
}

func TestSubscribersRunSegmentCombined(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo, lists, memberships := b.subscribers(t), b.lists(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		listID := addList(t, lists, tenantID, "Newsletter")
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
	})
}

func TestSubscribersRunSegmentEmptyResult(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.subscribers(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addSubscriberWith(t, repo, tenantID, "free@example.com", map[string]any{"plan": "free"})

		seg := mustSegment(t, domain.Node{
			Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "enterprise"},
		})
		subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 0, total)
		require.Empty(t, subs)
	})
}

// TestSubscribersRunSegmentOperators holds the YQL segment translator to the
// Postgres one: every operator, on fields, attributes and memberships, plus
// nested groups, selects the same subscribers on both backends.
func TestSubscribersRunSegmentOperators(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo, lists, memberships := b.subscribers(t), b.lists(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		listID := addList(t, lists, tenantID, "Newsletter")
		alice := addSubscriberNamed(t, repo, tenantID, "alice@example.com", "Алиса Петрова",
			map[string]any{"plan": "pro", "n": 42, "s42": "42", "vip": true, "a.b": "dot"})
		bob := addSubscriberNamed(t, repo, tenantID, "bob@example.com", "Bob",
			map[string]any{"plan": "free", "n": 7})
		carol := addSubscriberNamed(t, repo, tenantID, "carol@example.com", "Carol", nil)
		require.NoError(t, repo.Update(ctx, tenantID, carol, func(s *domain.Subscriber) (*domain.Subscriber, error) {
			s.Blocklist()
			return s, nil
		}))
		require.NoError(t, memberships.Attach(ctx, tenantID, alice, listID, domain.SubscriptionConfirmed))
		require.NoError(t, memberships.Attach(ctx, tenantID, bob, listID, domain.SubscriptionUnconfirmed))

		const (
			a = "alice@example.com"
			o = "bob@example.com"
			c = "carol@example.com"
		)
		field := func(f string, op domain.SegmentOp, v string) domain.Node {
			return domain.Node{Field: &domain.FieldCondition{Field: f, Op: op, Value: v}}
		}
		attr := func(k string, op domain.SegmentOp, v any) domain.Node {
			return domain.Node{Attr: &domain.AttrCondition{Key: k, Op: op, Value: v}}
		}
		member := func(status string) domain.Node {
			return domain.Node{Member: &domain.MemberCondition{ListID: listID, Status: status}}
		}
		for _, tc := range []struct {
			name string
			node domain.Node
			want []string
		}{
			{"email eq is case-insensitive", field("email", domain.OpEq, "ALICE@Example.com"), []string{a}},
			{"email neq", field("email", domain.OpNeq, "alice@example.com"), []string{o, c}},
			{"name contains Cyrillic", field("name", domain.OpContains, "петров"), []string{a}},
			{"name contains ignores case", field("name", domain.OpContains, "OB"), []string{o}},
			{"state eq", field("state", domain.OpEq, "blocklisted"), []string{c}},
			{"name exists", field("name", domain.OpExists, ""), []string{a, o, c}},
			{"attr exists", attr("plan", domain.OpExists, nil), []string{a, o}},
			{"attr eq", attr("plan", domain.OpEq, "pro"), []string{a}},
			{"attr neq skips a missing key", attr("plan", domain.OpNeq, "pro"), []string{o}},
			{"attr contains ignores case", attr("plan", domain.OpContains, "RO"), []string{a}},
			{"attr gt", attr("n", domain.OpGt, 10), []string{a}},
			{"attr lt", attr("n", domain.OpLt, 10), []string{o}},
			{"attr gte", attr("n", domain.OpGte, 42), []string{a}},
			{"attr lte", attr("n", domain.OpLte, 7), []string{o}},
			{"attr gt on a numeric string", attr("s42", domain.OpGt, "10"), []string{a}},
			{"attr eq on a number", attr("n", domain.OpEq, float64(42)), []string{a}},
			{"attr eq on a boolean", attr("vip", domain.OpEq, true), []string{a}},
			{"attr key with a dot is literal", attr("a.b", domain.OpEq, "dot"), []string{a}},
			{"member any status", member(""), []string{a, o}},
			{"member with status", member(string(domain.SubscriptionConfirmed)), []string{a}},
			{"or group", domain.Node{Conj: domain.ConjOr, Children: []domain.Node{
				attr("plan", domain.OpEq, "free"), member(string(domain.SubscriptionConfirmed)),
			}}, []string{a, o}},
			{"nested groups", domain.Node{Conj: domain.ConjAnd, Children: []domain.Node{
				member(""),
				{Conj: domain.ConjOr, Children: []domain.Node{
					attr("n", domain.OpGt, 10), field("name", domain.OpEq, "Bob"),
				}},
			}}, []string{a, o}},
		} {
			seg := mustSegment(t, tc.node)
			subs, total, err := repo.RunSegment(ctx, tenantID, seg, domain.DefaultPage)
			require.NoError(t, err, tc.name)
			require.Equal(t, tc.want, emails(subs), tc.name)
			require.Equal(t, len(tc.want), total, tc.name)
			count, err := repo.CountSegment(ctx, tenantID, seg)
			require.NoError(t, err, tc.name)
			require.Equal(t, len(tc.want), count, tc.name)
		}
	})
}

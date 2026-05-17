package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// fakeLists is a minimal in-memory domain.ListRepository for query tests.
type fakeLists struct{ items []*domain.List }

func (f *fakeLists) Add(context.Context, string, *domain.List) (string, error) { return "", nil }
func (f *fakeLists) Update(context.Context, string, string, func(*domain.List) (*domain.List, error)) error {
	return nil
}
func (f *fakeLists) Delete(context.Context, string, string) error { return nil }
func (f *fakeLists) Get(_ context.Context, _, id string) (*domain.List, error) {
	for _, l := range f.items {
		if l.ID() == id {
			return l, nil
		}
	}
	return nil, domain.ErrListNotFound
}
func (f *fakeLists) All(context.Context, string, domain.Page) ([]*domain.List, int, error) {
	return f.items, len(f.items), nil
}

// fakeSubscribers is a minimal in-memory domain.SubscriberRepository.
type fakeSubscribers struct{ items []*domain.Subscriber }

func (f *fakeSubscribers) Add(context.Context, string, *domain.Subscriber) (string, error) {
	return "", nil
}
func (f *fakeSubscribers) Update(context.Context, string, string, func(*domain.Subscriber) (*domain.Subscriber, error)) error {
	return nil
}
func (f *fakeSubscribers) UpsertByEmail(context.Context, string, *domain.Subscriber) (bool, error) {
	return false, nil
}
func (f *fakeSubscribers) Delete(context.Context, string, string) error { return nil }
func (f *fakeSubscribers) Get(_ context.Context, _, id string) (*domain.Subscriber, error) {
	for _, s := range f.items {
		if s.ID() == id {
			return s, nil
		}
	}
	return nil, domain.ErrSubscriberNotFound
}
func (f *fakeSubscribers) Search(context.Context, string, string, domain.Page) ([]*domain.Subscriber, int, error) {
	return f.items, len(f.items), nil
}
func (f *fakeSubscribers) InList(context.Context, string, string, domain.Page) ([]*domain.Subscriber, int, error) {
	return f.items, len(f.items), nil
}
func (f *fakeSubscribers) RunSegment(context.Context, string, domain.Segment, domain.Page) ([]*domain.Subscriber, int, error) {
	return f.items, len(f.items), nil
}
func (f *fakeSubscribers) CountSegment(context.Context, string, domain.Segment) (int, error) {
	return len(f.items), nil
}

// fakeJobs is a minimal in-memory domain.JobRepository.
type fakeJobs struct{ summary domain.JobSummary }

func (f *fakeJobs) AddImport(context.Context, string, *domain.ImportJob, []byte) (string, error) {
	return "", nil
}
func (f *fakeJobs) AddExport(context.Context, string, *domain.ExportJob) (string, error) {
	return "", nil
}
func (f *fakeJobs) UpdateImport(context.Context, string, string, func(*domain.ImportJob) (*domain.ImportJob, error)) error {
	return nil
}
func (f *fakeJobs) UpdateExport(context.Context, string, string, func(*domain.ExportJob) (*domain.ExportJob, error)) error {
	return nil
}
func (f *fakeJobs) GetImport(context.Context, string, string) (*domain.ImportJob, error) {
	return nil, domain.ErrJobNotFound
}
func (f *fakeJobs) GetExport(context.Context, string, string) (*domain.ExportJob, error) {
	return nil, domain.ErrJobNotFound
}
func (f *fakeJobs) Summary(context.Context, string, string) (domain.JobSummary, error) {
	return f.summary, nil
}
func (f *fakeJobs) StagedFile(context.Context, string, string) ([]byte, error) { return nil, nil }
func (f *fakeJobs) StageResult(context.Context, string, string, string, []byte) error {
	return nil
}

// fakeMemberships is a minimal in-memory domain.MembershipRepository.
type fakeMemberships struct{ items []*domain.Membership }

func (f *fakeMemberships) Attach(context.Context, string, string, string, domain.SubscriptionStatus) error {
	return nil
}
func (f *fakeMemberships) Detach(context.Context, string, string, string) error { return nil }
func (f *fakeMemberships) SetStatus(context.Context, string, string, string, domain.SubscriptionStatus) error {
	return nil
}
func (f *fakeMemberships) ForSubscriber(context.Context, string, string) ([]*domain.Membership, error) {
	return f.items, nil
}

func TestListListsHandler(t *testing.T) {
	t.Parallel()
	l := domain.HydrateList("l1", "t1", "Newsletter", "", domain.VisibilityPrivate,
		domain.OptInSingle, []string{"x"}, time.Now(), time.Now())
	h := query.NewListListsHandler(&fakeLists{items: []*domain.List{l}})

	page, err := h.Handle(context.Background(), query.ListLists{TenantID: "t1"})
	require.NoError(t, err)
	require.Equal(t, 1, page.Total)
	require.Equal(t, "Newsletter", page.Lists[0].Name)
}

func TestGetListHandlerMissing(t *testing.T) {
	t.Parallel()
	h := query.NewGetListHandler(&fakeLists{})
	_, err := h.Handle(context.Background(), query.GetList{TenantID: "t1", ListID: "nope"})
	require.ErrorIs(t, err, domain.ErrListNotFound)
}

func TestSearchSubscribersHandler(t *testing.T) {
	t.Parallel()
	s := domain.HydrateSubscriber("s1", "t1", "a@b.com", "Pat", domain.StateEnabled,
		domain.HydrateAttributes(map[string]any{"plan": "pro"}), time.Now(), time.Now())
	h := query.NewSearchSubscribersHandler(&fakeSubscribers{items: []*domain.Subscriber{s}})

	page, err := h.Handle(context.Background(), query.SearchSubscribers{TenantID: "t1"})
	require.NoError(t, err)
	require.Equal(t, 1, page.Total)
	require.Equal(t, "a@b.com", page.Subscribers[0].Email)
}

func TestGetJobStatusHandler(t *testing.T) {
	t.Parallel()
	jobs := &fakeJobs{summary: domain.JobSummary{
		ID: "job-1", Kind: "import", Status: domain.JobCompleted,
		CreatedCount: 3, UpdatedCount: 1, FailedCount: 1,
		Failures: []domain.RowFailure{{Row: 4, Reason: "bad email"}},
	}}
	h := query.NewGetJobStatusHandler(jobs)

	view, err := h.Handle(context.Background(), query.GetJobStatus{TenantID: "t1", JobID: "job-1"})
	require.NoError(t, err)
	require.Equal(t, "completed", view.Status)
	require.Equal(t, 3, view.CreatedCount)
	require.Len(t, view.Failures, 1)
	require.Equal(t, "bad email", view.Failures[0].Reason)
}

func TestRunSegmentHandler(t *testing.T) {
	t.Parallel()
	s := domain.HydrateSubscriber("s1", "t1", "a@b.com", "Pat", domain.StateEnabled,
		domain.HydrateAttributes(map[string]any{"plan": "pro"}), time.Now(), time.Now())
	seg, err := domain.NewSegment(domain.Node{
		Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "pro"},
	})
	require.NoError(t, err)
	h := query.NewRunSegmentHandler(&fakeSubscribers{items: []*domain.Subscriber{s}})

	t.Run("returns matching subscribers", func(t *testing.T) {
		t.Parallel()
		page, err := h.Handle(context.Background(), query.RunSegment{TenantID: "t1", Segment: *seg})
		require.NoError(t, err)
		require.Equal(t, 1, page.Total)
		require.Equal(t, "a@b.com", page.Subscribers[0].Email)
	})

	t.Run("count only omits the subscriber list", func(t *testing.T) {
		t.Parallel()
		page, err := h.Handle(context.Background(),
			query.RunSegment{TenantID: "t1", Segment: *seg, CountOnly: true})
		require.NoError(t, err)
		require.Equal(t, 1, page.Total)
		require.Empty(t, page.Subscribers)
	})
}

func TestGetSubscriberHandlerIncludesMemberships(t *testing.T) {
	t.Parallel()
	s := domain.HydrateSubscriber("s1", "t1", "a@b.com", "Pat", domain.StateEnabled,
		domain.Attributes{}, time.Now(), time.Now())
	m := domain.HydrateMembership("t1", "s1", "l1", domain.SubscriptionConfirmed,
		time.Now(), time.Now())
	h := query.NewGetSubscriberHandler(
		&fakeSubscribers{items: []*domain.Subscriber{s}},
		&fakeMemberships{items: []*domain.Membership{m}})

	view, err := h.Handle(context.Background(), query.GetSubscriber{TenantID: "t1", SubscriberID: "s1"})
	require.NoError(t, err)
	require.Len(t, view.Memberships, 1)
	require.Equal(t, "l1", view.Memberships[0].ListID)
	require.Equal(t, "confirmed", view.Memberships[0].Status)
}

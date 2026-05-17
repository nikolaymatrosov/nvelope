package command_test

import (
	"context"
	"strconv"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// fakeLists is an in-memory domain.ListRepository for handler unit tests.
type fakeLists struct {
	byID map[string]*domain.List
	seq  int
}

func newFakeLists() *fakeLists { return &fakeLists{byID: map[string]*domain.List{}} }

func (f *fakeLists) Add(_ context.Context, tenantID string, l *domain.List) (string, error) {
	for _, existing := range f.byID {
		if existing.TenantID() == tenantID && existing.Name() == l.Name() {
			return "", domain.ErrListNameTaken
		}
	}
	f.seq++
	id := "list-" + strconv.Itoa(f.seq)
	f.byID[id] = domain.HydrateList(id, tenantID, l.Name(), l.Description(),
		l.Visibility(), l.OptIn(), l.Tags(), time.Now(), time.Now())
	return id, nil
}

func (f *fakeLists) Update(_ context.Context, tenantID, id string,
	fn func(*domain.List) (*domain.List, error)) error {
	l, ok := f.byID[id]
	if !ok || l.TenantID() != tenantID {
		return domain.ErrListNotFound
	}
	updated, err := fn(l)
	if err != nil {
		return err
	}
	f.byID[id] = updated
	return nil
}

func (f *fakeLists) Delete(_ context.Context, tenantID, id string) error {
	l, ok := f.byID[id]
	if !ok || l.TenantID() != tenantID {
		return domain.ErrListNotFound
	}
	delete(f.byID, id)
	return nil
}

func (f *fakeLists) Get(_ context.Context, tenantID, id string) (*domain.List, error) {
	l, ok := f.byID[id]
	if !ok || l.TenantID() != tenantID {
		return nil, domain.ErrListNotFound
	}
	return l, nil
}

func (f *fakeLists) All(_ context.Context, tenantID string, _ domain.Page) ([]*domain.List, int, error) {
	var out []*domain.List
	for _, l := range f.byID {
		if l.TenantID() == tenantID {
			out = append(out, l)
		}
	}
	return out, len(out), nil
}

// fakeSubscribers is an in-memory domain.SubscriberRepository.
type fakeSubscribers struct {
	byID map[string]*domain.Subscriber
	seq  int
}

func newFakeSubscribers() *fakeSubscribers {
	return &fakeSubscribers{byID: map[string]*domain.Subscriber{}}
}

func (f *fakeSubscribers) Add(_ context.Context, tenantID string, s *domain.Subscriber) (string, error) {
	for _, existing := range f.byID {
		if existing.TenantID() == tenantID && existing.Email() == s.Email() {
			return "", domain.ErrSubscriberEmailTaken
		}
	}
	f.seq++
	id := "sub-" + strconv.Itoa(f.seq)
	f.byID[id] = domain.HydrateSubscriber(id, tenantID, s.Email(), s.Name(), s.State(),
		s.Attributes(), time.Now(), time.Now())
	return id, nil
}

func (f *fakeSubscribers) Update(_ context.Context, tenantID, id string,
	fn func(*domain.Subscriber) (*domain.Subscriber, error)) error {
	s, ok := f.byID[id]
	if !ok || s.TenantID() != tenantID {
		return domain.ErrSubscriberNotFound
	}
	updated, err := fn(s)
	if err != nil {
		return err
	}
	f.byID[id] = updated
	return nil
}

func (f *fakeSubscribers) UpsertByEmail(_ context.Context, tenantID string,
	s *domain.Subscriber) (bool, error) {
	for id, existing := range f.byID {
		if existing.TenantID() == tenantID && existing.Email() == s.Email() {
			f.byID[id] = domain.HydrateSubscriber(id, tenantID, s.Email(), s.Name(),
				existing.State(), s.Attributes(), existing.CreatedAt(), time.Now())
			return false, nil
		}
	}
	_, err := f.Add(context.Background(), tenantID, s)
	return true, err
}

func (f *fakeSubscribers) Delete(_ context.Context, tenantID, id string) error {
	s, ok := f.byID[id]
	if !ok || s.TenantID() != tenantID {
		return domain.ErrSubscriberNotFound
	}
	delete(f.byID, id)
	return nil
}

func (f *fakeSubscribers) Get(_ context.Context, tenantID, id string) (*domain.Subscriber, error) {
	s, ok := f.byID[id]
	if !ok || s.TenantID() != tenantID {
		return nil, domain.ErrSubscriberNotFound
	}
	return s, nil
}

func (f *fakeSubscribers) Search(_ context.Context, tenantID, _ string,
	_ domain.Page) ([]*domain.Subscriber, int, error) {
	var out []*domain.Subscriber
	for _, s := range f.byID {
		if s.TenantID() == tenantID {
			out = append(out, s)
		}
	}
	return out, len(out), nil
}

func (f *fakeSubscribers) InList(context.Context, string, string,
	domain.Page) ([]*domain.Subscriber, int, error) {
	return nil, 0, nil
}

func (f *fakeSubscribers) RunSegment(context.Context, string, domain.Segment,
	domain.Page) ([]*domain.Subscriber, int, error) {
	return nil, 0, nil
}

func (f *fakeSubscribers) CountSegment(context.Context, string, domain.Segment) (int, error) {
	return 0, nil
}

// fakeJobs is an in-memory domain.JobRepository for handler unit tests.
type fakeJobs struct {
	imports map[string]*domain.ImportJob
	exports map[string]*domain.ExportJob
	seq     int
}

func newFakeJobs() *fakeJobs {
	return &fakeJobs{
		imports: map[string]*domain.ImportJob{},
		exports: map[string]*domain.ExportJob{},
	}
}

func (f *fakeJobs) AddImport(_ context.Context, tenantID string, j *domain.ImportJob, _ []byte) (string, error) {
	f.seq++
	id := "job-" + strconv.Itoa(f.seq)
	f.imports[id] = domain.HydrateImportJob(id, tenantID, j.RequestedBy(), j.FileName(),
		j.TargetListIDs(), j.Status(), 0, 0, 0, nil, time.Now(), nil, nil)
	return id, nil
}

func (f *fakeJobs) AddExport(_ context.Context, tenantID string, j *domain.ExportJob) (string, error) {
	f.seq++
	id := "job-" + strconv.Itoa(f.seq)
	f.exports[id] = domain.HydrateExportJob(id, tenantID, j.RequestedBy(), j.Selection(),
		j.ListID(), j.Segment(), j.Status(), 0, time.Now(), nil, nil)
	return id, nil
}

func (f *fakeJobs) UpdateImport(_ context.Context, _, id string,
	fn func(*domain.ImportJob) (*domain.ImportJob, error)) error {
	j, ok := f.imports[id]
	if !ok {
		return domain.ErrJobNotFound
	}
	updated, err := fn(j)
	if err != nil {
		return err
	}
	f.imports[id] = updated
	return nil
}

func (f *fakeJobs) UpdateExport(_ context.Context, _, id string,
	fn func(*domain.ExportJob) (*domain.ExportJob, error)) error {
	j, ok := f.exports[id]
	if !ok {
		return domain.ErrJobNotFound
	}
	updated, err := fn(j)
	if err != nil {
		return err
	}
	f.exports[id] = updated
	return nil
}

func (f *fakeJobs) GetImport(_ context.Context, _, id string) (*domain.ImportJob, error) {
	j, ok := f.imports[id]
	if !ok {
		return nil, domain.ErrJobNotFound
	}
	return j, nil
}

func (f *fakeJobs) GetExport(_ context.Context, _, id string) (*domain.ExportJob, error) {
	j, ok := f.exports[id]
	if !ok {
		return nil, domain.ErrJobNotFound
	}
	return j, nil
}

func (f *fakeJobs) Summary(_ context.Context, _, id string) (domain.JobSummary, error) {
	if j, ok := f.imports[id]; ok {
		c, u, fl := j.Counts()
		return domain.JobSummary{ID: id, Kind: "import", Status: j.Status(),
			CreatedCount: c, UpdatedCount: u, FailedCount: fl, Failures: j.Failures()}, nil
	}
	if j, ok := f.exports[id]; ok {
		return domain.JobSummary{ID: id, Kind: "export", Status: j.Status(),
			RowCount: j.RowCount()}, nil
	}
	return domain.JobSummary{}, domain.ErrJobNotFound
}

func (f *fakeJobs) StagedFile(context.Context, string, string) ([]byte, error) {
	return nil, nil
}

func (f *fakeJobs) StageResult(context.Context, string, string, string, []byte) error {
	return nil
}

// fakeEnqueuer records the jobs handed to the queue.
type fakeEnqueuer struct {
	imports []string
	exports []string
}

func (f *fakeEnqueuer) EnqueueImport(_ context.Context, _, jobID string) error {
	f.imports = append(f.imports, jobID)
	return nil
}

func (f *fakeEnqueuer) EnqueueExport(_ context.Context, _, jobID string) error {
	f.exports = append(f.exports, jobID)
	return nil
}

// membershipKey identifies one membership.
type membershipKey struct{ subscriberID, listID string }

// fakeMemberships is an in-memory domain.MembershipRepository.
type fakeMemberships struct {
	byKey map[membershipKey]*domain.Membership
}

func newFakeMemberships() *fakeMemberships {
	return &fakeMemberships{byKey: map[membershipKey]*domain.Membership{}}
}

func (f *fakeMemberships) Attach(_ context.Context, tenantID, subscriberID, listID string,
	status domain.SubscriptionStatus) error {
	k := membershipKey{subscriberID, listID}
	if _, ok := f.byKey[k]; ok {
		return domain.ErrMembershipExists
	}
	m := domain.HydrateMembership(tenantID, subscriberID, listID, status, time.Now(), time.Now())
	f.byKey[k] = m
	return nil
}

func (f *fakeMemberships) Detach(_ context.Context, _, subscriberID, listID string) error {
	k := membershipKey{subscriberID, listID}
	if _, ok := f.byKey[k]; !ok {
		return domain.ErrMembershipNotFound
	}
	delete(f.byKey, k)
	return nil
}

func (f *fakeMemberships) SetStatus(_ context.Context, tenantID, subscriberID, listID string,
	status domain.SubscriptionStatus) error {
	k := membershipKey{subscriberID, listID}
	m, ok := f.byKey[k]
	if !ok {
		return domain.ErrMembershipNotFound
	}
	if err := m.ChangeStatus(status); err != nil {
		return err
	}
	f.byKey[k] = domain.HydrateMembership(tenantID, subscriberID, listID, m.Status(),
		m.CreatedAt(), time.Now())
	return nil
}

func (f *fakeMemberships) ForSubscriber(_ context.Context, _, subscriberID string) ([]*domain.Membership, error) {
	var out []*domain.Membership
	for k, m := range f.byKey {
		if k.subscriberID == subscriberID {
			out = append(out, m)
		}
	}
	return out, nil
}

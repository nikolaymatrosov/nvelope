package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/sending/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// fakeRepo is an in-memory SendingDomainRepository for command unit tests.
type fakeRepo struct {
	byID   map[string]*domain.SendingDomain
	nextID int
	addErr error
	getErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[string]*domain.SendingDomain{}}
}

func (r *fakeRepo) Add(_ context.Context, tenantID string, d *domain.SendingDomain) (string, error) {
	if r.addErr != nil {
		return "", r.addErr
	}
	r.nextID++
	id := "dom-" + string(rune('0'+r.nextID))
	r.byID[id] = domain.HydrateSendingDomain(id, tenantID, d.Domain(), d.Status(),
		d.DKIMRecords(), d.SPFRecord(), d.DMARCRecord(), d.IdentityRef(),
		d.FailureReason(), d.CreatedAt(), d.VerifiedAt(), d.LastCheckedAt())
	return id, nil
}

func (r *fakeRepo) Get(_ context.Context, _, id string) (*domain.SendingDomain, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	d, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrDomainNotFound
	}
	return d, nil
}

func (r *fakeRepo) Update(_ context.Context, _, id string,
	fn func(*domain.SendingDomain) (*domain.SendingDomain, error)) error {
	d, ok := r.byID[id]
	if !ok {
		return domain.ErrDomainNotFound
	}
	updated, err := fn(d)
	if err != nil {
		return err
	}
	r.byID[id] = updated
	return nil
}

func (r *fakeRepo) All(context.Context, string) ([]*domain.SendingDomain, error) { return nil, nil }
func (r *fakeRepo) PendingIDs(context.Context, string) ([]string, error)         { return nil, nil }

// fakeProvisioner is a deterministic DomainProvisioner.
type fakeProvisioner struct {
	err error
}

func (p fakeProvisioner) Provision(_ context.Context, dom string) (domain.ProvisionResult, error) {
	if p.err != nil {
		return domain.ProvisionResult{}, p.err
	}
	return domain.ProvisionResult{
		IdentityRef: dom,
		DKIMRecords: []domain.DNSRecord{{Type: "CNAME", Name: "sel._domainkey." + dom, Value: "v"}},
		SPFRecord:   "v=spf1 ~all",
		DMARCRecord: "v=DMARC1; p=none;",
	}, nil
}

// fakeEnqueuer records EnqueueVerify calls.
type fakeEnqueuer struct {
	calls []string
	err   error
}

func (e *fakeEnqueuer) EnqueueVerify(_ context.Context, _, domainID string) error {
	if e.err != nil {
		return e.err
	}
	e.calls = append(e.calls, domainID)
	return nil
}

func TestAddDomainProvisionsAndEnqueues(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	enq := &fakeEnqueuer{}
	h := command.NewAddDomainHandler(repo, fakeProvisioner{}, enq)

	res, err := h.Handle(context.Background(), command.AddDomain{
		TenantID: "tenant-1", Domain: "mail.acme.com",
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.DomainID)
	require.Equal(t, []string{res.DomainID}, enq.calls, "the verification poll is enqueued")

	stored := repo.byID[res.DomainID]
	require.Equal(t, domain.StatusPending, stored.Status())
	require.Equal(t, "v=spf1 ~all", stored.SPFRecord())
	require.Len(t, stored.DKIMRecords(), 1)
}

func TestAddDomainRejectsInvalidName(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	enq := &fakeEnqueuer{}
	h := command.NewAddDomainHandler(repo, fakeProvisioner{}, enq)

	_, err := h.Handle(context.Background(), command.AddDomain{TenantID: "t1", Domain: "not a domain"})
	require.ErrorIs(t, err, domain.ErrDomainInvalid)
	require.Empty(t, enq.calls, "an invalid domain is never provisioned or enqueued")
}

func TestAddDomainSurfacesProvisioningFailure(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	enq := &fakeEnqueuer{}
	h := command.NewAddDomainHandler(repo, fakeProvisioner{err: domain.ErrProvisioningFailed}, enq)

	_, err := h.Handle(context.Background(), command.AddDomain{TenantID: "t1", Domain: "mail.acme.com"})
	require.ErrorIs(t, err, domain.ErrProvisioningFailed)
	require.Empty(t, repo.byID, "a provisioning failure persists nothing")
}

func TestRecheckDomainEnqueuesForPending(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	enq := &fakeEnqueuer{}
	d, _ := domain.NewSendingDomain("tenant-1", "mail.acme.com")
	id, _ := repo.Add(context.Background(), "tenant-1", d)

	h := command.NewRecheckDomainHandler(repo, enq)
	require.NoError(t, h.Handle(context.Background(), command.RecheckDomain{
		TenantID: "tenant-1", DomainID: id,
	}))
	require.Equal(t, []string{id}, enq.calls)
}

func TestRecheckDomainRejectsNonPending(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	enq := &fakeEnqueuer{}
	d, _ := domain.NewSendingDomain("tenant-1", "mail.acme.com")
	id, _ := repo.Add(context.Background(), "tenant-1", d)
	require.NoError(t, repo.Update(context.Background(), "tenant-1", id,
		func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
			return d, d.MarkVerified(time.Now())
		}))

	h := command.NewRecheckDomainHandler(repo, enq)
	err := h.Handle(context.Background(), command.RecheckDomain{TenantID: "tenant-1", DomainID: id})
	require.ErrorIs(t, err, domain.ErrDomainNotPending)
	require.Empty(t, enq.calls)
}

func TestRecheckDomainNotFound(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	h := command.NewRecheckDomainHandler(repo, &fakeEnqueuer{})
	err := h.Handle(context.Background(), command.RecheckDomain{TenantID: "t1", DomainID: "missing"})
	require.ErrorIs(t, err, domain.ErrDomainNotFound)
}

func TestAddDomainPropagatesRepositoryError(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	repo.addErr = errors.New("db down")
	h := command.NewAddDomainHandler(repo, fakeProvisioner{}, &fakeEnqueuer{})
	_, err := h.Handle(context.Background(), command.AddDomain{TenantID: "t1", Domain: "mail.acme.com"})
	require.Error(t, err)
}

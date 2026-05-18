// Package command holds the sending context's state-changing handlers, named
// in business language.
package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// DomainVerifyEnqueuer hands a sending domain to the durable queue for
// verification polling. It is declared here, by the command layer that depends
// on it, and implemented by the platform/jobs River adapter.
type DomainVerifyEnqueuer interface {
	EnqueueVerify(ctx context.Context, tenantID, domainID string) error
}

// AddDomain is the request to register a sending domain.
type AddDomain struct {
	TenantID string
	Domain   string
}

// AddDomainResult carries the new domain's id.
type AddDomainResult struct {
	DomainID string
}

// AddDomainHandler handles the AddDomain command.
type AddDomainHandler struct {
	domains     domain.SendingDomainRepository
	provisioner domain.DomainProvisioner
	enqueuer    DomainVerifyEnqueuer
}

// NewAddDomainHandler builds the handler, failing fast on a nil dependency.
func NewAddDomainHandler(domains domain.SendingDomainRepository,
	provisioner domain.DomainProvisioner, enqueuer DomainVerifyEnqueuer) AddDomainHandler {
	if domains == nil || provisioner == nil || enqueuer == nil {
		panic("nil dependency")
	}
	return AddDomainHandler{domains: domains, provisioner: provisioner, enqueuer: enqueuer}
}

// Handle validates the domain, provisions a provider identity synchronously so
// the tenant receives DNS records immediately, persists the pending domain,
// and enqueues the verification poll.
func (h AddDomainHandler) Handle(ctx context.Context, cmd AddDomain) (AddDomainResult, error) {
	d, err := domain.NewSendingDomain(cmd.TenantID, cmd.Domain)
	if err != nil {
		return AddDomainResult{}, err
	}
	res, err := h.provisioner.Provision(ctx, d.Domain())
	if err != nil {
		return AddDomainResult{}, err
	}
	d.ApplyProvisioning(res.IdentityRef, res.DKIMRecords, res.SPFRecord, res.DMARCRecord)

	id, err := h.domains.Add(ctx, cmd.TenantID, d)
	if err != nil {
		return AddDomainResult{}, err
	}
	if err := h.enqueuer.EnqueueVerify(ctx, cmd.TenantID, id); err != nil {
		return AddDomainResult{}, err
	}
	return AddDomainResult{DomainID: id}, nil
}

// RecheckDomain is the request to trigger an immediate verification re-check.
type RecheckDomain struct {
	TenantID string
	DomainID string
}

// RecheckDomainHandler handles the RecheckDomain command.
type RecheckDomainHandler struct {
	domains  domain.SendingDomainRepository
	enqueuer DomainVerifyEnqueuer
}

// NewRecheckDomainHandler builds the handler, failing fast on a nil dependency.
func NewRecheckDomainHandler(domains domain.SendingDomainRepository,
	enqueuer DomainVerifyEnqueuer) RecheckDomainHandler {
	if domains == nil || enqueuer == nil {
		panic("nil dependency")
	}
	return RecheckDomainHandler{domains: domains, enqueuer: enqueuer}
}

// Handle enqueues a fresh verification poll, rejecting a domain that is no
// longer pending.
func (h RecheckDomainHandler) Handle(ctx context.Context, cmd RecheckDomain) error {
	d, err := h.domains.Get(ctx, cmd.TenantID, cmd.DomainID)
	if err != nil {
		return err
	}
	if !d.IsPending() {
		return domain.ErrDomainNotPending
	}
	return h.enqueuer.EnqueueVerify(ctx, cmd.TenantID, cmd.DomainID)
}

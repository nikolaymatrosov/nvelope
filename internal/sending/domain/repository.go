package domain

import "context"

// SendingDomainRepository persists sending domains. It is declared here, by the
// domain that depends on it; the pgx implementation lives in the adapters
// layer. Every operation runs inside a tenant-bound (app.tenant_id)
// transaction.
type SendingDomainRepository interface {
	// Add persists a new sending domain and returns its database-assigned id.
	// It returns ErrDomainAlreadyExists when the tenant already has that
	// domain.
	Add(ctx context.Context, tenantID string, d *SendingDomain) (string, error)
	// Get returns the domain, or ErrDomainNotFound.
	Get(ctx context.Context, tenantID, id string) (*SendingDomain, error)
	// Update loads the domain, runs fn, and persists the result. The closure is
	// the transaction boundary. It returns ErrDomainNotFound when absent.
	Update(ctx context.Context, tenantID, id string, fn func(*SendingDomain) (*SendingDomain, error)) error
	// All returns every sending domain of the tenant.
	All(ctx context.Context, tenantID string) ([]*SendingDomain, error)
	// PendingIDs lists the ids of domains still awaiting verification — for the
	// scheduler's recovery sweep.
	PendingIDs(ctx context.Context, tenantID string) ([]string, error)
}

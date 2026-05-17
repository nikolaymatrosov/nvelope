package domain

import "context"

// Page is a pagination request. Offset is the zero-based row offset; Limit is
// the maximum number of rows to return.
type Page struct {
	Offset int
	Limit  int
}

// DefaultPage is the pagination applied when a caller supplies none.
var DefaultPage = Page{Offset: 0, Limit: 50}

// Normalize clamps a page request to sane bounds, returning the page actually
// used. A non-positive limit falls back to the default; an oversized limit is
// capped; a negative offset is zeroed.
func (p Page) Normalize() Page {
	out := p
	if out.Limit <= 0 {
		out.Limit = DefaultPage.Limit
	}
	if out.Limit > 200 {
		out.Limit = 200
	}
	if out.Offset < 0 {
		out.Offset = 0
	}
	return out
}

// ListRepository persists lists. It is declared here, by the domain that
// depends on it; the pgx implementation lives in the adapters layer. Every
// operation runs inside a tenant-bound (app.tenant_id) transaction.
type ListRepository interface {
	// Add persists a new list and returns its database-assigned id. It returns
	// ErrListNameTaken when the tenant already has a list with that name.
	Add(ctx context.Context, tenantID string, l *List) (string, error)
	// Update loads the list, runs fn, and persists the result. The closure is
	// the transaction boundary. It returns ErrListNotFound when absent.
	Update(ctx context.Context, tenantID, id string, fn func(*List) (*List, error)) error
	// Delete removes the list and cascades its memberships. It returns
	// ErrListNotFound when absent.
	Delete(ctx context.Context, tenantID, id string) error
	// Get returns the list, or ErrListNotFound.
	Get(ctx context.Context, tenantID, id string) (*List, error)
	// All returns a page of the tenant's lists and the total count.
	All(ctx context.Context, tenantID string, page Page) ([]*List, int, error)
}

// SubscriberRepository persists subscribers. Every operation runs inside a
// tenant-bound transaction.
type SubscriberRepository interface {
	// Add persists a new subscriber and returns its database-assigned id. It
	// returns ErrSubscriberEmailTaken when the tenant already has that email.
	Add(ctx context.Context, tenantID string, s *Subscriber) (string, error)
	// Update loads the subscriber, runs fn, and persists the result.
	Update(ctx context.Context, tenantID, id string, fn func(*Subscriber) (*Subscriber, error)) error
	// UpsertByEmail creates the subscriber if its email is new, or updates the
	// existing one, reporting whether a new row was created.
	UpsertByEmail(ctx context.Context, tenantID string, s *Subscriber) (created bool, err error)
	// Delete removes the subscriber and cascades its memberships.
	Delete(ctx context.Context, tenantID, id string) error
	// Get returns the subscriber, or ErrSubscriberNotFound.
	Get(ctx context.Context, tenantID, id string) (*Subscriber, error)
	// Search returns a page of subscribers matching the free-text query q
	// (empty q matches all) and the total count.
	Search(ctx context.Context, tenantID, q string, page Page) ([]*Subscriber, int, error)
	// InList returns a page of the subscribers attached to one list and the
	// total count.
	InList(ctx context.Context, tenantID, listID string, page Page) ([]*Subscriber, int, error)
	// RunSegment translates a validated Segment to parameterized SQL and
	// returns a page of the matching subscribers and the total count.
	RunSegment(ctx context.Context, tenantID string, seg Segment, page Page) ([]*Subscriber, int, error)
	// CountSegment returns only the count of subscribers matching a Segment.
	CountSegment(ctx context.Context, tenantID string, seg Segment) (int, error)
}

// JobSummary is the kind-agnostic status of an import or export job, used by
// the GetJobStatus query.
type JobSummary struct {
	ID           string
	Kind         string // "import" or "export"
	Status       JobStatus
	FileName     string
	CreatedCount int
	UpdatedCount int
	FailedCount  int
	RowCount     int
	Failures     []RowFailure
}

// JobRepository persists import and export job records and their staged
// files. Every operation runs inside a tenant-bound transaction.
type JobRepository interface {
	// AddImport persists a new import job with its staged upload, returning
	// the job's database-assigned id.
	AddImport(ctx context.Context, tenantID string, j *ImportJob, fileBytes []byte) (string, error)
	// AddExport persists a new export job, returning its id.
	AddExport(ctx context.Context, tenantID string, j *ExportJob) (string, error)
	// UpdateImport loads the import job, runs fn, and persists the result.
	UpdateImport(ctx context.Context, tenantID, id string, fn func(*ImportJob) (*ImportJob, error)) error
	// UpdateExport loads the export job, runs fn, and persists the result.
	UpdateExport(ctx context.Context, tenantID, id string, fn func(*ExportJob) (*ExportJob, error)) error
	// GetImport returns the import job, or ErrJobNotFound.
	GetImport(ctx context.Context, tenantID, id string) (*ImportJob, error)
	// GetExport returns the export job, or ErrJobNotFound.
	GetExport(ctx context.Context, tenantID, id string) (*ExportJob, error)
	// Summary returns the kind-agnostic status of any job, or ErrJobNotFound.
	Summary(ctx context.Context, tenantID, id string) (JobSummary, error)
	// StagedFile returns the staged file bytes for a job.
	StagedFile(ctx context.Context, tenantID, id string) ([]byte, error)
	// StageResult writes a generated file (an export result) onto the job.
	StageResult(ctx context.Context, tenantID, id, fileName string, data []byte) error
}

// MembershipRepository persists the link between subscribers and lists. Every
// operation runs inside a tenant-bound transaction.
type MembershipRepository interface {
	// Attach links a subscriber to a list with the given status. Re-attaching
	// an existing membership is rejected with a conflict.
	Attach(ctx context.Context, tenantID, subscriberID, listID string, status SubscriptionStatus) error
	// Detach removes a membership. It returns ErrMembershipNotFound when absent.
	Detach(ctx context.Context, tenantID, subscriberID, listID string) error
	// SetStatus changes a membership's subscription status through the domain
	// state machine. It returns ErrMembershipNotFound when absent.
	SetStatus(ctx context.Context, tenantID, subscriberID, listID string, status SubscriptionStatus) error
	// ForSubscriber returns every membership of one subscriber.
	ForSubscriber(ctx context.Context, tenantID, subscriberID string) ([]*Membership, error)
}

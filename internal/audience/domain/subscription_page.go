package domain

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// slugPattern is a lowercase kebab-case URL segment.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// localPartPattern is a conservative RFC-5321 local part.
var localPartPattern = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+$`)

// FormField is one configurable field, beyond the always-present email, that a
// public subscription page collects.
type FormField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

// SubscriptionPage is a tenant's configuration of one public subscription
// page: the lists a confirmed subscriber joins, the extra fields the form
// collects, and the sending identity its double-opt-in email is sent from. It
// is a tenant-plane aggregate reached only through the RLS-bound transaction
// owned by its repository adapter.
type SubscriptionPage struct {
	id              string
	tenantID        string
	slug            string
	title           string
	targetListIDs   []string
	fields          []FormField
	sendingDomainID string
	fromName        string
	fromLocalPart   string
	active          bool
	createdAt       time.Time
	updatedAt       time.Time
}

func validateFields(fields []FormField) ([]FormField, error) {
	out := make([]FormField, 0, len(fields))
	seen := map[string]bool{}
	for _, f := range fields {
		key := strings.TrimSpace(f.Key)
		label := strings.TrimSpace(f.Label)
		if key == "" || label == "" {
			return nil, apperr.NewIncorrectInput("validation_failed",
				"every form field needs a key and a label")
		}
		if seen[key] {
			return nil, apperr.NewIncorrectInput("validation_failed",
				"form field keys must be unique")
		}
		seen[key] = true
		out = append(out, FormField{Key: key, Label: label, Required: f.Required})
	}
	return out, nil
}

func validateSubscriptionPage(slug, title, sendingDomainID, fromName, fromLocalPart string,
	targetListIDs []string) (string, string, string, error) {

	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugPattern.MatchString(slug) {
		return "", "", "", apperr.NewIncorrectInput("validation_failed",
			"page slug must be lowercase letters, digits, and hyphens")
	}
	if strings.TrimSpace(title) == "" {
		return "", "", "", apperr.NewIncorrectInput("validation_failed", "page title is required")
	}
	if len(targetListIDs) == 0 {
		return "", "", "", apperr.NewIncorrectInput("validation_failed",
			"a subscription page must target at least one list")
	}
	if sendingDomainID == "" {
		return "", "", "", apperr.NewIncorrectInput("validation_failed",
			"a verified sending domain is required")
	}
	fromName = strings.TrimSpace(fromName)
	if fromName == "" {
		return "", "", "", apperr.NewIncorrectInput("validation_failed", "a from-name is required")
	}
	fromLocalPart = strings.TrimSpace(fromLocalPart)
	if !localPartPattern.MatchString(fromLocalPart) {
		return "", "", "", apperr.NewIncorrectInput("validation_failed",
			"the from-address local part is not valid")
	}
	return slug, fromName, fromLocalPart, nil
}

// NewSubscriptionPage builds a subscription page, rejecting any invariant
// violation. The database assigns the id and timestamps.
func NewSubscriptionPage(tenantID, slug, title string, targetListIDs []string, fields []FormField,
	sendingDomainID, fromName, fromLocalPart string) (*SubscriptionPage, error) {

	if tenantID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a tenant is required")
	}
	normSlug, normName, normLocal, err := validateSubscriptionPage(
		slug, title, sendingDomainID, fromName, fromLocalPart, targetListIDs)
	if err != nil {
		return nil, err
	}
	normFields, err := validateFields(fields)
	if err != nil {
		return nil, err
	}
	return &SubscriptionPage{
		tenantID:        tenantID,
		slug:            normSlug,
		title:           strings.TrimSpace(title),
		targetListIDs:   append([]string{}, targetListIDs...),
		fields:          normFields,
		sendingDomainID: sendingDomainID,
		fromName:        normName,
		fromLocalPart:   normLocal,
		active:          true,
	}, nil
}

// HydrateSubscriptionPage reconstructs a subscription page from a persisted
// row. Persistence only — it is not a constructor and performs no validation.
func HydrateSubscriptionPage(id, tenantID, slug, title string, targetListIDs []string,
	fields []FormField, sendingDomainID, fromName, fromLocalPart string, active bool,
	createdAt, updatedAt time.Time) *SubscriptionPage {

	return &SubscriptionPage{
		id:              id,
		tenantID:        tenantID,
		slug:            slug,
		title:           title,
		targetListIDs:   targetListIDs,
		fields:          fields,
		sendingDomainID: sendingDomainID,
		fromName:        fromName,
		fromLocalPart:   fromLocalPart,
		active:          active,
		createdAt:       createdAt,
		updatedAt:       updatedAt,
	}
}

// ID returns the page's database-assigned id.
func (p *SubscriptionPage) ID() string { return p.id }

// TenantID returns the owning tenant's id.
func (p *SubscriptionPage) TenantID() string { return p.tenantID }

// Slug returns the page's URL slug.
func (p *SubscriptionPage) Slug() string { return p.slug }

// Title returns the page title.
func (p *SubscriptionPage) Title() string { return p.title }

// TargetListIDs returns the lists a confirmed subscriber joins.
func (p *SubscriptionPage) TargetListIDs() []string { return p.targetListIDs }

// Fields returns the configured form fields.
func (p *SubscriptionPage) Fields() []FormField { return p.fields }

// SendingDomainID returns the sending domain the opt-in email is sent from.
func (p *SubscriptionPage) SendingDomainID() string { return p.sendingDomainID }

// FromName returns the opt-in email from-name.
func (p *SubscriptionPage) FromName() string { return p.fromName }

// FromLocalPart returns the opt-in email from-address local part.
func (p *SubscriptionPage) FromLocalPart() string { return p.fromLocalPart }

// Active reports whether the page is currently published.
func (p *SubscriptionPage) Active() bool { return p.active }

// CreatedAt returns when the page was created.
func (p *SubscriptionPage) CreatedAt() time.Time { return p.createdAt }

// UpdatedAt returns when the page was last changed.
func (p *SubscriptionPage) UpdatedAt() time.Time { return p.updatedAt }

// Reconfigure applies a full set of new attributes, rejecting any invariant
// violation. It is the single mutation entry point used by the update closure.
func (p *SubscriptionPage) Reconfigure(slug, title string, targetListIDs []string,
	fields []FormField, sendingDomainID, fromName, fromLocalPart string, active bool) error {

	normSlug, normName, normLocal, err := validateSubscriptionPage(
		slug, title, sendingDomainID, fromName, fromLocalPart, targetListIDs)
	if err != nil {
		return err
	}
	normFields, err := validateFields(fields)
	if err != nil {
		return err
	}
	p.slug = normSlug
	p.title = strings.TrimSpace(title)
	p.targetListIDs = append([]string{}, targetListIDs...)
	p.fields = normFields
	p.sendingDomainID = sendingDomainID
	p.fromName = normName
	p.fromLocalPart = normLocal
	p.active = active
	return nil
}

// SubscriptionPageRepository persists subscription pages. It is declared here,
// by the domain that depends on it; the pgx implementation lives in the
// adapters layer. Every operation runs inside a tenant-bound transaction.
type SubscriptionPageRepository interface {
	// Add persists a new page and returns its database-assigned id. It returns
	// ErrSubscriptionPageSlugTaken when the tenant already uses that slug.
	Add(ctx context.Context, tenantID string, p *SubscriptionPage) (string, error)
	// Update loads the page, runs fn, and persists the result. It returns
	// ErrSubscriptionPageNotFound when absent.
	Update(ctx context.Context, tenantID, id string,
		fn func(*SubscriptionPage) (*SubscriptionPage, error)) error
	// Get returns the page by id, or ErrSubscriptionPageNotFound.
	Get(ctx context.Context, tenantID, id string) (*SubscriptionPage, error)
	// GetBySlug returns the page by slug, or ErrSubscriptionPageNotFound.
	GetBySlug(ctx context.Context, tenantID, slug string) (*SubscriptionPage, error)
	// All returns every subscription page of the tenant.
	All(ctx context.Context, tenantID string) ([]*SubscriptionPage, error)
}

// SendingDomainChecker reports whether a sending domain belongs to a tenant,
// so a subscription page cannot be bound to another tenant's domain. It is
// declared here, by the use case that depends on it, and implemented by a
// bridge over the sending context's repository.
type SendingDomainChecker interface {
	// OwnedByTenant reports whether domainID is a sending domain of tenantID.
	OwnedByTenant(ctx context.Context, tenantID, domainID string) (bool, error)
}

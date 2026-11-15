package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBSubscriptionPages is the YDB-backed implementation of
// domain.SubscriptionPageRepository, for the serverless runtime. The target
// list ids and the form fields are JSON in Utf8 columns.
type YDBSubscriptionPages struct {
	driver *ydb.Driver
}

var _ domain.SubscriptionPageRepository = (*YDBSubscriptionPages)(nil)

// NewYDBSubscriptionPages builds a YDBSubscriptionPages repository over the
// driver.
func NewYDBSubscriptionPages(d *ydb.Driver) *YDBSubscriptionPages {
	return &YDBSubscriptionPages{driver: d}
}

// ydbSubscriptionPageColumns is the projection scanYDBSubscriptionPage reads,
// in order.
const ydbSubscriptionPageColumns = `id, tenant_id, slug, title, target_list_ids, fields,
    sending_domain_id, from_name, from_local_part, active, created_at, updated_at`

const ydbSelectSubscriptionPage = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbSubscriptionPageColumns + `
FROM subscription_pages
WHERE tenant_id = $tenant_id AND id = $id;`

// ydbSubscriptionPageWrite declares the parameters Add and Update bind from a
// page, in ydbSubscriptionPageParams order.
const ydbSubscriptionPageWrite = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $slug AS Utf8;
DECLARE $title AS Utf8;
DECLARE $target_list_ids AS Utf8;
DECLARE $fields AS Utf8;
DECLARE $sending_domain_id AS Utf8;
DECLARE $from_name AS Utf8;
DECLARE $from_local_part AS Utf8;
DECLARE $active AS Bool;
`

// Add persists a new page under a fresh UUID and returns the id. It returns
// domain.ErrSubscriptionPageSlugTaken when the tenant already uses the slug.
func (r *YDBSubscriptionPages) Add(ctx context.Context, tenantID string,
	p *domain.SubscriptionPage) (string, error) {

	id := uuid.NewString()
	params, err := ydbSubscriptionPageParams(tenantID, id, p)
	if err != nil {
		return "", err
	}
	err = r.driver.Query().Exec(ctx, ydbSubscriptionPageWrite+`INSERT INTO subscription_pages
    (tenant_id, id, slug, title, target_list_ids, fields, sending_domain_id,
     from_name, from_local_part, active, created_at, updated_at)
VALUES ($tenant_id, $id, $slug, $title, $target_list_ids, $fields, $sending_domain_id,
    $from_name, $from_local_part, $active, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		params)
	// Only the slug index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrSubscriptionPageSlugTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting subscription page: %w", err)
	}
	return id, nil
}

// Update loads the page, runs fn, and persists the result in one serializable
// transaction. It returns domain.ErrSubscriptionPageSlugTaken when the new
// slug belongs to another page of the tenant. The SDK retries the transaction
// on conflict, so fn may run more than once and must be retry-safe.
func (r *YDBSubscriptionPages) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.SubscriptionPage) (*domain.SubscriptionPage, error)) error {

	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbSubscriptionPageFromRow(tx.QueryRow(ctx, ydbSelectSubscriptionPage, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		params, err := ydbSubscriptionPageParams(tenantID, id, updated)
		if err != nil {
			return err
		}
		err = tx.Exec(ctx, ydbSubscriptionPageWrite+`UPDATE subscription_pages SET
    slug = $slug, title = $title, target_list_ids = $target_list_ids, fields = $fields,
    sending_domain_id = $sending_domain_id, from_name = $from_name,
    from_local_part = $from_local_part, active = $active, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`, params)
		// An UPDATE onto another page's slug hits the unique slug index at
		// the statement (spec "Plan B4 planning spike").
		if db.IsYDBConflict(err) {
			return domain.ErrSubscriptionPageSlugTaken
		}
		if err != nil {
			return fmt.Errorf("updating subscription page: %w", err)
		}
		return nil
	})
	// This covers a YDB that reports the collision at commit.
	if db.IsYDBConflict(err) {
		return domain.ErrSubscriptionPageSlugTaken
	}
	return err
}

// Get returns the page by id, or domain.ErrSubscriptionPageNotFound.
func (r *YDBSubscriptionPages) Get(ctx context.Context, tenantID, id string) (*domain.SubscriptionPage, error) {
	return ydbSubscriptionPageFromRow(r.driver.Query().QueryRow(ctx, ydbSelectSubscriptionPage, ydbTenantAndID(tenantID, id)))
}

// GetBySlug returns the page by slug, or domain.ErrSubscriptionPageNotFound.
func (r *YDBSubscriptionPages) GetBySlug(ctx context.Context, tenantID, slug string) (*domain.SubscriptionPage, error) {
	return ydbSubscriptionPageFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $slug AS Utf8;
SELECT `+ydbSubscriptionPageColumns+`
FROM subscription_pages VIEW idx_subscription_pages_slug
WHERE tenant_id = $tenant_id AND slug = $slug;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$slug").Text(slug).
			Build())))
}

// All returns every subscription page of the tenant, newest first. It is one
// statement outside a transaction, so nothing re-runs.
func (r *YDBSubscriptionPages) All(ctx context.Context, tenantID string) ([]*domain.SubscriptionPage, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbSubscriptionPageColumns+`
FROM subscription_pages
WHERE tenant_id = $tenant_id
ORDER BY created_at DESC;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing subscription pages: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var pages []*domain.SubscriptionPage
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing subscription pages: %w", err)
		}
		p, err := scanYDBSubscriptionPage(row)
		if err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, nil
}

// ydbSubscriptionPageParams binds every ydbSubscriptionPageWrite parameter.
func ydbSubscriptionPageParams(tenantID, id string, p *domain.SubscriptionPage) (query.ExecuteOption, error) {
	fields, err := json.Marshal(p.Fields())
	if err != nil {
		return nil, fmt.Errorf("encoding subscription page fields: %w", err)
	}
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Param("$slug").Text(p.Slug()).
		Param("$title").Text(p.Title()).
		Param("$target_list_ids").Text(ydbStrings(p.TargetListIDs())).
		Param("$fields").Text(string(fields)).
		Param("$sending_domain_id").Text(p.SendingDomainID()).
		Param("$from_name").Text(p.FromName()).
		Param("$from_local_part").Text(p.FromLocalPart()).
		Param("$active").Bool(p.Active()).
		Build()), nil
}

// ydbSubscriptionPageFromRow maps a single-row read to the page, translating
// "no row" into domain.ErrSubscriptionPageNotFound.
func ydbSubscriptionPageFromRow(row query.Row, err error) (*domain.SubscriptionPage, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrSubscriptionPageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading subscription page: %w", err)
	}
	return scanYDBSubscriptionPage(row)
}

// scanYDBSubscriptionPage reads one row in ydbSubscriptionPageColumns order.
func scanYDBSubscriptionPage(row query.Row) (*domain.SubscriptionPage, error) {
	var id, tenantID, slug, title, rawListIDs, rawFields, sendingDomainID, fromName, fromLocalPart string
	var active bool
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &slug, &title, &rawListIDs, &rawFields,
		&sendingDomainID, &fromName, &fromLocalPart, &active, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning subscription page: %w", err)
	}
	listIDs, err := ydbStringsFrom(rawListIDs)
	if err != nil {
		return nil, fmt.Errorf("decoding subscription page lists: %w", err)
	}
	var fields []domain.FormField
	if err := json.Unmarshal([]byte(rawFields), &fields); err != nil {
		return nil, fmt.Errorf("decoding subscription page fields: %w", err)
	}
	return domain.HydrateSubscriptionPage(id, tenantID, slug, title, listIDs, fields,
		sendingDomainID, fromName, fromLocalPart, active, createdAt, updatedAt), nil
}

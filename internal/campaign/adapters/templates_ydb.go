package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBTemplates is the YDB-backed implementation of domain.TemplateRepository,
// for the serverless runtime. YDB has no row-level security, so every
// statement filters by tenant_id explicitly.
type YDBTemplates struct {
	driver *ydb.Driver
}

var _ domain.TemplateRepository = (*YDBTemplates)(nil)

// NewYDBTemplates builds a YDBTemplates repository over the driver.
func NewYDBTemplates(d *ydb.Driver) *YDBTemplates {
	return &YDBTemplates{driver: d}
}

// ydbTemplateColumns is the projection scanYDBTemplate reads, in order.
const ydbTemplateColumns = `id, tenant_id, name, kind, subject, body_html, body_text, body_doc, theme, created_at, updated_at`

const ydbSelectTemplate = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbTemplateColumns + `
FROM templates
WHERE tenant_id = $tenant_id AND id = $id;`

// ydbTemplateWrite declares the parameters Add and Update bind from a
// template, in ydbTemplateParams order.
const ydbTemplateWrite = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $kind AS Utf8;
DECLARE $subject AS Utf8;
DECLARE $body_html AS Utf8;
DECLARE $body_text AS Utf8;
DECLARE $body_doc AS Optional<Utf8>;
DECLARE $theme AS Optional<Utf8>;
`

func ydbTemplateParams(tenantID, id string, t *domain.Template) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Param("$name").Text(t.Name()).
		Param("$kind").Text(string(t.Kind())).
		Param("$subject").Text(t.Subject()).
		Param("$body_html").Text(t.BodyHTML()).
		Param("$body_text").Text(t.BodyText()).
		Param("$body_doc").BeginOptional().Text(nullableJSONString(t.BodyDocJSON())).EndOptional().
		Param("$theme").BeginOptional().Text(nullableJSONString(t.ThemeJSON())).EndOptional().
		Build())
}

// Add persists a new template under a fresh UUID and returns the id. It
// returns domain.ErrTemplateNameTaken when the tenant already has a template
// with that name.
func (r *YDBTemplates) Add(ctx context.Context, tenantID string, t *domain.Template) (string, error) {
	id := uuid.NewString()
	err := r.driver.Query().Exec(ctx, ydbTemplateWrite+`INSERT INTO templates
    (tenant_id, id, name, kind, subject, body_html, body_text, body_doc, theme, created_at, updated_at)
VALUES ($tenant_id, $id, $name, $kind, $subject, $body_html, $body_text, $body_doc, $theme,
    CurrentUtcTimestamp(), CurrentUtcTimestamp());`, ydbTemplateParams(tenantID, id, t))
	// Only the name index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrTemplateNameTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting template: %w", err)
	}
	return id, nil
}

// Get returns the template, or domain.ErrTemplateNotFound.
func (r *YDBTemplates) Get(ctx context.Context, tenantID, id string) (*domain.Template, error) {
	return ydbTemplateFromRow(r.driver.Query().QueryRow(ctx, ydbSelectTemplate, ydbTenantAndID(tenantID, id)))
}

// Update loads the template, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so fn
// may run more than once and must be retry-safe.
func (r *YDBTemplates) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Template) (*domain.Template, error)) error {

	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbTemplateFromRow(tx.QueryRow(ctx, ydbSelectTemplate, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		err = tx.Exec(ctx, ydbTemplateWrite+`UPDATE templates SET
    name = $name, subject = $subject, body_html = $body_html, body_text = $body_text,
    body_doc = $body_doc, theme = $theme, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTemplateParams(tenantID, id, updated))
		if db.IsYDBConflict(err) {
			return domain.ErrTemplateNameTaken
		}
		if err != nil {
			return fmt.Errorf("updating template: %w", err)
		}
		return nil
	})
	// This covers a YDB that reports the collision at commit.
	if db.IsYDBConflict(err) {
		return domain.ErrTemplateNameTaken
	}
	return err
}

// All returns a page of the tenant's templates, ordered by name, and the total
// count, read in one transaction.
func (r *YDBTemplates) All(ctx context.Context, tenantID string, page domain.Page) ([]*domain.Template, int, error) {
	page = page.Normalize()
	var templates []*domain.Template
	var total int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		count, err := ydbCount(ctx, tx, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM templates WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("counting templates: %w", err)
		}
		// ponytail: OFFSET paging reads every skipped row (spec "Ceilings").
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT `+ydbTemplateColumns+`
FROM templates
WHERE tenant_id = $tenant_id
ORDER BY name
LIMIT $limit OFFSET $offset;`, ydbTenantAndPage(tenantID, page.Limit, page.Offset))
		if err != nil {
			return fmt.Errorf("listing templates: %w", err)
		}
		// Collected into a local and assigned at the end: the SDK may re-run
		// the closure, and a retry must not append to rows a failed run read.
		var local []*domain.Template
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing templates: %w", err)
			}
			t, err := scanYDBTemplate(row)
			if err != nil {
				return err
			}
			local = append(local, t)
		}
		templates, total = local, count
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return templates, total, nil
}

// Delete removes a template, or returns domain.ErrTemplateNotFound when no
// such template exists for the tenant. Postgres sets campaigns.template_id to
// NULL by foreign-key action, so the same transaction does it here: a campaign
// keeps its copied content and loses the link. transactional_messages keeps
// its template_id, which nothing reads.
func (r *YDBTemplates) Delete(ctx context.Context, tenantID, id string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM templates
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`, ydbTenantAndID(tenantID, id))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrTemplateNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting template: %w", err)
		}
		// ponytail: no index on campaigns.template_id, so this scans the
		// tenant's campaigns. Add a GLOBAL SYNC index on (tenant_id,
		// template_id) if a tenant outgrows that.
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
UPDATE campaigns SET template_id = NULL
WHERE tenant_id = $tenant_id AND template_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
			return fmt.Errorf("unlinking template from campaigns: %w", err)
		}
		return nil
	})
}

// ydbTemplateFromRow maps a single-row read to the template, translating "no
// row" into domain.ErrTemplateNotFound.
func ydbTemplateFromRow(row query.Row, err error) (*domain.Template, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrTemplateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading template: %w", err)
	}
	return scanYDBTemplate(row)
}

// scanYDBTemplate reads one row in ydbTemplateColumns order.
func scanYDBTemplate(row query.Row) (*domain.Template, error) {
	var id, tenantID, name, kind, subject, bodyHTML, bodyText string
	var bodyDoc, theme *string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &name, &kind, &subject, &bodyHTML, &bodyText,
		&bodyDoc, &theme, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning template: %w", err)
	}
	return domain.HydrateTemplate(id, tenantID, name, domain.Kind(kind), subject,
		bodyHTML, bodyText, jsonBytes(bodyDoc), jsonBytes(theme), createdAt, updatedAt), nil
}

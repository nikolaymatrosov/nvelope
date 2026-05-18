// Package adapters implements the campaign domain's interfaces against
// PostgreSQL, the Postbox client, and the Redis rate limiter. Every
// tenant-plane operation runs inside the shared RLS-bound transaction
// (internal/platform/tenantdb).
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Templates is the pgx-backed implementation of domain.TemplateRepository.
type Templates struct {
	pool *pgxpool.Pool
}

var _ domain.TemplateRepository = (*Templates)(nil)

// NewTemplates builds a Templates repository over the given pool.
func NewTemplates(pool *pgxpool.Pool) *Templates {
	return &Templates{pool: pool}
}

const templateColumns = "id, tenant_id, name, kind, subject, body_html, body_text, created_at, updated_at"

// scanTemplateRow reads one template row in templateColumns order.
func scanTemplateRow(row pgx.Row) (*domain.Template, error) {
	var id, tenantID, name, kind, subject, bodyHTML, bodyText string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &name, &kind, &subject, &bodyHTML, &bodyText,
		&createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return domain.HydrateTemplate(id, tenantID, name, domain.Kind(kind), subject,
		bodyHTML, bodyText, createdAt, updatedAt), nil
}

// Add persists a new template and returns its database-assigned id.
func (r *Templates) Add(ctx context.Context, tenantID string, t *domain.Template) (string, error) {
	var id string
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO templates (tenant_id, name, kind, subject, body_html, body_text)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			tenantID, t.Name(), string(t.Kind()), t.Subject(), t.BodyHTML(), t.BodyText()).Scan(&id)
		if db.IsUniqueViolation(err) {
			return domain.ErrTemplateNameTaken
		}
		if err != nil {
			return fmt.Errorf("inserting template: %w", err)
		}
		return nil
	})
	return id, err
}

// Get returns the template, or domain.ErrTemplateNotFound.
func (r *Templates) Get(ctx context.Context, tenantID, id string) (*domain.Template, error) {
	var out *domain.Template
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		t, err := r.getTx(ctx, tx, id)
		if err != nil {
			return err
		}
		out = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Update loads the template, runs fn, and persists the result.
func (r *Templates) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Template) (*domain.Template, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		loaded, err := r.getTx(ctx, tx, id)
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`UPDATE templates SET name = $1, subject = $2, body_html = $3, body_text = $4,
			    updated_at = now() WHERE id = $5`,
			updated.Name(), updated.Subject(), updated.BodyHTML(), updated.BodyText(), id)
		if db.IsUniqueViolation(err) {
			return domain.ErrTemplateNameTaken
		}
		if err != nil {
			return fmt.Errorf("updating template: %w", err)
		}
		return nil
	})
}

// All returns a page of the tenant's templates and the total count.
func (r *Templates) All(ctx context.Context, tenantID string, page domain.Page) ([]*domain.Template, int, error) {
	page = page.Normalize()
	var templates []*domain.Template
	var total int
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM templates").Scan(&total); err != nil {
			return fmt.Errorf("counting templates: %w", err)
		}
		rows, err := tx.Query(ctx,
			"SELECT "+templateColumns+" FROM templates ORDER BY name LIMIT $1 OFFSET $2",
			page.Limit, page.Offset)
		if err != nil {
			return fmt.Errorf("listing templates: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			t, err := scanTemplateRow(rows)
			if err != nil {
				return err
			}
			templates = append(templates, t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return templates, total, nil
}

func (r *Templates) getTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Template, error) {
	row := tx.QueryRow(ctx, "SELECT "+templateColumns+" FROM templates WHERE id = $1", id)
	t, err := scanTemplateRow(row)
	if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
		return nil, domain.ErrTemplateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading template: %w", err)
	}
	return t, nil
}

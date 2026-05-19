package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// SubscriptionPages is the pgx-backed implementation of
// domain.SubscriptionPageRepository.
type SubscriptionPages struct {
	pool *pgxpool.Pool
}

var _ domain.SubscriptionPageRepository = (*SubscriptionPages)(nil)

// NewSubscriptionPages builds a SubscriptionPages repository over the pool.
func NewSubscriptionPages(pool *pgxpool.Pool) *SubscriptionPages {
	return &SubscriptionPages{pool: pool}
}

const subscriptionPageColumns = "id, tenant_id, slug, title, target_list_ids, fields, " +
	"sending_domain_id, from_name, from_local_part, active, created_at, updated_at"

func scanSubscriptionPageRow(row pgx.Row) (*domain.SubscriptionPage, error) {
	var id, tenantID, slug, title, sendingDomainID, fromName, fromLocalPart string
	var listIDs []string
	var fieldsBytes []byte
	var active bool
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &slug, &title, &listIDs, &fieldsBytes,
		&sendingDomainID, &fromName, &fromLocalPart, &active, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var fields []domain.FormField
	if len(fieldsBytes) > 0 {
		if err := json.Unmarshal(fieldsBytes, &fields); err != nil {
			return nil, fmt.Errorf("decoding subscription page fields: %w", err)
		}
	}
	return domain.HydrateSubscriptionPage(id, tenantID, slug, title, listIDs, fields,
		sendingDomainID, fromName, fromLocalPart, active, createdAt, updatedAt), nil
}

// Add persists a new subscription page and returns its id.
func (r *SubscriptionPages) Add(ctx context.Context, tenantID string,
	p *domain.SubscriptionPage) (string, error) {

	fields, err := json.Marshal(p.Fields())
	if err != nil {
		return "", fmt.Errorf("encoding subscription page fields: %w", err)
	}
	var id string
	err = tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO subscription_pages
			   (tenant_id, slug, title, target_list_ids, fields, sending_domain_id,
			    from_name, from_local_part, active)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
			tenantID, p.Slug(), p.Title(), p.TargetListIDs(), fields, p.SendingDomainID(),
			p.FromName(), p.FromLocalPart(), p.Active()).Scan(&id)
		if db.IsUniqueViolation(err) {
			return domain.ErrSubscriptionPageSlugTaken
		}
		if err != nil {
			return fmt.Errorf("inserting subscription page: %w", err)
		}
		return nil
	})
	return id, err
}

// Update loads the page, runs fn, and persists the result.
func (r *SubscriptionPages) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.SubscriptionPage) (*domain.SubscriptionPage, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		loaded, err := r.getTx(ctx, tx, "id = $1", id)
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		fields, err := json.Marshal(updated.Fields())
		if err != nil {
			return fmt.Errorf("encoding subscription page fields: %w", err)
		}
		_, err = tx.Exec(ctx,
			`UPDATE subscription_pages SET slug = $1, title = $2, target_list_ids = $3,
			        fields = $4, sending_domain_id = $5, from_name = $6, from_local_part = $7,
			        active = $8, updated_at = now() WHERE id = $9`,
			updated.Slug(), updated.Title(), updated.TargetListIDs(), fields,
			updated.SendingDomainID(), updated.FromName(), updated.FromLocalPart(),
			updated.Active(), id)
		if db.IsUniqueViolation(err) {
			return domain.ErrSubscriptionPageSlugTaken
		}
		if err != nil {
			return fmt.Errorf("updating subscription page: %w", err)
		}
		return nil
	})
}

// Get returns the page by id.
func (r *SubscriptionPages) Get(ctx context.Context, tenantID, id string) (*domain.SubscriptionPage, error) {
	return r.lookup(ctx, tenantID, "id = $1", id)
}

// GetBySlug returns the page by slug.
func (r *SubscriptionPages) GetBySlug(ctx context.Context, tenantID, slug string) (*domain.SubscriptionPage, error) {
	return r.lookup(ctx, tenantID, "slug = $1", slug)
}

func (r *SubscriptionPages) lookup(ctx context.Context, tenantID, where, arg string) (*domain.SubscriptionPage, error) {
	var out *domain.SubscriptionPage
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		p, err := r.getTx(ctx, tx, where, arg)
		if err != nil {
			return err
		}
		out = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *SubscriptionPages) getTx(ctx context.Context, tx pgx.Tx, where, arg string) (*domain.SubscriptionPage, error) {
	row := tx.QueryRow(ctx, "SELECT "+subscriptionPageColumns+" FROM subscription_pages WHERE "+where, arg)
	p, err := scanSubscriptionPageRow(row)
	if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
		return nil, domain.ErrSubscriptionPageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading subscription page: %w", err)
	}
	return p, nil
}

// All returns every subscription page of the tenant.
func (r *SubscriptionPages) All(ctx context.Context, tenantID string) ([]*domain.SubscriptionPage, error) {
	var pages []*domain.SubscriptionPage
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT "+subscriptionPageColumns+" FROM subscription_pages ORDER BY created_at DESC")
		if err != nil {
			return fmt.Errorf("listing subscription pages: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanSubscriptionPageRow(rows)
			if err != nil {
				return err
			}
			pages = append(pages, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return pages, nil
}

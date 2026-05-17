// Package adapters implements the audience domain's repository interfaces
// against PostgreSQL. Every tenant-plane operation runs inside the shared
// RLS-bound transaction (internal/platform/tenantdb).
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Lists is the pgx-backed implementation of domain.ListRepository.
type Lists struct {
	pool *pgxpool.Pool
}

var _ domain.ListRepository = (*Lists)(nil)

// NewLists builds a Lists repository over the given pool.
func NewLists(pool *pgxpool.Pool) *Lists {
	return &Lists{pool: pool}
}

const listColumns = "id, tenant_id, name, description, visibility, optin, tags, created_at, updated_at"

// scanListRow reads one list row in listColumns order.
func scanListRow(row pgx.Row) (*domain.List, error) {
	var id, tenantID, name, description, visibility, optin string
	var tags []string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &name, &description, &visibility, &optin,
		&tags, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return domain.HydrateList(id, tenantID, name, description,
		domain.Visibility(visibility), domain.OptIn(optin), tags, createdAt, updatedAt), nil
}

// Add persists a new list and returns its database-assigned id.
func (r *Lists) Add(ctx context.Context, tenantID string, l *domain.List) (string, error) {
	var id string
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO lists (tenant_id, name, description, visibility, optin, tags)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			tenantID, l.Name(), l.Description(), string(l.Visibility()), string(l.OptIn()), l.Tags()).
			Scan(&id)
		if db.IsUniqueViolation(err) {
			return domain.ErrListNameTaken
		}
		if err != nil {
			return fmt.Errorf("inserting list: %w", err)
		}
		return nil
	})
	return id, err
}

// Update loads the list, runs fn, and persists the result.
func (r *Lists) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.List) (*domain.List, error)) error {

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
			`UPDATE lists SET name = $1, description = $2, visibility = $3, optin = $4,
			        tags = $5, updated_at = now() WHERE id = $6`,
			updated.Name(), updated.Description(), string(updated.Visibility()),
			string(updated.OptIn()), updated.Tags(), id)
		if db.IsUniqueViolation(err) {
			return domain.ErrListNameTaken
		}
		if err != nil {
			return fmt.Errorf("updating list: %w", err)
		}
		return nil
	})
}

// Delete removes the list and cascades its memberships.
func (r *Lists) Delete(ctx context.Context, tenantID, id string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM lists WHERE id = $1", id)
		if db.IsInvalidInput(err) {
			return domain.ErrListNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting list: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrListNotFound
		}
		return nil
	})
}

// Get returns the list, or domain.ErrListNotFound.
func (r *Lists) Get(ctx context.Context, tenantID, id string) (*domain.List, error) {
	var out *domain.List
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		l, err := r.getTx(ctx, tx, id)
		if err != nil {
			return err
		}
		out = l
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// All returns a page of the tenant's lists and the total count.
func (r *Lists) All(ctx context.Context, tenantID string, page domain.Page) ([]*domain.List, int, error) {
	page = page.Normalize()
	var lists []*domain.List
	var total int
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM lists").Scan(&total); err != nil {
			return fmt.Errorf("counting lists: %w", err)
		}
		rows, err := tx.Query(ctx,
			"SELECT "+listColumns+" FROM lists ORDER BY name LIMIT $1 OFFSET $2",
			page.Limit, page.Offset)
		if err != nil {
			return fmt.Errorf("listing lists: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			l, err := scanListRow(rows)
			if err != nil {
				return err
			}
			lists = append(lists, l)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return lists, total, nil
}

func (r *Lists) getTx(ctx context.Context, tx pgx.Tx, id string) (*domain.List, error) {
	row := tx.QueryRow(ctx, "SELECT "+listColumns+" FROM lists WHERE id = $1", id)
	l, err := scanListRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrListNotFound
	}
	if db.IsInvalidInput(err) {
		return nil, domain.ErrListNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading list: %w", err)
	}
	return l, nil
}

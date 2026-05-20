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

// Fields is the pgx-backed implementation of domain.FieldRepository over the
// subscriber_fields table from migration 000020.
type Fields struct {
	pool *pgxpool.Pool
}

var _ domain.FieldRepository = (*Fields)(nil)

// NewFields builds a Fields repository over the given pool.
func NewFields(pool *pgxpool.Pool) *Fields {
	return &Fields{pool: pool}
}

const fieldColumns = "id, tenant_id, slug, display_name, type, default_value, position, created_at, updated_at"

func scanFieldRow(row pgx.Row) (*domain.Field, error) {
	var id, tenantID, slug, displayName, fieldType, defaultValue string
	var position int
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &slug, &displayName, &fieldType,
		&defaultValue, &position, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return domain.HydrateField(id, tenantID, slug, displayName,
		domain.FieldType(fieldType), defaultValue, position, false, createdAt, updatedAt), nil
}

// Add persists a new field and returns its database-assigned id.
func (r *Fields) Add(ctx context.Context, tenantID string, f *domain.Field) (string, error) {
	if domain.IsBuiltinFieldSlug(f.Slug()) {
		return "", domain.ErrFieldBuiltinSlug
	}
	var id string
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO subscriber_fields
			    (tenant_id, slug, display_name, type, default_value, position)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			tenantID, f.Slug(), f.DisplayName(), string(f.Type()),
			f.DefaultValue(), f.Position()).Scan(&id)
		if db.IsUniqueViolation(err) {
			return domain.ErrFieldSlugTaken
		}
		if err != nil {
			return fmt.Errorf("inserting subscriber field: %w", err)
		}
		return nil
	})
	return id, err
}

// Get returns the field, or domain.ErrFieldNotFound.
func (r *Fields) Get(ctx context.Context, tenantID, id string) (*domain.Field, error) {
	var out *domain.Field
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		f, err := r.getTx(ctx, tx, id)
		if err != nil {
			return err
		}
		out = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Update loads the field, runs fn, and persists the result.
func (r *Fields) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Field) (*domain.Field, error)) error {

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
			`UPDATE subscriber_fields
			    SET display_name = $1, type = $2, default_value = $3,
			        position = $4, updated_at = now()
			  WHERE id = $5`,
			updated.DisplayName(), string(updated.Type()), updated.DefaultValue(),
			updated.Position(), id)
		if err != nil {
			return fmt.Errorf("updating subscriber field: %w", err)
		}
		return nil
	})
}

// Delete removes the field, or returns domain.ErrFieldNotFound.
func (r *Fields) Delete(ctx context.Context, tenantID, id string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM subscriber_fields WHERE id = $1", id)
		if db.IsInvalidInput(err) {
			return domain.ErrFieldNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting subscriber field: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrFieldNotFound
		}
		return nil
	})
}

// All returns every field for the tenant ordered by position.
func (r *Fields) All(ctx context.Context, tenantID string) ([]*domain.Field, error) {
	var fields []*domain.Field
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT "+fieldColumns+" FROM subscriber_fields ORDER BY position, created_at")
		if err != nil {
			return fmt.Errorf("listing subscriber fields: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			f, err := scanFieldRow(rows)
			if err != nil {
				return err
			}
			fields = append(fields, f)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return fields, nil
}

// Reorder atomically updates the position of every field id in positions.
// Callers (the ReorderFields command handler) supply a complete map.
func (r *Fields) Reorder(ctx context.Context, tenantID string, positions map[string]int) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		for id, pos := range positions {
			tag, err := tx.Exec(ctx,
				`UPDATE subscriber_fields SET position = $1, updated_at = now() WHERE id = $2`,
				pos, id)
			if err != nil {
				return fmt.Errorf("reordering subscriber field %s: %w", id, err)
			}
			if tag.RowsAffected() == 0 {
				return domain.ErrFieldNotFound
			}
		}
		return nil
	})
}

func (r *Fields) getTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Field, error) {
	row := tx.QueryRow(ctx,
		"SELECT "+fieldColumns+" FROM subscriber_fields WHERE id = $1", id)
	f, err := scanFieldRow(row)
	if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
		return nil, domain.ErrFieldNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading subscriber field: %w", err)
	}
	return f, nil
}

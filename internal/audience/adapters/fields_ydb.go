package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBFields is the YDB-backed implementation of domain.FieldRepository, for
// the serverless runtime. Built-in pseudo-rows are never persisted, as on
// Postgres.
type YDBFields struct {
	driver *ydb.Driver
}

var _ domain.FieldRepository = (*YDBFields)(nil)

// NewYDBFields builds a YDBFields repository over the driver.
func NewYDBFields(d *ydb.Driver) *YDBFields {
	return &YDBFields{driver: d}
}

// ydbFieldColumns is the projection scanYDBField reads, in order.
const ydbFieldColumns = `id, tenant_id, slug, display_name, type, default_value, position, created_at, updated_at`

const ydbSelectField = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbFieldColumns + `
FROM subscriber_fields
WHERE tenant_id = $tenant_id AND id = $id;`

// Add persists a new field under a fresh UUID and returns the id. It returns
// domain.ErrFieldBuiltinSlug for a built-in slug and domain.ErrFieldSlugTaken
// when the tenant already has a field with that slug.
func (r *YDBFields) Add(ctx context.Context, tenantID string, f *domain.Field) (string, error) {
	if domain.IsBuiltinFieldSlug(f.Slug()) {
		return "", domain.ErrFieldBuiltinSlug
	}
	id := uuid.NewString()
	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $slug AS Utf8;
DECLARE $display_name AS Utf8;
DECLARE $type AS Utf8;
DECLARE $default_value AS Utf8;
DECLARE $position AS Int64;
INSERT INTO subscriber_fields
    (tenant_id, id, slug, display_name, type, default_value, position, created_at, updated_at)
VALUES ($tenant_id, $id, $slug, $display_name, $type, $default_value, $position,
    CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$slug").Text(f.Slug()).
			Param("$display_name").Text(f.DisplayName()).
			Param("$type").Text(string(f.Type())).
			Param("$default_value").Text(f.DefaultValue()).
			Param("$position").Int64(int64(f.Position())).
			Build()))
	// Only the slug index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrFieldSlugTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting subscriber field: %w", err)
	}
	return id, nil
}

// Get returns the field, or domain.ErrFieldNotFound.
func (r *YDBFields) Get(ctx context.Context, tenantID, id string) (*domain.Field, error) {
	return ydbFieldFromRow(r.driver.Query().QueryRow(ctx, ydbSelectField, ydbTenantAndID(tenantID, id)))
}

// Update loads the field, runs fn, and persists the result in one serializable
// transaction. The slug is immutable, as on Postgres. The SDK retries the
// transaction on conflict, so fn may run more than once and must be
// retry-safe.
func (r *YDBFields) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Field) (*domain.Field, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbFieldFromRow(tx.QueryRow(ctx, ydbSelectField, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $display_name AS Utf8;
DECLARE $type AS Utf8;
DECLARE $default_value AS Utf8;
DECLARE $position AS Int64;
UPDATE subscriber_fields SET display_name = $display_name, type = $type,
    default_value = $default_value, position = $position, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$display_name").Text(updated.DisplayName()).
				Param("$type").Text(string(updated.Type())).
				Param("$default_value").Text(updated.DefaultValue()).
				Param("$position").Int64(int64(updated.Position())).
				Build())); err != nil {
			return fmt.Errorf("updating subscriber field: %w", err)
		}
		return nil
	})
}

// Delete removes the field. It returns domain.ErrFieldNotFound when absent.
func (r *YDBFields) Delete(ctx context.Context, tenantID, id string) error {
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM subscriber_fields
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`, ydbTenantAndID(tenantID, id))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrFieldNotFound
	}
	if err != nil {
		return fmt.Errorf("deleting subscriber field: %w", err)
	}
	return nil
}

// All returns every field of the tenant in display order: position, then
// creation time.
func (r *YDBFields) All(ctx context.Context, tenantID string) ([]*domain.Field, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbFieldColumns+`
FROM subscriber_fields
WHERE tenant_id = $tenant_id
ORDER BY position, created_at;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing subscriber fields: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var fields []*domain.Field
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing subscriber fields: %w", err)
		}
		f, err := scanYDBField(row)
		if err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, nil
}

// Reorder sets the position of every given field in one transaction. It
// returns domain.ErrFieldNotFound, and writes nothing, when an id is not a
// field of the tenant.
func (r *YDBFields) Reorder(ctx context.Context, tenantID string, positions map[string]int) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		for id, pos := range positions {
			_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $position AS Int64;
UPDATE subscriber_fields SET position = $position, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$id").Text(id).
					Param("$position").Int64(int64(pos)).
					Build()))
			if errors.Is(err, query.ErrNoRows) {
				return domain.ErrFieldNotFound
			}
			if err != nil {
				return fmt.Errorf("reordering subscriber field %s: %w", id, err)
			}
		}
		return nil
	})
}

// ydbFieldFromRow maps a single-row read to the field, translating "no row"
// into domain.ErrFieldNotFound.
func ydbFieldFromRow(row query.Row, err error) (*domain.Field, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrFieldNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading subscriber field: %w", err)
	}
	return scanYDBField(row)
}

// scanYDBField reads one row in ydbFieldColumns order.
func scanYDBField(row query.Row) (*domain.Field, error) {
	var id, tenantID, slug, displayName, fieldType, defaultValue string
	var position int64
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &slug, &displayName, &fieldType, &defaultValue,
		&position, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning subscriber field: %w", err)
	}
	return domain.HydrateField(id, tenantID, slug, displayName, domain.FieldType(fieldType),
		defaultValue, int(position), false, createdAt, updatedAt), nil
}

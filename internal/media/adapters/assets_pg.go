// Package adapters wires the media context's domain interfaces to concrete
// infrastructure: a pgx-backed metadata repository, an S3-compatible blob
// store, and an in-memory blob-store fake for use-case tests.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Assets is the pgx-backed implementation of domain.MediaRepository.
type Assets struct {
	pool *pgxpool.Pool
}

var _ domain.MediaRepository = (*Assets)(nil)

// NewAssets builds an Assets repository over the pool.
func NewAssets(pool *pgxpool.Pool) *Assets {
	return &Assets{pool: pool}
}

const assetColumns = "id, tenant_id, filename, content_type, size_bytes, " +
	"storage_key, public_url, COALESCE(uploaded_by::text, ''), created_at"

func scanAssetRow(row pgx.Row) (*domain.MediaAsset, error) {
	var id, tenantID, filename, contentType, storageKey, publicURL, uploadedBy string
	var sizeBytes int64
	var createdAt time.Time
	if err := row.Scan(&id, &tenantID, &filename, &contentType, &sizeBytes,
		&storageKey, &publicURL, &uploadedBy, &createdAt); err != nil {
		return nil, err
	}
	return domain.HydrateMediaAsset(id, tenantID, filename, contentType, sizeBytes,
		storageKey, publicURL, uploadedBy, createdAt), nil
}

// Add persists a new media-asset row. The id is set by the caller so it can
// be embedded in the storage key before the bytes are written.
func (r *Assets) Add(ctx context.Context, a *domain.MediaAsset) error {
	return tenantdb.WithTenant(ctx, r.pool, a.TenantID(), func(ctx context.Context, tx pgx.Tx) error {
		var uploadedBy any
		if a.UploadedBy() != "" {
			uploadedBy = a.UploadedBy()
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO media_assets
			   (id, tenant_id, filename, content_type, size_bytes,
			    storage_key, public_url, uploaded_by)
			 VALUES (@id, @tenant_id, @filename, @content_type, @size_bytes,
			         @storage_key, @public_url, @uploaded_by)`,
			pgx.NamedArgs{
				"id":           a.ID(),
				"tenant_id":    a.TenantID(),
				"filename":     a.Filename(),
				"content_type": a.ContentType(),
				"size_bytes":   a.SizeBytes(),
				"storage_key":  a.StorageKey(),
				"public_url":   a.PublicURL(),
				"uploaded_by":  uploadedBy,
			})
		if err != nil {
			return fmt.Errorf("inserting media asset: %w", err)
		}
		return nil
	})
}

// Get returns the asset by id.
func (r *Assets) Get(ctx context.Context, tenantID, id string) (*domain.MediaAsset, error) {
	var out *domain.MediaAsset
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"SELECT "+assetColumns+" FROM media_assets WHERE id = $1", id)
		a, err := scanAssetRow(row)
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrMediaNotFound
		}
		if err != nil {
			return fmt.Errorf("loading media asset: %w", err)
		}
		out = a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// List returns the tenant's media assets, newest first.
func (r *Assets) List(ctx context.Context, tenantID string) ([]*domain.MediaAsset, error) {
	var assets []*domain.MediaAsset
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT "+assetColumns+" FROM media_assets ORDER BY created_at DESC")
		if err != nil {
			return fmt.Errorf("listing media assets: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAssetRow(rows)
			if err != nil {
				return err
			}
			assets = append(assets, a)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return assets, nil
}

// Delete removes the metadata row, returning ErrMediaNotFound when absent.
func (r *Assets) Delete(ctx context.Context, tenantID, id string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM media_assets WHERE id = $1", id)
		if db.IsInvalidInput(err) {
			return domain.ErrMediaNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting media asset: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrMediaNotFound
		}
		return nil
	})
}

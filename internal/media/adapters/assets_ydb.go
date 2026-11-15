package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// YDBAssets is the YDB-backed implementation of domain.MediaRepository, for
// the serverless runtime. YDB has no row-level security, so every statement
// filters by tenant_id explicitly.
type YDBAssets struct {
	driver *ydb.Driver
}

var _ domain.MediaRepository = (*YDBAssets)(nil)

// NewYDBAssets builds a YDBAssets repository over the driver.
func NewYDBAssets(d *ydb.Driver) *YDBAssets {
	return &YDBAssets{driver: d}
}

const ydbAssetColumns = `id, tenant_id, filename, content_type, size_bytes, width, height,
    storage_key, uploaded_by, published_at, created_at`

func ydbTenantAndID(tenantID, id string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Build())
}

func scanYDBAsset(row query.Row) (*domain.MediaAsset, error) {
	var id, tenantID, filename, contentType, storageKey string
	var sizeBytes int64
	var width, height int32
	var uploadedBy *string
	var publishedAt *time.Time
	var createdAt time.Time
	if err := row.Scan(&id, &tenantID, &filename, &contentType, &sizeBytes, &width, &height,
		&storageKey, &uploadedBy, &publishedAt, &createdAt); err != nil {
		return nil, fmt.Errorf("scanning media asset: %w", err)
	}
	by := ""
	if uploadedBy != nil {
		by = *uploadedBy
	}
	return domain.HydrateMediaAsset(id, tenantID, filename, contentType, sizeBytes,
		int(width), int(height), storageKey, by, publishedAt, createdAt), nil
}

// Add persists a new asset row. The id is set by the caller, so a second Add
// of the same id fails with the key conflict.
func (r *YDBAssets) Add(ctx context.Context, a *domain.MediaAsset) error {
	var uploadedBy *string
	if a.UploadedBy() != "" {
		s := a.UploadedBy()
		uploadedBy = &s
	}
	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $filename AS Utf8;
DECLARE $content_type AS Utf8;
DECLARE $size_bytes AS Int64;
DECLARE $width AS Int32;
DECLARE $height AS Int32;
DECLARE $storage_key AS Utf8;
DECLARE $uploaded_by AS Optional<Utf8>;
INSERT INTO media_assets
    (tenant_id, id, filename, content_type, size_bytes, width, height,
     storage_key, uploaded_by, created_at)
VALUES ($tenant_id, $id, $filename, $content_type, $size_bytes, $width, $height,
        $storage_key, $uploaded_by, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(a.TenantID()).
			Param("$id").Text(a.ID()).
			Param("$filename").Text(a.Filename()).
			Param("$content_type").Text(a.ContentType()).
			Param("$size_bytes").Int64(a.SizeBytes()).
			Param("$width").Int32(int32(a.Width())).
			Param("$height").Int32(int32(a.Height())).
			Param("$storage_key").Text(a.StorageKey()).
			Param("$uploaded_by").BeginOptional().Text(uploadedBy).EndOptional().
			Build()))
	if err != nil {
		return fmt.Errorf("inserting media asset: %w", err)
	}
	return nil
}

// Get returns the asset by id, or domain.ErrMediaNotFound.
func (r *YDBAssets) Get(ctx context.Context, tenantID, id string) (*domain.MediaAsset, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT `+ydbAssetColumns+`
FROM media_assets WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id))
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrMediaNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading media asset: %w", err)
	}
	return scanYDBAsset(row)
}

// List returns the tenant's assets, newest first.
//
// ponytail: reads and sorts every asset of the tenant (the port has no
// paging). Fine for one organisation's library; the upgrade path is a
// GLOBAL SYNC index on (tenant_id, created_at) and keyset paging.
func (r *YDBAssets) List(ctx context.Context, tenantID string) ([]*domain.MediaAsset, error) {
	var out []*domain.MediaAsset
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbAssetColumns+`
FROM media_assets WHERE tenant_id = $tenant_id
ORDER BY created_at DESC, id DESC;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("listing media assets: %w", err)
		}
		// Collected into a local and assigned at the end: the SDK may re-run
		// the closure, and a retry must not append to rows a failed run read.
		var local []*domain.MediaAsset
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing media assets: %w", err)
			}
			a, err := scanYDBAsset(row)
			if err != nil {
				return err
			}
			local = append(local, a)
		}
		out = local
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Delete removes the row, or returns domain.ErrMediaNotFound when absent.
func (r *YDBAssets) Delete(ctx context.Context, tenantID, id string) error {
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM media_assets WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`, ydbTenantAndID(tenantID, id))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrMediaNotFound
	}
	if err != nil {
		return fmt.Errorf("deleting media asset: %w", err)
	}
	return nil
}

// MarkPublished records that the asset's public copy exists, keeping the
// first timestamp when one is already set.
func (r *YDBAssets) MarkPublished(ctx context.Context, tenantID, id string, at time.Time) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT published_at FROM media_assets WHERE tenant_id = $tenant_id AND id = $id;`,
			ydbTenantAndID(tenantID, id))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrMediaNotFound
		}
		if err != nil {
			return fmt.Errorf("loading media asset: %w", err)
		}
		var publishedAt *time.Time
		if err := row.Scan(&publishedAt); err != nil {
			return fmt.Errorf("scanning media asset: %w", err)
		}
		if publishedAt != nil {
			return nil
		}
		err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $at AS Timestamp;
UPDATE media_assets SET published_at = $at WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$at").Timestamp(at).
				Build()))
		if err != nil {
			return fmt.Errorf("marking media asset published: %w", err)
		}
		return nil
	})
}

func ydbTenantParam(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

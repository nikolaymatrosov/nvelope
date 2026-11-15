# Serverless On-Prem — Plan B9 (media on YDB) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- Port the `media` context to YDB: `YDBAssets` behind the existing `domain.MediaRepository` port, held to the same contract tests as the Postgres `Assets`.
- Prove that the media use cases the campaign closures call (`PublishAssets`, `DeleteAsset`) behave the same over both repositories, including the repeated run a YDB retry causes.

**Architecture:**
- **Migration.** One file, `internal/db/ydbmigrations/000010_media.up.sql`, creates `media_assets`, keyed `(tenant_id, id)`, no secondary index.
- **Adapter.** `YDBAssets` sits next to the pg `Assets` in `internal/media/adapters` and calls `db.Query()` / `DoTx` directly, like the B5 to B8 adapters. It owns two small helpers (`ydbTenantAndID`, `ydbTenantParam`); the package has no `ydb_common.go` because nothing else in it needs YDB.
- **Blob store.** Unchanged: `S3BlobStore` serves both runtimes, `MemoryBlobStore` serves the tests. The media repository makes no S3 call inside its own `DoTx`; the campaign `Update` closures' publish copy is the V6 exception, which `TestPublishAssetsIsRepeatable` covers.
- **Contract tests.** `assets_pg_test.go` becomes `assets_test.go`, a pg+ydb suite through an `eachBackend` harness (a `backend` struct with `assets` and `newTenant`), as in campaign and deliverability. `usecases_test.go` runs the publish and delete handlers over both.

**Tech Stack:**
- Go 1.26
- `github.com/ydb-platform/ydb-go-sdk/v3` v3.136.4 (query service)
- testcontainers-go, local-ydb `26.3.1.23`
- testify, `github.com/google/uuid`

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md).
- Binding sections:
  - "Storage layer (YDB)": Transactions; Schema and migrations; SQL translation rules, including the rows Plan B9 adds.
  - "Spike findings (Plan A)": V5 and V6.
  - "Plan B2 planning spike" to "Plan B9 planning spike", the last of which holds this plan's decisions and the Plan C4 hand-off.
  - "Ceilings".
- **Media feature:** [docs/superpowers/specs/2026-11-14-media-library-design.md](../specs/2026-11-14-media-library-design.md). Its user decisions stay intact: deleting a library asset keeps its public copy unless the caller asks otherwise; no SVG; library URLs on the app origin with a 302 to a presigned URL.
- **Dry run.** During planning, every file in this plan was applied stage by stage to the worktree, then reverted. Stage N is Task N below.
  - After each stage, `go build ./...`, `go vet`, `golangci-lint run` and the media suites passed. Stage 1 also ran `internal/db`.
  - Sabotage check: with the `tenant_id` predicate removed from `Get` and from the `MarkPublished` read, `TestAssetsAreTenantScoped/ydb` and `TestPublishAssetsFailsOnAnotherTenantsAsset/ydb` fail.
  - A throwaway race test (six `MarkPublished` and six `Add` calls at once, three runs) passed; it is not kept.
  - On the final state: 
    - `go build ./... && go vet ./... && go test ./...` had no failing package;
    - `golangci-lint run` reported `0 issues.`;
    - `make lint-arch` reported `[arch] clean`;
    - `go.mod` and `go.sum` were unchanged.

**Scope:**
- This is the ninth and last Plan B plan.
- Not in scope:
  - Wiring `NewYDBAssets` into the composition root. That is Plan C4 (see "Done criteria").
  - The Postgres adapter, the blob stores, the HTTP handlers and the media use cases: unchanged.
  - `S3BlobStore.Publish` sending `ACL: public-read` (see "Found, not fixed" in the spec's B9 section).

## Global Constraints

- **Branch.**
  - Work on `feat/serverless-onprem` in the worktree `/Users/nikthespirit/experiments/nvelope-wt/serverless-onprem`. Both already exist, and the branch starts at `80208d3` plus the commit that adds this plan.
  - Do not create branches or worktrees. Do not push or merge.
  - The main checkout belongs to other sessions.
  - Never use bare `git stash` or `git stash pop`; the stash is shared.
- **Docker.**
  - Docker on this machine is OrbStack. Prefix every `go test` / `make test` / `make ci` with `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && `.
  - Ryuk reaps test containers shortly after a run ends. A run that starts within that window can hang or fail with `No such container`; kill it (`pkill -f adapters.test`) and re-run.
  - The shared containers `nvelope-test-pg`, `nvelope-test-ydb`, `nvelope-test-s3-strict` and `nvelope-test-sqs` may belong to another session's live run. Never `docker rm` them.
- **YDB container.** It is `ydbplatform/local-ydb:26.3.1.23`, amd64 under Rosetta. A fresh container can take about 45 s to start, so use `-timeout 8m` on runs that touch YDB, and `-timeout 15m` for the full suite.
- **Known environmental flakes. None of these is caused by this plan:**
  - `internal/platform/ratelimit` `TestAllowIsConsistentUnderConcurrency` can fail with `redis eval failed: i/o timeout` under full-suite load. Re-run that package alone.
  - pg-backed packages can all fail at once with `no migration found for version N`, or with `unexpected postmaster exit`. That means another worktree's run is using the shared `nvelope-test-pg`. Re-run once it is gone.
  - `internal/api` `TestPublicSubscriptionCrossTenantIsolation` can fail on a reused pg database (a fixed `hash-a` token), `TestEmailVerificationsDeleteExpiredBefore` on a duplicate token hash, and `TestTransactionalSendWithValidKey` with a 500. Re-run once.
- **Dependencies.** No new Go modules and no `go.mod` change.
- **SaaS behaviour.** Postgres adapters, River, Redis and every existing test keep their behaviour. `go test ./...` passes after every task.
- **YQL conventions** (spec "SQL translation rules" and V4):
  - Every query `DECLARE`s each parameter: `DECLARE $id AS Utf8;`. A nullable column's parameter is `Optional<...>` and is bound with `BeginOptional()...EndOptional()` from a pointer.
  - Literals that target a `Utf8` column take the `u` suffix; a bare `"..."` is `String`.
  - Every DDL statement is idempotent (`CREATE TABLE IF NOT EXISTS`). A statement ends on a line ending in `;`. Never end a line with `;` inside a string literal or a `--` comment.
  - An `Int64` column scans into `int64`, an `Int32` into `int32`, not `int`.
- **Tenant scoping.** The table's primary key starts with `tenant_id`, and every query filters by the `tenantID` the port receives.
- **No column defaults, no CHECK constraints, no foreign keys.** Every `INSERT` writes every `NOT NULL` column.
- **`DoTx` closures must be retry-safe** (V6):
  - only database operations inside;
  - values the caller reads are assigned at the end of the closure, never accumulated across runs;
  - never `append` to a slice, or write to a map, declared outside the closure.
- **Commits.** Use conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.**
  - `goimports` is not installed, so use `gofmt -w`.
  - `golangci-lint` is on PATH. It lints test files too.
  - Run arch lint as `PATH="$HOME/go/bin:$PATH" make lint-arch`, with PATH set inline. Run git commands one per line, as the commit steps show.

## File structure

| File | Responsibility |
|------|----------------|
| `internal/db/ydbmigrations/000010_media.up.sql` | the `media_assets` table |
| `internal/media/adapters/assets_ydb.go` | `YDBAssets` |
| `internal/media/adapters/assets_test.go` | the `backend` harness and the pg+ydb contract suite (was `assets_pg_test.go`) |
| `internal/media/adapters/usecases_test.go` | `PublishAssets` and `DeleteAsset` over both repositories |

---

### Task 1: Migration and `YDBAssets`, with the repository contract suite on both backends

**Files:**
- Create: `internal/db/ydbmigrations/000010_media.up.sql`
- Create: `internal/media/adapters/assets_ydb.go`
- Rename and rewrite: `internal/media/adapters/assets_pg_test.go` -> `assets_test.go`

**Interfaces:**
- Consumes:
  - The port `domain.MediaRepository` (`internal/media/domain/repository.go`), unchanged.
  - `domain.HydrateMediaAsset`, `domain.NewMediaAsset`, `domain.ErrMediaNotFound`.
  - `dbtest.YDBMigrated`, `dbtest.AppPool`, `dbtest.RandString`.
- Produces:
  ```go
  // package adapters
  func NewYDBAssets(d *ydb.Driver) *YDBAssets
  ```
  and the `backend` harness (`name`, `assets`, `newTenant`), `eachBackend`, `seedTenant` and `newAsset`, which Task 2 reuses.

- [ ] **Step 1: Create the migration.**

```sql
-- Media context (internal/media): the metadata of a tenant's private media
-- library. The bytes live in Object Storage under media/<tenant_id>/<id>/...;
-- this table holds only the row. No column has a DEFAULT, no CHECK and no
-- foreign key: the adapter writes every value and the domain keeps them valid
-- (see 000008_campaign). width and height are 0 for a non-image (Postgres
-- stores NULL), uploaded_by is NULL when unknown. The listing reads every row
-- of one tenant's key range and orders it in YQL, so no secondary index.
CREATE TABLE IF NOT EXISTS media_assets (
    tenant_id    Utf8      NOT NULL,
    id           Utf8      NOT NULL,
    filename     Utf8      NOT NULL,
    content_type Utf8      NOT NULL,
    size_bytes   Int64     NOT NULL,
    width        Int32     NOT NULL,
    height       Int32     NOT NULL,
    storage_key  Utf8      NOT NULL,
    uploaded_by  Utf8,
    published_at Timestamp,
    created_at   Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id)
);
```

- [ ] **Step 2: Create `internal/media/adapters/assets_ydb.go`.**

```go
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
```

- [ ] **Step 3: Rewrite the repository tests as a pg+ydb suite.**

Run `git mv internal/media/adapters/assets_pg_test.go internal/media/adapters/assets_test.go`, then replace its whole content with:

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/media/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// backend is one storage implementation of the media repository under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name   string
	assets func(t *testing.T) domain.MediaRepository
	// newTenant is a real row on Postgres, where media_assets carries a
	// foreign key to tenants. YDB has none, so a bare UUID stands in.
	newTenant func(t *testing.T) string
}

var backends = []backend{
	{
		name: "pg",
		assets: func(t *testing.T) domain.MediaRepository {
			return adapters.NewAssets(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
	},
	{
		name: "ydb",
		assets: func(t *testing.T) domain.MediaRepository {
			return adapters.NewYDBAssets(dbtest.YDBMigrated(t))
		},
		newTenant: func(*testing.T) string { return uuid.NewString() },
	},
}

func eachBackend(t *testing.T, body func(t *testing.T, b backend)) {
	t.Helper()
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			body(t, b)
		})
	}
}

func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"media-"+dbtest.RandString()).Scan(&id))
	return id
}

func newAsset(t *testing.T, tenantID string) *domain.MediaAsset {
	t.Helper()
	id := uuid.NewString()
	a, err := domain.NewMediaAsset(id, tenantID, "logo.png", "image/png", 42, 1<<20, 64, 32,
		"media/"+tenantID+"/"+id+"/logo.png", "")
	require.NoError(t, err)
	return a
}

func TestAssetsAddGetListDelete(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		a := newAsset(t, tenantID)
		require.NoError(t, repo.Add(ctx, a))

		got, err := repo.Get(ctx, tenantID, a.ID())
		require.NoError(t, err)
		require.Equal(t, "logo.png", got.Filename())
		require.Equal(t, "image/png", got.ContentType())
		require.EqualValues(t, 42, got.SizeBytes())
		require.Equal(t, 64, got.Width())
		require.Equal(t, 32, got.Height())
		require.Equal(t, a.StorageKey(), got.StorageKey())
		require.Equal(t, tenantID, got.TenantID())
		require.False(t, got.IsPublished())
		require.WithinDuration(t, time.Now(), got.CreatedAt(), time.Minute)

		list, err := repo.List(ctx, tenantID)
		require.NoError(t, err)
		require.Len(t, list, 1)

		require.NoError(t, repo.Delete(ctx, tenantID, a.ID()))
		_, err = repo.Get(ctx, tenantID, a.ID())
		require.ErrorIs(t, err, domain.ErrMediaNotFound)
	})
}

func TestAssetsGetUnknownIsNotFound(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Get(ctx, tenantID, uuid.NewString())
		require.ErrorIs(t, err, domain.ErrMediaNotFound)
		_, err = repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrMediaNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, uuid.NewString()), domain.ErrMediaNotFound)
	})
}

func TestAssetsAreTenantScoped(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		owner, other := b.newTenant(t), b.newTenant(t)

		a := newAsset(t, owner)
		require.NoError(t, repo.Add(ctx, a))

		_, err := repo.Get(ctx, other, a.ID())
		require.ErrorIs(t, err, domain.ErrMediaNotFound)
		require.ErrorIs(t, repo.Delete(ctx, other, a.ID()), domain.ErrMediaNotFound)
		require.ErrorIs(t, repo.MarkPublished(ctx, other, a.ID(), time.Now()), domain.ErrMediaNotFound)
		list, err := repo.List(ctx, other)
		require.NoError(t, err)
		require.Empty(t, list)

		got, err := repo.Get(ctx, owner, a.ID())
		require.NoError(t, err)
		require.False(t, got.IsPublished(), "another tenant's MarkPublished changed nothing")
	})
}

func TestAssetsListOrdersNewestFirst(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		first := newAsset(t, tenantID)
		require.NoError(t, repo.Add(ctx, first))
		second := newAsset(t, tenantID)
		require.NoError(t, repo.Add(ctx, second))

		list, err := repo.List(ctx, tenantID)
		require.NoError(t, err)
		require.Len(t, list, 2)
		require.Equal(t, second.ID(), list[0].ID(), "the newest asset is returned first")
	})
}

func TestAssetsMarkPublished(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		a := newAsset(t, tenantID)
		require.NoError(t, repo.Add(ctx, a))
		require.NoError(t, repo.MarkPublished(ctx, tenantID, a.ID(), time.Now()))
		require.NoError(t, repo.MarkPublished(ctx, tenantID, a.ID(), time.Now()), "marking twice is fine")

		got, err := repo.Get(ctx, tenantID, a.ID())
		require.NoError(t, err)
		require.True(t, got.IsPublished())

		require.ErrorIs(t, repo.MarkPublished(ctx, tenantID, uuid.NewString(), time.Now()),
			domain.ErrMediaNotFound)
	})
}

func TestAssetsStorePDFWithoutDimensions(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id := uuid.NewString()
		pdf, err := domain.NewMediaAsset(id, tenantID, "a.pdf", "application/pdf", 42, 1<<20, 0, 0,
			"media/"+tenantID+"/"+id+"/a.pdf", "")
		require.NoError(t, err)
		require.NoError(t, repo.Add(ctx, pdf))
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Zero(t, got.Width())
		require.Zero(t, got.Height())
		require.Empty(t, got.UploadedBy())
	})
}

// The uploader is a foreign key to users on Postgres (it needs a platform user
// and a user row), so this one test runs on YDB only.
func TestYDBAssetsStoreTheUploader(t *testing.T) {
	t.Parallel()
	repo := adapters.NewYDBAssets(dbtest.YDBMigrated(t))
	ctx := context.Background()
	tenantID, user := uuid.NewString(), uuid.NewString()

	id := uuid.NewString()
	a, err := domain.NewMediaAsset(id, tenantID, "logo.png", "image/png", 42, 1<<20, 64, 32,
		"media/"+tenantID+"/"+id+"/logo.png", user)
	require.NoError(t, err)
	require.NoError(t, repo.Add(ctx, a))

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, user, got.UploadedBy())
	require.Error(t, repo.Add(ctx, a), "the id is the key: a second Add of it fails")
}
```

- [ ] **Step 4: Verify.**

```bash
gofmt -l internal/media internal/db
go build ./... && go vet ./internal/media/... ./internal/db/...
golangci-lint run ./internal/media/... ./internal/db/...
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/media/... ./internal/db/... -count=1 -timeout 8m
```

Expect: no `gofmt` output, `0 issues.`, every package `ok`. `go test -v -run Assets ./internal/media/adapters` lists a `/pg` and a `/ydb` subtest for each contract test, and `TestYDBAssetsStoreTheUploader`.

- [ ] **Step 5: Sabotage check (do not commit).** In `Get` and in the `MarkPublished` read, replace `WHERE tenant_id = $tenant_id AND id = $id` with `WHERE id = $id AND $tenant_id = $tenant_id`. `TestAssetsAreTenantScoped/ydb` must fail. Restore the file (`git diff` shows nothing for it afterwards beyond your change).

- [ ] **Step 6: Commit.**

```bash
git add internal/db/ydbmigrations/000010_media.up.sql internal/media/adapters/assets_ydb.go internal/media/adapters/assets_test.go
git commit -m "feat(media): port the media library to YDB"
```

(`git add` also stages the rename's deletion of `assets_pg_test.go`; confirm with `git status`.)

---

### Task 2: The media use cases over both repositories

**Files:**
- Create: `internal/media/adapters/usecases_test.go`

**Interfaces:**
- Consumes: `backend`, `eachBackend`, `newAsset` from Task 1; `command.NewPublishAssetsHandler`, `command.NewDeleteAssetHandler`; `adapters.NewMemoryBlobStore` and its `PublishErr`, `IsPublic`.
- Produces: tests only.

Why: the three campaign `Update` closures (`campaigns.go`, `templates.go`, `save_visual_template.go`) call `PublishAssets`, which a YDB `DoTx` retry re-runs. The repeat must copy nothing and return the same text (V6, accepted in the spec). These tests hold the handlers to that over the YDB repository, and pin the "delete keeps the public copy" decision of the media library.

- [ ] **Step 1: Create `internal/media/adapters/usecases_test.go`.**

```go
package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/media/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/media/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

const useCaseBaseURL = "https://app.test"

// storeAsset adds an asset to the repository and its bytes to the blob store,
// as UploadAsset does, and returns the asset with its library URL.
func storeAsset(t *testing.T, repo domain.MediaRepository, blobs *adapters.MemoryBlobStore,
	tenantID string) (*domain.MediaAsset, string) {
	t.Helper()
	a := newAsset(t, tenantID)
	require.NoError(t, blobs.Put(context.Background(), a.StorageKey(), a.ContentType(), 4,
		bytes.NewReader([]byte("png!"))))
	require.NoError(t, repo.Add(context.Background(), a))
	return a, domain.LibraryURL(useCaseBaseURL, "acme", a.ID())
}

// PublishAssets runs inside the campaign Update closures, which the YDB SDK
// re-runs on a retry. The second run must copy nothing and give the same text.
func TestPublishAssetsIsRepeatable(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		blobs := adapters.NewMemoryBlobStore("https://media.test")
		ctx := context.Background()
		tenantID := b.newTenant(t)
		a, url := storeAsset(t, repo, blobs, tenantID)
		publish := command.NewPublishAssetsHandler(repo, blobs, useCaseBaseURL)

		html := `<img src="` + url + `">`
		first, err := publish.Handle(ctx, command.PublishAssets{TenantID: tenantID, Texts: []string{html}})
		require.NoError(t, err)
		key := publishedKey(a)
		require.Equal(t, `<img src="https://media.test/`+key+`">`, first.Texts[0])
		require.True(t, blobs.IsPublic(key))

		blobs.PublishErr = errors.New("must not copy twice")
		second, err := publish.Handle(ctx, command.PublishAssets{TenantID: tenantID, Texts: []string{html}})
		require.NoError(t, err)
		require.Equal(t, first.Texts, second.Texts)

		got, err := repo.Get(ctx, tenantID, a.ID())
		require.NoError(t, err)
		require.True(t, got.IsPublished())
	})
}

func TestPublishAssetsFailsOnAnotherTenantsAsset(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		blobs := adapters.NewMemoryBlobStore("https://media.test")
		owner, other := b.newTenant(t), b.newTenant(t)
		a, url := storeAsset(t, repo, blobs, owner)

		_, err := command.NewPublishAssetsHandler(repo, blobs, useCaseBaseURL).Handle(context.Background(),
			command.PublishAssets{TenantID: other, Texts: []string{`<img src="` + url + `">`}})
		require.ErrorIs(t, err, domain.ErrMediaMissing)
		require.False(t, blobs.IsPublic(publishedKey(a)), "nothing was copied")
	})
}

// Deleting a library asset keeps its public copy unless the caller asks for
// it to go, as the media-library spec decides.
func TestDeleteAssetKeepsOrRemovesThePublishedCopy(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		blobs := adapters.NewMemoryBlobStore("https://media.test")
		ctx := context.Background()
		tenantID := b.newTenant(t)
		publish := command.NewPublishAssetsHandler(repo, blobs, useCaseBaseURL)
		del := command.NewDeleteAssetHandler(repo, blobs)

		keep, keepURL := storeAsset(t, repo, blobs, tenantID)
		drop, dropURL := storeAsset(t, repo, blobs, tenantID)
		_, err := publish.Handle(ctx, command.PublishAssets{TenantID: tenantID,
			Texts: []string{keepURL + " " + dropURL}})
		require.NoError(t, err)
		require.True(t, blobs.IsPublic(publishedKey(keep)))
		require.True(t, blobs.IsPublic(publishedKey(drop)))

		res, err := del.Handle(ctx, command.DeleteAsset{TenantID: tenantID, AssetID: keep.ID()})
		require.NoError(t, err)
		require.False(t, res.Unpublished)
		require.True(t, blobs.IsPublic(publishedKey(keep)), "the public copy outlives the library asset")

		res, err = del.Handle(ctx, command.DeleteAsset{TenantID: tenantID, AssetID: drop.ID(), Unpublish: true})
		require.NoError(t, err)
		require.True(t, res.Unpublished)
		require.False(t, blobs.IsPublic(publishedKey(drop)))
		require.True(t, blobs.IsPublic(publishedKey(keep)))

		list, err := repo.List(ctx, tenantID)
		require.NoError(t, err)
		require.Empty(t, list)
		_, err = del.Handle(ctx, command.DeleteAsset{TenantID: tenantID, AssetID: keep.ID()})
		require.ErrorIs(t, err, domain.ErrMediaNotFound)
	})
}

func publishedKey(a *domain.MediaAsset) string {
	return "published/" + a.TenantID() + "/" + a.ID() + "/" + a.Filename()
}
```

- [ ] **Step 2: Verify.**

```bash
gofmt -l internal/media
go vet ./internal/media/... && golangci-lint run ./internal/media/...
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/media/... -count=1 -timeout 8m
```

Expect: `0 issues.`, every package `ok`.

- [ ] **Step 3: Commit.**

```bash
git add internal/media/adapters/usecases_test.go
git commit -m "test(media): hold publish and delete to the same suite on both backends"
```

---

## Done criteria for Plan B9

- `000010_media.up.sql` applies on a fresh YDB and again without error.
- `YDBAssets` is held to the same suite as the Postgres `Assets`, and `PublishAssets` and `DeleteAsset` pass over both.
- `go build ./... && go vet ./... && go test ./...`, `golangci-lint run` and `make lint-arch` are clean.
- The spec's "Plan B9 planning spike" lists every repository constructor the app wires and its YDB version: Plan B is complete for every in-scope context.
- **Not done here, left to Plan C4** (spec "Plan B9 planning spike", "For C4 and the completeness of Plan B"):
  - wiring `NewYDBAssets` in `buildMedia` of the serverless composition root;
  - billing nil-wiring, the Redis throttles replaced by `ydbthrottle`, and the billing usage-rollup, which has no `/cron` route (billing is not wired); the other cron routes were built in C1 (`internal/service/worker_routes.go` mounts `/cron/domain-verify-sweep` and `/cron/analytics-refresh`), and B8's `/cron/feedback-sweep` route plus its timer remain for C4/C3;
  - verifying `S3BlobStore.Publish`'s `ACL: public-read` against the real bucket and service account.

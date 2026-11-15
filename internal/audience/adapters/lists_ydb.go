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

// YDBLists is the YDB-backed implementation of domain.ListRepository, for the
// serverless runtime. YDB has no row-level security, so every statement
// filters by tenant_id explicitly. A list's tags are stored as a JSON array in
// a Utf8 column.
type YDBLists struct {
	driver *ydb.Driver
}

var _ domain.ListRepository = (*YDBLists)(nil)

// NewYDBLists builds a YDBLists repository over the driver.
func NewYDBLists(d *ydb.Driver) *YDBLists {
	return &YDBLists{driver: d}
}

// ydbListColumns is the projection scanYDBList reads, in order.
const ydbListColumns = `id, tenant_id, name, description, visibility, optin, tags, created_at, updated_at`

const ydbSelectList = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbListColumns + `
FROM lists
WHERE tenant_id = $tenant_id AND id = $id;`

// Add persists a new list under a fresh UUID and returns the id. It returns
// domain.ErrListNameTaken when the tenant already has a list with that name.
func (r *YDBLists) Add(ctx context.Context, tenantID string, l *domain.List) (string, error) {
	id := uuid.NewString()
	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $description AS Utf8;
DECLARE $visibility AS Utf8;
DECLARE $optin AS Utf8;
DECLARE $tags AS Utf8;
INSERT INTO lists (tenant_id, id, name, description, visibility, optin, tags, created_at, updated_at)
VALUES ($tenant_id, $id, $name, $description, $visibility, $optin, $tags,
    CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$name").Text(l.Name()).
			Param("$description").Text(l.Description()).
			Param("$visibility").Text(string(l.Visibility())).
			Param("$optin").Text(string(l.OptIn())).
			Param("$tags").Text(ydbStrings(l.Tags())).
			Build()))
	// Only the name index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrListNameTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting list: %w", err)
	}
	return id, nil
}

// Update loads the list, runs fn, and persists the result in one serializable
// transaction. It returns domain.ErrListNameTaken when the new name belongs to
// another list of the tenant. The SDK retries the transaction on conflict, so
// fn may run more than once and must be retry-safe.
func (r *YDBLists) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.List) (*domain.List, error)) error {

	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbListFromRow(tx.QueryRow(ctx, ydbSelectList, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $description AS Utf8;
DECLARE $visibility AS Utf8;
DECLARE $optin AS Utf8;
DECLARE $tags AS Utf8;
UPDATE lists SET name = $name, description = $description, visibility = $visibility,
    optin = $optin, tags = $tags, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$name").Text(updated.Name()).
				Param("$description").Text(updated.Description()).
				Param("$visibility").Text(string(updated.Visibility())).
				Param("$optin").Text(string(updated.OptIn())).
				Param("$tags").Text(ydbStrings(updated.Tags())).
				Build()))
		// An UPDATE onto another list's name hits the unique name index at
		// the statement (spec "Plan B4 planning spike").
		if db.IsYDBConflict(err) {
			return domain.ErrListNameTaken
		}
		if err != nil {
			return fmt.Errorf("updating list: %w", err)
		}
		return nil
	})
	// This covers a YDB that reports the collision at commit.
	if db.IsYDBConflict(err) {
		return domain.ErrListNameTaken
	}
	return err
}

// Delete removes the list and, in the same transaction, the rows Postgres
// deletes by foreign-key cascade: the list's memberships, the iam per-list
// role assignments on it and the campaign targets that name it. It returns domain.ErrListNotFound when absent.
func (r *YDBLists) Delete(ctx context.Context, tenantID, id string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM lists
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`, ydbTenantAndID(tenantID, id))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrListNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting list: %w", err)
		}
		// ponytail: one transaction deletes every membership of the list; page
		// the delete if a list outgrows what one YDB transaction can hold.
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM subscriber_lists ON
SELECT tenant_id, subscriber_id, list_id
FROM subscriber_lists VIEW idx_subscriber_lists_list
WHERE tenant_id = $tenant_id AND list_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
			return fmt.Errorf("deleting list memberships: %w", err)
		}
		// user_list_roles belongs to iam. Postgres cascades into it through
		// its foreign key to lists; without this delete a dangling row would
		// keep its role in use forever. The scan covers the tenant's per-list
		// role assignments, a handful per user.
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM user_list_roles
WHERE tenant_id = $tenant_id AND list_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
			return fmt.Errorf("deleting list roles: %w", err)
		}
		// campaign_lists belongs to campaign. Postgres cascades into it
		// through its foreign key to lists; a dangling target would make
		// every send of that campaign resolve a list that no longer exists.
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM campaign_lists ON
SELECT tenant_id, campaign_id, id
FROM campaign_lists VIEW idx_campaign_lists_list
WHERE tenant_id = $tenant_id AND list_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
			return fmt.Errorf("deleting campaign targets: %w", err)
		}
		return nil
	})
}

// Get returns the list, or domain.ErrListNotFound.
func (r *YDBLists) Get(ctx context.Context, tenantID, id string) (*domain.List, error) {
	return ydbListFromRow(r.driver.Query().QueryRow(ctx, ydbSelectList, ydbTenantAndID(tenantID, id)))
}

// All returns a page of the tenant's lists, ordered by name, and the total
// count, read in one transaction.
func (r *YDBLists) All(ctx context.Context, tenantID string, page domain.Page) ([]*domain.List, int, error) {
	page = page.Normalize()
	var lists []*domain.List
	var total int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		count, err := ydbCount(ctx, tx, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM lists WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("counting lists: %w", err)
		}
		// ponytail: OFFSET paging reads every skipped row (spec "Ceilings").
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT `+ydbListColumns+`
FROM lists
WHERE tenant_id = $tenant_id
ORDER BY name
LIMIT $limit OFFSET $offset;`, ydbTenantAndPage(tenantID, page))
		if err != nil {
			return fmt.Errorf("listing lists: %w", err)
		}
		// Collected into a local and assigned at the end: the SDK may re-run
		// the closure, and a retry must not append to rows a failed run read.
		var local []*domain.List
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing lists: %w", err)
			}
			l, err := scanYDBList(row)
			if err != nil {
				return err
			}
			local = append(local, l)
		}
		lists, total = local, count
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return lists, total, nil
}

// ydbListFromRow maps a single-row read to the list, translating "no row"
// into domain.ErrListNotFound.
func ydbListFromRow(row query.Row, err error) (*domain.List, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrListNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading list: %w", err)
	}
	return scanYDBList(row)
}

// scanYDBList reads one row in ydbListColumns order.
func scanYDBList(row query.Row) (*domain.List, error) {
	var id, tenantID, name, description, visibility, optin, rawTags string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &name, &description, &visibility, &optin,
		&rawTags, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning list: %w", err)
	}
	tags, err := ydbStringsFrom(rawTags)
	if err != nil {
		return nil, fmt.Errorf("decoding list tags: %w", err)
	}
	return domain.HydrateList(id, tenantID, name, description,
		domain.Visibility(visibility), domain.OptIn(optin), tags, createdAt, updatedAt), nil
}

// ydbStrings encodes a string slice as the JSON array a Utf8 column stores
// (Postgres: text[]). A nil slice encodes as [], as Postgres reads back '{}'.
func ydbStrings(s []string) string {
	if s == nil {
		s = []string{}
	}
	raw, _ := json.Marshal(s) // a []string always encodes
	return string(raw)
}

// ydbStringsFrom decodes a column written by ydbStrings.
func ydbStringsFrom(raw string) ([]string, error) {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ydbCount runs a single-row COUNT(*) query.
func ydbCount(ctx context.Context, tx query.TxActor, q string, opt query.ExecuteOption) (int, error) {
	row, err := tx.QueryRow(ctx, q, opt)
	if err != nil {
		return 0, err
	}
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return int(n), nil
}

// ydbTenantParam binds the $tenant_id parameter.
func ydbTenantParam(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

// ydbTenantAndID binds the (tenant_id, id) primary key.
func ydbTenantAndID(tenantID, id string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Build())
}

// ydbTenantAndPage binds $tenant_id, $limit and $offset for a normalized page.
func ydbTenantAndPage(tenantID string, page domain.Page) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$limit").Uint64(uint64(page.Limit)).
		Param("$offset").Uint64(uint64(page.Offset)).
		Build())
}

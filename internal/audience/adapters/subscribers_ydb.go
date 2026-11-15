package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBSubscribers is the YDB-backed implementation of
// domain.SubscriberRepository, for the serverless runtime. Attributes are a
// JsonDocument, which segment queries read with JSON_VALUE and JSON_EXISTS.
// YDB has no foreign keys, so Delete removes the subscriber's memberships in
// the same transaction, as the Postgres cascade does.
type YDBSubscribers struct {
	driver *ydb.Driver
}

var _ domain.SubscriberRepository = (*YDBSubscribers)(nil)

// NewYDBSubscribers builds a YDBSubscribers repository over the driver.
func NewYDBSubscribers(d *ydb.Driver) *YDBSubscribers {
	return &YDBSubscribers{driver: d}
}

// ydbSubscriberColumns is the projection scanYDBSubscriber reads, in order.
// Every query aliases the subscribers table as s.
const ydbSubscriberColumns = `s.id, s.tenant_id, s.email, s.name, s.state, s.attributes,
    s.created_at, s.updated_at`

const ydbSelectSubscriber = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbSubscriberColumns + `
FROM subscribers AS s
WHERE s.tenant_id = $tenant_id AND s.id = $id;`

// Add persists a new subscriber under a fresh UUID and returns the id. It
// returns domain.ErrSubscriberEmailTaken when the tenant already has that
// email.
func (r *YDBSubscribers) Add(ctx context.Context, tenantID string, s *domain.Subscriber) (string, error) {
	attrs, err := marshalAttributes(s.Attributes())
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	err = r.driver.Query().Exec(ctx, ydbInsertSubscriber, ydbSubscriberParams(tenantID, id, s, attrs))
	// Only the email index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrSubscriberEmailTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting subscriber: %w", err)
	}
	return id, nil
}

const ydbInsertSubscriber = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $name AS Utf8;
DECLARE $state AS Utf8;
DECLARE $attributes AS JsonDocument;
INSERT INTO subscribers (tenant_id, id, email, name, state, attributes, created_at, updated_at)
VALUES ($tenant_id, $id, $email, $name, $state, $attributes,
    CurrentUtcTimestamp(), CurrentUtcTimestamp());`

// ydbSubscriberParams binds every column ydbInsertSubscriber writes.
func ydbSubscriberParams(tenantID, id string, s *domain.Subscriber, attrs []byte) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Param("$email").Text(s.Email()).
		Param("$name").Text(s.Name()).
		Param("$state").Text(string(s.State())).
		Param("$attributes").JSONDocumentFromBytes(attrs).
		Build())
}

// Update loads the subscriber, runs fn, and persists its name, state and
// attributes in one serializable transaction. The email is immutable, as on
// Postgres. The SDK retries the transaction on conflict, so fn may run more
// than once and must be retry-safe.
func (r *YDBSubscribers) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Subscriber) (*domain.Subscriber, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbSubscriberFromRow(tx.QueryRow(ctx, ydbSelectSubscriber, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		attrs, err := marshalAttributes(updated.Attributes())
		if err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $state AS Utf8;
DECLARE $attributes AS JsonDocument;
UPDATE subscribers SET name = $name, state = $state, attributes = $attributes,
    updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$name").Text(updated.Name()).
				Param("$state").Text(string(updated.State())).
				Param("$attributes").JSONDocumentFromBytes(attrs).
				Build())); err != nil {
			return fmt.Errorf("updating subscriber: %w", err)
		}
		return nil
	})
}

// UpsertByEmail creates the subscriber if its email is new, or updates the
// name and attributes of the existing one, reporting whether a new row was
// created. The existing subscriber's state is kept, as on Postgres, so an
// import never re-enables a blocklisted address. Concurrent upserts of one
// email abort each other, and the SDK retries the loser, which then sees the
// winner's row.
func (r *YDBSubscribers) UpsertByEmail(ctx context.Context, tenantID string,
	s *domain.Subscriber) (bool, error) {

	attrs, err := marshalAttributes(s.Attributes())
	if err != nil {
		return false, err
	}
	newID := uuid.NewString()
	var created bool
	err = r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $email AS Utf8;
SELECT id FROM subscribers VIEW idx_subscribers_email
WHERE tenant_id = $tenant_id AND email = $email;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$email").Text(s.Email()).
				Build()))
		if errors.Is(err, query.ErrNoRows) {
			if err := tx.Exec(ctx, ydbInsertSubscriber,
				ydbSubscriberParams(tenantID, newID, s, attrs)); err != nil {
				return fmt.Errorf("inserting subscriber: %w", err)
			}
			created = true
			return nil
		}
		if err != nil {
			return fmt.Errorf("looking up subscriber by email: %w", err)
		}
		var id string
		if err := row.Scan(&id); err != nil {
			return fmt.Errorf("scanning subscriber id: %w", err)
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $attributes AS JsonDocument;
UPDATE subscribers SET name = $name, attributes = $attributes, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$name").Text(s.Name()).
				Param("$attributes").JSONDocumentFromBytes(attrs).
				Build())); err != nil {
			return fmt.Errorf("updating subscriber: %w", err)
		}
		created = false
		return nil
	})
	if err != nil {
		return false, err
	}
	return created, nil
}

// Delete removes the subscriber and, in the same transaction, its
// memberships, as the Postgres cascade does. It returns
// domain.ErrSubscriberNotFound when absent.
func (r *YDBSubscribers) Delete(ctx context.Context, tenantID, id string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM subscribers
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`, ydbTenantAndID(tenantID, id))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrSubscriberNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting subscriber: %w", err)
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM subscriber_lists
WHERE tenant_id = $tenant_id AND subscriber_id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
			return fmt.Errorf("deleting subscriber memberships: %w", err)
		}
		return nil
	})
}

// Get returns the subscriber, or domain.ErrSubscriberNotFound.
func (r *YDBSubscribers) Get(ctx context.Context, tenantID, id string) (*domain.Subscriber, error) {
	return ydbSubscriberFromRow(r.driver.Query().QueryRow(ctx, ydbSelectSubscriber,
		ydbTenantAndID(tenantID, id)))
}

// GetByEmail returns the subscriber with exactly this email, or
// domain.ErrSubscriberNotFound. The column holds the email lower-cased, so
// lower-casing the argument matches the Postgres citext comparison.
func (r *YDBSubscribers) GetByEmail(ctx context.Context, tenantID, email string) (*domain.Subscriber, error) {
	return ydbSubscriberFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $email AS Utf8;
SELECT `+ydbSubscriberColumns+`
FROM subscribers VIEW idx_subscribers_email AS s
WHERE s.tenant_id = $tenant_id AND s.email = $email;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$email").Text(strings.ToLower(email)).
			Build())))
}

// Search returns a page of subscribers whose email or name contains q, case
// insensitively, and the total count. An empty q matches all subscribers.
func (r *YDBSubscribers) Search(ctx context.Context, tenantID, q string,
	page domain.Page) ([]*domain.Subscriber, int, error) {

	page = page.Normalize()
	pattern := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	const where = `WHERE s.tenant_id = $tenant_id
    AND ($pattern = "%%"u OR s.email ILIKE $pattern OR s.name ILIKE $pattern)`
	params := func(withPage bool) query.ExecuteOption {
		b := ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$pattern").Text(pattern)
		if withPage {
			b = b.Param("$limit").Uint64(uint64(page.Limit)).
				Param("$offset").Uint64(uint64(page.Offset))
		}
		return query.WithParameters(b.Build())
	}
	return r.page(ctx, "searching subscribers",
		`DECLARE $tenant_id AS Utf8;
DECLARE $pattern AS Utf8;
SELECT COUNT(*) FROM subscribers AS s
`+where+`;`, params(false),
		`DECLARE $tenant_id AS Utf8;
DECLARE $pattern AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT `+ydbSubscriberColumns+`
FROM subscribers AS s
`+where+`
ORDER BY s.created_at DESC
LIMIT $limit OFFSET $offset;`, params(true))
}

// InList returns a page of the subscribers attached to one list and the
// total count. An unsubscribed membership is excluded: a subscriber who has
// left a list is no longer a recipient of, nor exported as a member of, it.
func (r *YDBSubscribers) InList(ctx context.Context, tenantID, listID string,
	page domain.Page) ([]*domain.Subscriber, int, error) {

	page = page.Normalize()
	const from = `FROM subscriber_lists VIEW idx_subscriber_lists_list AS sl
JOIN subscribers AS s ON s.tenant_id = sl.tenant_id AND s.id = sl.subscriber_id
WHERE sl.tenant_id = $tenant_id AND sl.list_id = $list_id
    AND sl.subscription_status != "unsubscribed"u`
	params := func(withPage bool) query.ExecuteOption {
		b := ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$list_id").Text(listID)
		if withPage {
			b = b.Param("$limit").Uint64(uint64(page.Limit)).
				Param("$offset").Uint64(uint64(page.Offset))
		}
		return query.WithParameters(b.Build())
	}
	return r.page(ctx, "listing list subscribers",
		`DECLARE $tenant_id AS Utf8;
DECLARE $list_id AS Utf8;
SELECT COUNT(*) `+from+`;`, params(false),
		`DECLARE $tenant_id AS Utf8;
DECLARE $list_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT `+ydbSubscriberColumns+`
`+from+`
ORDER BY s.created_at DESC
LIMIT $limit OFFSET $offset;`, params(true))
}

// RunSegment translates a validated Segment to YQL and returns a page of the
// matching subscribers and the total count.
func (r *YDBSubscribers) RunSegment(ctx context.Context, tenantID string, seg domain.Segment,
	page domain.Page) ([]*domain.Subscriber, int, error) {

	page = page.Normalize()
	where, args := translateSegmentYQL(seg)
	declares := ydbSegmentDeclares(args)
	return r.page(ctx, "running segment",
		`DECLARE $tenant_id AS Utf8;
`+declares+`SELECT COUNT(*) FROM subscribers AS s
WHERE s.tenant_id = $tenant_id AND (`+where+`);`, ydbSegmentParams(tenantID, args, nil),
		`DECLARE $tenant_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
`+declares+`SELECT `+ydbSubscriberColumns+`
FROM subscribers AS s
WHERE s.tenant_id = $tenant_id AND (`+where+`)
ORDER BY s.created_at DESC
LIMIT $limit OFFSET $offset;`, ydbSegmentParams(tenantID, args, &page))
}

// CountSegment returns only the count of subscribers matching a Segment.
func (r *YDBSubscribers) CountSegment(ctx context.Context, tenantID string, seg domain.Segment) (int, error) {
	where, args := translateSegmentYQL(seg)
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
`+ydbSegmentDeclares(args)+`SELECT COUNT(*) FROM subscribers AS s
WHERE s.tenant_id = $tenant_id AND (`+where+`);`, ydbSegmentParams(tenantID, args, nil))
	if err != nil {
		return 0, fmt.Errorf("counting segment: %w", err)
	}
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("counting segment: %w", err)
	}
	return int(n), nil
}

// ydbSegmentParams binds $tenant_id, the segment's parameters and, for a page
// query, $limit and $offset.
func ydbSegmentParams(tenantID string, args []ydbSegmentArg, page *domain.Page) query.ExecuteOption {
	b := ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID)
	for _, a := range args {
		b = b.Param(a.name).Text(a.value)
	}
	if page != nil {
		b = b.Param("$limit").Uint64(uint64(page.Limit)).
			Param("$offset").Uint64(uint64(page.Offset))
	}
	return query.WithParameters(b.Build())
}

// page runs a COUNT(*) query and a page query in one transaction and returns
// the page and the total.
// ponytail: COUNT(*) plus OFFSET reads every matching row (spec "Ceilings");
// keyset paging if a tenant outgrows it.
func (r *YDBSubscribers) page(ctx context.Context, what string,
	countQuery string, countParams query.ExecuteOption,
	pageQuery string, pageParams query.ExecuteOption) ([]*domain.Subscriber, int, error) {

	var subs []*domain.Subscriber
	var total int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		count, err := ydbCount(ctx, tx, countQuery, countParams)
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		rs, err := tx.QueryResultSet(ctx, pageQuery, pageParams)
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		// Collected into a local and assigned at the end: the SDK may re-run
		// the closure, and a retry must not append to rows a failed run read.
		var local []*domain.Subscriber
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("%s: %w", what, err)
			}
			s, err := scanYDBSubscriber(row)
			if err != nil {
				return err
			}
			local = append(local, s)
		}
		subs, total = local, count
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return subs, total, nil
}

// ydbSubscriberFromRow maps a single-row read to the subscriber, translating
// "no row" into domain.ErrSubscriberNotFound.
func ydbSubscriberFromRow(row query.Row, err error) (*domain.Subscriber, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrSubscriberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading subscriber: %w", err)
	}
	return scanYDBSubscriber(row)
}

// scanYDBSubscriber reads one row in ydbSubscriberColumns order.
func scanYDBSubscriber(row query.Row) (*domain.Subscriber, error) {
	var id, tenantID, email, name, state, rawAttrs string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &email, &name, &state, &rawAttrs,
		&createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning subscriber: %w", err)
	}
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(rawAttrs), &raw); err != nil {
		return nil, fmt.Errorf("decoding subscriber attributes: %w", err)
	}
	return domain.HydrateSubscriber(id, tenantID, email, name, domain.State(state),
		domain.HydrateAttributes(raw), createdAt, updatedAt), nil
}

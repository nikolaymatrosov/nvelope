package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Subscribers is the pgx-backed implementation of domain.SubscriberRepository.
type Subscribers struct {
	pool *pgxpool.Pool
}

var _ domain.SubscriberRepository = (*Subscribers)(nil)

// NewSubscribers builds a Subscribers repository over the given pool.
func NewSubscribers(pool *pgxpool.Pool) *Subscribers {
	return &Subscribers{pool: pool}
}

const subscriberColumns = "id, tenant_id, email, name, state, attributes, created_at, updated_at"

// scanSubscriberRow reads one subscriber row in subscriberColumns order.
func scanSubscriberRow(row pgx.Row) (*domain.Subscriber, error) {
	var id, tenantID, email, name, state string
	var attrBytes []byte
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &email, &name, &state, &attrBytes,
		&createdAt, &updatedAt); err != nil {
		return nil, err
	}
	raw := map[string]any{}
	if len(attrBytes) > 0 {
		if err := json.Unmarshal(attrBytes, &raw); err != nil {
			return nil, fmt.Errorf("decoding subscriber attributes: %w", err)
		}
	}
	return domain.HydrateSubscriber(id, tenantID, email, name, domain.State(state),
		domain.HydrateAttributes(raw), createdAt, updatedAt), nil
}

// marshalAttributes encodes a subscriber's attributes as a jsonb document.
func marshalAttributes(a domain.Attributes) ([]byte, error) {
	b, err := json.Marshal(a.Values())
	if err != nil {
		return nil, fmt.Errorf("encoding subscriber attributes: %w", err)
	}
	return b, nil
}

// Add persists a new subscriber and returns its database-assigned id.
func (r *Subscribers) Add(ctx context.Context, tenantID string, s *domain.Subscriber) (string, error) {
	attrs, err := marshalAttributes(s.Attributes())
	if err != nil {
		return "", err
	}
	var id string
	err = tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO subscribers (tenant_id, email, name, state, attributes)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			tenantID, s.Email(), s.Name(), string(s.State()), attrs).Scan(&id)
		if db.IsUniqueViolation(err) {
			return domain.ErrSubscriberEmailTaken
		}
		if err != nil {
			return fmt.Errorf("inserting subscriber: %w", err)
		}
		return nil
	})
	return id, err
}

// Update loads the subscriber, runs fn, and persists the result.
func (r *Subscribers) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Subscriber) (*domain.Subscriber, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		loaded, err := r.getTx(ctx, tx, id)
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		return writeSubscriber(ctx, tx, id, updated)
	})
}

// UpsertByEmail creates the subscriber if its email is new, or updates the
// existing one, reporting whether a new row was created.
func (r *Subscribers) UpsertByEmail(ctx context.Context, tenantID string,
	s *domain.Subscriber) (bool, error) {

	attrs, err := marshalAttributes(s.Attributes())
	if err != nil {
		return false, err
	}
	var created bool
	err = tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		// xmax = 0 on the returned row iff the row was freshly inserted.
		var inserted bool
		err := tx.QueryRow(ctx,
			`INSERT INTO subscribers (tenant_id, email, name, state, attributes)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (tenant_id, email) DO UPDATE
			   SET name = EXCLUDED.name, attributes = EXCLUDED.attributes, updated_at = now()
			 RETURNING (xmax = 0)`,
			tenantID, s.Email(), s.Name(), string(s.State()), attrs).Scan(&inserted)
		if err != nil {
			return fmt.Errorf("upserting subscriber: %w", err)
		}
		created = inserted
		return nil
	})
	return created, err
}

// Delete removes the subscriber and cascades its memberships.
func (r *Subscribers) Delete(ctx context.Context, tenantID, id string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM subscribers WHERE id = $1", id)
		if db.IsInvalidInput(err) {
			return domain.ErrSubscriberNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting subscriber: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrSubscriberNotFound
		}
		return nil
	})
}

// Get returns the subscriber, or domain.ErrSubscriberNotFound.
func (r *Subscribers) Get(ctx context.Context, tenantID, id string) (*domain.Subscriber, error) {
	var out *domain.Subscriber
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		s, err := r.getTx(ctx, tx, id)
		if err != nil {
			return err
		}
		out = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Search returns a page of subscribers matching the free-text query q and the
// total count. An empty q matches all subscribers.
func (r *Subscribers) Search(ctx context.Context, tenantID, q string,
	page domain.Page) ([]*domain.Subscriber, int, error) {

	page = page.Normalize()
	var subs []*domain.Subscriber
	var total int
	pattern := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		const where = "WHERE ($1 = '%%' OR lower(email::text) LIKE $1 OR lower(name) LIKE $1)"
		if err := tx.QueryRow(ctx,
			"SELECT count(*) FROM subscribers "+where, pattern).Scan(&total); err != nil {
			return fmt.Errorf("counting subscribers: %w", err)
		}
		rows, err := tx.Query(ctx,
			"SELECT "+subscriberColumns+" FROM subscribers "+where+
				" ORDER BY created_at DESC LIMIT $2 OFFSET $3",
			pattern, page.Limit, page.Offset)
		if err != nil {
			return fmt.Errorf("searching subscribers: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			s, err := scanSubscriberRow(rows)
			if err != nil {
				return err
			}
			subs = append(subs, s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return subs, total, nil
}

// InList returns a page of the subscribers attached to one list.
func (r *Subscribers) InList(ctx context.Context, tenantID, listID string,
	page domain.Page) ([]*domain.Subscriber, int, error) {

	page = page.Normalize()
	var subs []*domain.Subscriber
	var total int
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		const join = "JOIN subscriber_lists sl ON sl.subscriber_id = s.id WHERE sl.list_id = $1"
		if err := tx.QueryRow(ctx,
			"SELECT count(*) FROM subscribers s "+join, listID).Scan(&total); err != nil {
			if db.IsInvalidInput(err) {
				return nil
			}
			return fmt.Errorf("counting list subscribers: %w", err)
		}
		rows, err := tx.Query(ctx,
			"SELECT "+prefixColumns("s")+" FROM subscribers s "+join+
				" ORDER BY s.created_at DESC LIMIT $2 OFFSET $3",
			listID, page.Limit, page.Offset)
		if err != nil {
			return fmt.Errorf("listing list subscribers: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			s, err := scanSubscriberRow(rows)
			if err != nil {
				return err
			}
			subs = append(subs, s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return subs, total, nil
}

// RunSegment translates a validated Segment to parameterized SQL and returns
// a page of the matching subscribers and the total count.
func (r *Subscribers) RunSegment(ctx context.Context, tenantID string, seg domain.Segment,
	page domain.Page) ([]*domain.Subscriber, int, error) {

	page = page.Normalize()
	where, args := translateSegment(seg)
	var subs []*domain.Subscriber
	var total int
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			"SELECT count(*) FROM subscribers s WHERE "+where, args...).Scan(&total); err != nil {
			return fmt.Errorf("counting segment: %w", err)
		}
		pageArgs := append(append([]any{}, args...), page.Limit, page.Offset)
		rows, err := tx.Query(ctx,
			fmt.Sprintf("SELECT %s FROM subscribers s WHERE %s ORDER BY s.created_at DESC LIMIT $%d OFFSET $%d",
				prefixColumns("s"), where, len(args)+1, len(args)+2),
			pageArgs...)
		if err != nil {
			return fmt.Errorf("running segment: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			s, err := scanSubscriberRow(rows)
			if err != nil {
				return err
			}
			subs = append(subs, s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return subs, total, nil
}

// CountSegment returns only the count of subscribers matching a Segment.
func (r *Subscribers) CountSegment(ctx context.Context, tenantID string, seg domain.Segment) (int, error) {
	where, args := translateSegment(seg)
	var total int
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT count(*) FROM subscribers s WHERE "+where, args...).Scan(&total)
	})
	if err != nil {
		return 0, fmt.Errorf("counting segment: %w", err)
	}
	return total, nil
}

// prefixColumns returns subscriberColumns with each column prefixed by the
// given table alias.
func prefixColumns(alias string) string {
	cols := []string{"id", "tenant_id", "email", "name", "state", "attributes",
		"created_at", "updated_at"}
	for i, c := range cols {
		cols[i] = alias + "." + c
	}
	return strings.Join(cols, ", ")
}

func (r *Subscribers) getTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Subscriber, error) {
	row := tx.QueryRow(ctx, "SELECT "+subscriberColumns+" FROM subscribers WHERE id = $1", id)
	s, err := scanSubscriberRow(row)
	if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
		return nil, domain.ErrSubscriberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading subscriber: %w", err)
	}
	return s, nil
}

// writeSubscriber persists the mutable fields of an existing subscriber.
func writeSubscriber(ctx context.Context, tx pgx.Tx, id string, s *domain.Subscriber) error {
	attrs, err := marshalAttributes(s.Attributes())
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`UPDATE subscribers SET name = $1, state = $2, attributes = $3, updated_at = now()
		 WHERE id = $4`,
		s.Name(), string(s.State()), attrs, id)
	if err != nil {
		return fmt.Errorf("updating subscriber: %w", err)
	}
	return nil
}

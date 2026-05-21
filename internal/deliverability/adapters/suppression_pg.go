package adapters

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Suppressions is the pgx-backed implementation of
// domain.SuppressionRepository.
type Suppressions struct {
	pool *pgxpool.Pool
}

var _ domain.SuppressionRepository = (*Suppressions)(nil)

// NewSuppressions builds a Suppressions repository over the given pool.
func NewSuppressions(pool *pgxpool.Pool) *Suppressions {
	return &Suppressions{pool: pool}
}

// Upsert adds a suppression entry; an address already suppressed for the
// tenant is a no-op.
func (r *Suppressions) Upsert(ctx context.Context, e *domain.SuppressionEntry) error {
	return tenantdb.WithTenant(ctx, r.pool, e.TenantID(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO suppression_list (tenant_id, email, reason, source_event_id, note)
			 VALUES (@tenant_id, @email, @reason, @source_event_id, @note)
			 ON CONFLICT (tenant_id, email) DO NOTHING`,
			pgx.NamedArgs{
				"tenant_id":       e.TenantID(),
				"email":           e.Email(),
				"reason":          string(e.Reason()),
				"source_event_id": nullString(e.SourceEventID()),
				"note":            e.Note(),
			})
		if err != nil {
			return fmt.Errorf("upserting suppression entry: %w", err)
		}
		return nil
	})
}

// Remove deletes a suppression entry, returning ErrSuppressionNotFound when no
// entry matches the address.
func (r *Suppressions) Remove(ctx context.Context, tenantID, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			"DELETE FROM suppression_list WHERE email = $1", email)
		if err != nil {
			return fmt.Errorf("removing suppression entry: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrSuppressionNotFound
		}
		return nil
	})
}

// List returns a page of the tenant's suppression entries, ordered by id, and
// the cursor for the next page (empty when the page is the last).
func (r *Suppressions) List(ctx context.Context, tenantID string, f domain.SuppressionFilter) (
	[]*domain.SuppressionEntry, string, error) {

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	conds := []string{}
	args := []any{}
	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.Reason != "" {
		add("reason = $%d", string(f.Reason))
	}
	if f.EmailLike != "" {
		add("email ILIKE '%%' || $%d || '%%'", f.EmailLike)
	}
	if f.Cursor != "" {
		add("id > $%d", f.Cursor)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit+1)
	query := fmt.Sprintf(
		`SELECT id, tenant_id, email, reason, coalesce(source_event_id::text, ''),
		        suppressed_at, note
		 FROM suppression_list %s ORDER BY id LIMIT $%d`, where, len(args))

	var out []*domain.SuppressionEntry
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("listing suppression entries: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			e, err := scanSuppressionRow(rows)
			if err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(out) > limit {
		nextCursor = out[limit-1].ID()
		out = out[:limit]
	}
	return out, nextCursor, nil
}

// scanSuppressionRow reads one suppression_list row.
func scanSuppressionRow(row pgx.Row) (*domain.SuppressionEntry, error) {
	var id, tenantID, email, reason, sourceEventID, note string
	var suppressedAt time.Time
	if err := row.Scan(&id, &tenantID, &email, &reason, &sourceEventID,
		&suppressedAt, &note); err != nil {
		return nil, err
	}
	return domain.HydrateSuppressionEntry(id, tenantID, email,
		domain.SuppressionReason(reason), sourceEventID, suppressedAt, note), nil
}

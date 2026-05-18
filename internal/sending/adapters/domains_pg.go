// Package adapters implements the sending domain's interfaces against
// PostgreSQL and the Postbox client. Every tenant-plane operation runs inside
// the shared RLS-bound transaction (internal/platform/tenantdb).
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// SendingDomains is the pgx-backed implementation of
// domain.SendingDomainRepository.
type SendingDomains struct {
	pool *pgxpool.Pool
}

var _ domain.SendingDomainRepository = (*SendingDomains)(nil)

// NewSendingDomains builds a SendingDomains repository over the given pool.
func NewSendingDomains(pool *pgxpool.Pool) *SendingDomains {
	return &SendingDomains{pool: pool}
}

const sendingDomainColumns = `id, tenant_id, domain, status, dkim_records, spf_record,
	dmarc_record, postbox_identity_ref, failure_reason, created_at, verified_at, last_checked_at`

// scanSendingDomainRow reads one row in sendingDomainColumns order.
func scanSendingDomainRow(row pgx.Row) (*domain.SendingDomain, error) {
	var id, tenantID, name, status, spf, dmarc, identityRef, failureReason string
	var dkimJSON []byte
	var createdAt time.Time
	var verifiedAt, lastCheckedAt *time.Time
	if err := row.Scan(&id, &tenantID, &name, &status, &dkimJSON, &spf,
		&dmarc, &identityRef, &failureReason, &createdAt, &verifiedAt, &lastCheckedAt); err != nil {
		return nil, err
	}
	var dkim []domain.DNSRecord
	if len(dkimJSON) > 0 {
		if err := json.Unmarshal(dkimJSON, &dkim); err != nil {
			return nil, fmt.Errorf("decoding dkim records: %w", err)
		}
	}
	return domain.HydrateSendingDomain(id, tenantID, name, domain.Status(status), dkim,
		spf, dmarc, identityRef, failureReason, createdAt, verifiedAt, lastCheckedAt), nil
}

// Add persists a new sending domain and returns its database-assigned id.
func (r *SendingDomains) Add(ctx context.Context, tenantID string, d *domain.SendingDomain) (string, error) {
	dkimJSON, err := json.Marshal(d.DKIMRecords())
	if err != nil {
		return "", fmt.Errorf("encoding dkim records: %w", err)
	}
	var id string
	err = tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO sending_domains
			    (tenant_id, domain, status, dkim_records, spf_record, dmarc_record, postbox_identity_ref)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			tenantID, d.Domain(), string(d.Status()), dkimJSON, d.SPFRecord(),
			d.DMARCRecord(), d.IdentityRef()).Scan(&id)
		if db.IsUniqueViolation(err) {
			return domain.ErrDomainAlreadyExists
		}
		if err != nil {
			return fmt.Errorf("inserting sending domain: %w", err)
		}
		return nil
	})
	return id, err
}

// Get returns the domain, or domain.ErrDomainNotFound.
func (r *SendingDomains) Get(ctx context.Context, tenantID, id string) (*domain.SendingDomain, error) {
	var out *domain.SendingDomain
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		d, err := r.getTx(ctx, tx, id)
		if err != nil {
			return err
		}
		out = d
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Update loads the domain, runs fn, and persists the result.
func (r *SendingDomains) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.SendingDomain) (*domain.SendingDomain, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		loaded, err := r.getTx(ctx, tx, id)
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		dkimJSON, err := json.Marshal(updated.DKIMRecords())
		if err != nil {
			return fmt.Errorf("encoding dkim records: %w", err)
		}
		_, err = tx.Exec(ctx,
			`UPDATE sending_domains SET status = $1, dkim_records = $2, spf_record = $3,
			    dmarc_record = $4, postbox_identity_ref = $5, failure_reason = $6,
			    verified_at = $7, last_checked_at = $8 WHERE id = $9`,
			string(updated.Status()), dkimJSON, updated.SPFRecord(), updated.DMARCRecord(),
			updated.IdentityRef(), updated.FailureReason(), updated.VerifiedAt(),
			updated.LastCheckedAt(), id)
		if err != nil {
			return fmt.Errorf("updating sending domain: %w", err)
		}
		return nil
	})
}

// All returns every sending domain of the tenant.
func (r *SendingDomains) All(ctx context.Context, tenantID string) ([]*domain.SendingDomain, error) {
	var domains []*domain.SendingDomain
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT "+sendingDomainColumns+" FROM sending_domains ORDER BY domain")
		if err != nil {
			return fmt.Errorf("listing sending domains: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			d, err := scanSendingDomainRow(rows)
			if err != nil {
				return err
			}
			domains = append(domains, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return domains, nil
}

// PendingIDs lists the ids of domains still awaiting verification.
func (r *SendingDomains) PendingIDs(ctx context.Context, tenantID string) ([]string, error) {
	var ids []string
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT id FROM sending_domains WHERE status = 'pending'")
		if err != nil {
			return fmt.Errorf("listing pending domains: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *SendingDomains) getTx(ctx context.Context, tx pgx.Tx, id string) (*domain.SendingDomain, error) {
	row := tx.QueryRow(ctx, "SELECT "+sendingDomainColumns+" FROM sending_domains WHERE id = $1", id)
	d, err := scanSendingDomainRow(row)
	if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
		return nil, domain.ErrDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading sending domain: %w", err)
	}
	return d, nil
}

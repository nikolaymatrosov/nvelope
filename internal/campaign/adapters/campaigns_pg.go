package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Campaigns is the pgx-backed implementation of domain.CampaignRepository.
type Campaigns struct {
	pool *pgxpool.Pool
}

var _ domain.CampaignRepository = (*Campaigns)(nil)

// NewCampaigns builds a Campaigns repository over the given pool.
func NewCampaigns(pool *pgxpool.Pool) *Campaigns {
	return &Campaigns{pool: pool}
}

const campaignColumns = `id, tenant_id, name, subject, body_html, body_text, from_name,
	from_local_part, sending_domain_id, template_id, status, max_send_errors,
	sent_count, failed_count, recipient_count, created_at, updated_at, started_at, finished_at,
	archive_visible, archived_at`

// nullableString maps "" to nil for a nullable text/uuid column.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// deref returns the pointed-to string, or "" for nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// scanCampaignRow reads one campaign row in campaignColumns order.
func scanCampaignRow(row pgx.Row) (*domain.Campaign, error) {
	var id, tenantID, name, subject, bodyHTML, bodyText, fromName, fromLocalPart, status string
	var sendingDomainID, templateID *string
	var maxSendErrors, sentCount, failedCount, recipientCount int
	var createdAt, updatedAt time.Time
	var startedAt, finishedAt, archivedAt *time.Time
	var archiveVisible bool
	if err := row.Scan(&id, &tenantID, &name, &subject, &bodyHTML, &bodyText, &fromName,
		&fromLocalPart, &sendingDomainID, &templateID, &status, &maxSendErrors,
		&sentCount, &failedCount, &recipientCount, &createdAt, &updatedAt,
		&startedAt, &finishedAt, &archiveVisible, &archivedAt); err != nil {
		return nil, err
	}
	return domain.HydrateCampaign(id, tenantID, name, subject, bodyHTML, bodyText, fromName,
		fromLocalPart, deref(sendingDomainID), deref(templateID), domain.CampaignStatus(status),
		maxSendErrors, sentCount, failedCount, recipientCount,
		createdAt, updatedAt, startedAt, finishedAt, archiveVisible, archivedAt), nil
}

// Add persists a new campaign and returns its database-assigned id.
func (r *Campaigns) Add(ctx context.Context, tenantID string, c *domain.Campaign) (string, error) {
	var id string
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO campaigns
			    (tenant_id, name, subject, body_html, body_text, from_name, from_local_part,
			     sending_domain_id, template_id, status, max_send_errors)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
			tenantID, c.Name(), c.Subject(), c.BodyHTML(), c.BodyText(), c.FromName(),
			c.FromLocalPart(), nullableString(c.SendingDomainID()), nullableString(c.TemplateID()),
			string(c.Status()), c.MaxSendErrors()).Scan(&id)
		if err != nil {
			return fmt.Errorf("inserting campaign: %w", err)
		}
		return nil
	})
	return id, err
}

// Get returns the campaign, or domain.ErrCampaignNotFound.
func (r *Campaigns) Get(ctx context.Context, tenantID, id string) (*domain.Campaign, error) {
	var out *domain.Campaign
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		c, err := r.getTx(ctx, tx, id)
		if err != nil {
			return err
		}
		out = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Update loads the campaign, runs fn, and persists the result.
func (r *Campaigns) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Campaign) (*domain.Campaign, error)) error {

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
			`UPDATE campaigns SET name = $1, subject = $2, body_html = $3, body_text = $4,
			    from_name = $5, from_local_part = $6, sending_domain_id = $7, status = $8,
			    max_send_errors = $9, sent_count = $10, failed_count = $11, recipient_count = $12,
			    started_at = $13, finished_at = $14, archive_visible = $15, archived_at = $16,
			    updated_at = now() WHERE id = $17`,
			updated.Name(), updated.Subject(), updated.BodyHTML(), updated.BodyText(),
			updated.FromName(), updated.FromLocalPart(), nullableString(updated.SendingDomainID()),
			string(updated.Status()), updated.MaxSendErrors(), updated.SentCount(),
			updated.FailedCount(), updated.RecipientCount(),
			updated.StartedAt(), updated.FinishedAt(),
			updated.ArchiveVisible(), updated.ArchivedAt(), id)
		if err != nil {
			return fmt.Errorf("updating campaign: %w", err)
		}
		return nil
	})
}

// Archived returns a page of the tenant's archive-visible campaigns newest-
// first by archived_at, and the total count.
func (r *Campaigns) Archived(ctx context.Context, tenantID string, page domain.Page) ([]*domain.Campaign, int, error) {
	page = page.Normalize()
	var out []*domain.Campaign
	var total int
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			"SELECT count(*) FROM campaigns WHERE archive_visible = true").Scan(&total); err != nil {
			return fmt.Errorf("counting archived campaigns: %w", err)
		}
		rows, err := tx.Query(ctx,
			"SELECT "+campaignColumns+" FROM campaigns WHERE archive_visible = true "+
				"ORDER BY archived_at DESC LIMIT $1 OFFSET $2",
			page.Limit, page.Offset)
		if err != nil {
			return fmt.Errorf("listing archived campaigns: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanCampaignRow(rows)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// All returns a page of the tenant's campaigns and the total count.
func (r *Campaigns) All(ctx context.Context, tenantID string, page domain.Page) ([]*domain.Campaign, int, error) {
	page = page.Normalize()
	var campaigns []*domain.Campaign
	var total int
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM campaigns").Scan(&total); err != nil {
			return fmt.Errorf("counting campaigns: %w", err)
		}
		rows, err := tx.Query(ctx,
			"SELECT "+campaignColumns+" FROM campaigns ORDER BY created_at DESC LIMIT $1 OFFSET $2",
			page.Limit, page.Offset)
		if err != nil {
			return fmt.Errorf("listing campaigns: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanCampaignRow(rows)
			if err != nil {
				return err
			}
			campaigns = append(campaigns, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return campaigns, total, nil
}

// SaveTargets replaces a campaign's targeted lists and segments.
func (r *Campaigns) SaveTargets(ctx context.Context, tenantID, campaignID string,
	targets []domain.Target) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"DELETE FROM campaign_lists WHERE campaign_id = $1", campaignID); err != nil {
			return fmt.Errorf("clearing campaign targets: %w", err)
		}
		for _, t := range targets {
			_, err := tx.Exec(ctx,
				`INSERT INTO campaign_lists (campaign_id, tenant_id, list_id, segment_query)
				 VALUES ($1, $2, $3, $4)`,
				campaignID, tenantID, nullableString(t.ListID), nullableJSON(t.SegmentQuery))
			if err != nil {
				return fmt.Errorf("inserting campaign target: %w", err)
			}
		}
		return nil
	})
}

// Targets returns a campaign's targeted lists and segments.
func (r *Campaigns) Targets(ctx context.Context, tenantID, campaignID string) ([]domain.Target, error) {
	var targets []domain.Target
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT list_id, segment_query FROM campaign_lists WHERE campaign_id = $1", campaignID)
		if err != nil {
			return fmt.Errorf("listing campaign targets: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var listID *string
			var segment []byte
			if err := rows.Scan(&listID, &segment); err != nil {
				return err
			}
			targets = append(targets, domain.Target{ListID: deref(listID), SegmentQuery: segment})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return targets, nil
}

func (r *Campaigns) getTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Campaign, error) {
	row := tx.QueryRow(ctx, "SELECT "+campaignColumns+" FROM campaigns WHERE id = $1", id)
	c, err := scanCampaignRow(row)
	if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
		return nil, domain.ErrCampaignNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading campaign: %w", err)
	}
	return c, nil
}

// nullableJSON maps an empty byte slice to nil for a nullable jsonb column.
func nullableJSON(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}

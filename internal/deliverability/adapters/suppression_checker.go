package adapters

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// SuppressionChecker is the pre-send gate consumed by the campaign send paths.
// It implements the campaign context's domain-owned SuppressionChecker port by
// reading the deliverability suppression list.
type SuppressionChecker struct {
	pool *pgxpool.Pool
}

var _ campaigndomain.SuppressionChecker = (*SuppressionChecker)(nil)

// NewSuppressionChecker builds a SuppressionChecker over the given pool.
func NewSuppressionChecker(pool *pgxpool.Pool) *SuppressionChecker {
	return &SuppressionChecker{pool: pool}
}

// Suppressed returns the subset of emails on the tenant's suppression list,
// mapped to the reason each was suppressed.
func (c *SuppressionChecker) Suppressed(ctx context.Context, tenantID string,
	emails []string) (map[string]string, error) {

	if len(emails) == 0 {
		return map[string]string{}, nil
	}
	normalized := make([]string, len(emails))
	for i, e := range emails {
		normalized[i] = strings.ToLower(strings.TrimSpace(e))
	}

	out := map[string]string{}
	err := tenantdb.WithTenant(ctx, c.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT email, reason FROM suppression_list WHERE email = ANY($1)", normalized)
		if err != nil {
			return fmt.Errorf("checking suppression list: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var email, reason string
			if err := rows.Scan(&email, &reason); err != nil {
				return err
			}
			out[email] = reason
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

package adapters

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// TransactionalMessages is the pgx-backed implementation of
// domain.TransactionalMessageRepository.
type TransactionalMessages struct {
	pool *pgxpool.Pool
}

var _ domain.TransactionalMessageRepository = (*TransactionalMessages)(nil)

// NewTransactionalMessages builds a TransactionalMessages repository over the
// given pool.
func NewTransactionalMessages(pool *pgxpool.Pool) *TransactionalMessages {
	return &TransactionalMessages{pool: pool}
}

// Record persists one transactional send inside the tenant-bound transaction.
func (r *TransactionalMessages) Record(ctx context.Context, tenantID, templateID,
	providerMessageID, recipientEmail string) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO transactional_messages
			   (tenant_id, template_id, provider_message_id, recipient_email)
			 VALUES (@tenant_id, @template_id, @provider_message_id, @recipient_email)`,
			pgx.NamedArgs{
				"tenant_id":           tenantID,
				"template_id":         nullableString(templateID),
				"provider_message_id": providerMessageID,
				"recipient_email":     recipientEmail,
			})
		if err != nil {
			return fmt.Errorf("recording transactional message: %w", err)
		}
		return nil
	})
}

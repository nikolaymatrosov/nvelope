package adapters

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// YDBTransactionalMessages is the YDB-backed implementation of
// domain.TransactionalMessageRepository, for the serverless runtime.
type YDBTransactionalMessages struct {
	driver *ydb.Driver
}

var _ domain.TransactionalMessageRepository = (*YDBTransactionalMessages)(nil)

// NewYDBTransactionalMessages builds a YDBTransactionalMessages repository
// over the driver.
func NewYDBTransactionalMessages(d *ydb.Driver) *YDBTransactionalMessages {
	return &YDBTransactionalMessages{driver: d}
}

// Record persists one transactional send under a fresh UUID. A provider
// message id that is already recorded is an error, as on Postgres.
func (r *YDBTransactionalMessages) Record(ctx context.Context, tenantID, templateID,
	providerMessageID, recipientEmail string) error {

	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $template_id AS Optional<Utf8>;
DECLARE $provider_message_id AS Utf8;
DECLARE $recipient_email AS Utf8;
INSERT INTO transactional_messages
    (tenant_id, id, template_id, provider_message_id, recipient_email, sent_at)
VALUES ($tenant_id, $id, $template_id, $provider_message_id, $recipient_email,
    CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(uuid.NewString()).
			Param("$template_id").BeginOptional().Text(nullableString(templateID)).EndOptional().
			Param("$provider_message_id").Text(providerMessageID).
			Param("$recipient_email").Text(recipientEmail).
			Build()))
	if err != nil {
		return fmt.Errorf("recording transactional message: %w", err)
	}
	return nil
}

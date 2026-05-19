package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// chargeReq builds a charge request for the mock gateway tests.
func chargeReq(key, tenantID, invoiceID string) domain.ChargeRequest {
	return domain.ChargeRequest{
		IdempotencyKey: key,
		Amount:         domain.NewMoney(990000, "RUB"),
		TenantID:       tenantID,
		InvoiceID:      invoiceID,
	}
}

func TestMockGatewayApprovesByDefault(t *testing.T) {
	t.Parallel()
	g := adapters.NewMockGateway()
	res, err := g.Charge(context.Background(), chargeReq("inv-1:1", "tenant-1", "inv-1"))
	require.NoError(t, err)
	require.Equal(t, domain.ChargeApproved, res.Outcome)
	require.NotEmpty(t, res.GatewayReference)
}

func TestMockGatewayDeterministicAndIdempotent(t *testing.T) {
	t.Parallel()
	g := adapters.NewMockGateway()
	ctx := context.Background()

	first, err := g.Charge(ctx, chargeReq("inv-7:1", "tenant-1", "inv-7"))
	require.NoError(t, err)
	second, err := g.Charge(ctx, chargeReq("inv-7:1", "tenant-1", "inv-7"))
	require.NoError(t, err)

	// Re-charging the same idempotency key returns an identical result — the
	// gateway never double-charges.
	require.Equal(t, first, second)

	// A different key yields a different, still deterministic reference.
	other, err := g.Charge(ctx, chargeReq("inv-7:2", "tenant-1", "inv-7"))
	require.NoError(t, err)
	require.NotEqual(t, first.GatewayReference, other.GatewayReference)
}

func TestMockGatewayDeclineTenant(t *testing.T) {
	t.Parallel()
	g := adapters.NewMockGateway()
	ctx := context.Background()
	g.DeclineTenant("tenant-bad", "card_declined")

	res, err := g.Charge(ctx, chargeReq("inv-1:1", "tenant-bad", "inv-1"))
	require.NoError(t, err)
	require.Equal(t, domain.ChargeDeclined, res.Outcome)
	require.Equal(t, "card_declined", res.DeclineReason)

	// Another tenant is unaffected.
	ok, err := g.Charge(ctx, chargeReq("inv-2:1", "tenant-good", "inv-2"))
	require.NoError(t, err)
	require.Equal(t, domain.ChargeApproved, ok.Outcome)
}

func TestMockGatewayDeclineInvoiceAndKey(t *testing.T) {
	t.Parallel()
	g := adapters.NewMockGateway()
	ctx := context.Background()
	g.DeclineInvoice("inv-9", "insufficient_funds")
	g.DeclineKey("inv-5:1", "expired_card")

	declined, err := g.Charge(ctx, chargeReq("inv-9:1", "tenant-1", "inv-9"))
	require.NoError(t, err)
	require.Equal(t, domain.ChargeDeclined, declined.Outcome)

	keyed, err := g.Charge(ctx, chargeReq("inv-5:1", "tenant-1", "inv-5"))
	require.NoError(t, err)
	require.Equal(t, domain.ChargeDeclined, keyed.Outcome)

	// A different attempt on the keyed invoice is not declined.
	retry, err := g.Charge(ctx, chargeReq("inv-5:2", "tenant-1", "inv-5"))
	require.NoError(t, err)
	require.Equal(t, domain.ChargeApproved, retry.Outcome)
}

func TestMockGatewayErrorAndReset(t *testing.T) {
	t.Parallel()
	g := adapters.NewMockGateway()
	ctx := context.Background()
	g.ErrorTenant("tenant-err")

	res, err := g.Charge(ctx, chargeReq("inv-1:1", "tenant-err", "inv-1"))
	require.NoError(t, err)
	require.Equal(t, domain.ChargeError, res.Outcome)

	g.Reset()
	res, err = g.Charge(ctx, chargeReq("inv-1:1", "tenant-err", "inv-1"))
	require.NoError(t, err)
	require.Equal(t, domain.ChargeApproved, res.Outcome)
}

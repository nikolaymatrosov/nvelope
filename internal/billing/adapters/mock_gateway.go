package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// MockGateway is the deterministic, in-process PaymentGateway shipped in Phase
// 5. The outcome of a charge is a pure function of its inputs: approve by
// default, unless a programmed rule forces a decline or a transient error for a
// chosen idempotency key, invoice, or tenant. Re-charging the same idempotency
// key always returns the same result, so renewal, dunning, and idempotency
// tests are repeatable without an external system.
type MockGateway struct {
	mu    sync.RWMutex
	rules map[string]domain.ChargeResult
}

var _ domain.PaymentGateway = (*MockGateway)(nil)

// NewMockGateway builds a MockGateway that approves every charge by default.
func NewMockGateway() *MockGateway {
	return &MockGateway{rules: map[string]domain.ChargeResult{}}
}

// DeclineTenant programs every charge for a tenant to be declined.
func (g *MockGateway) DeclineTenant(tenantID, reason string) {
	g.set(tenantID, domain.ChargeResult{Outcome: domain.ChargeDeclined, DeclineReason: reason})
}

// DeclineInvoice programs every charge for one invoice to be declined.
func (g *MockGateway) DeclineInvoice(invoiceID, reason string) {
	g.set(invoiceID, domain.ChargeResult{Outcome: domain.ChargeDeclined, DeclineReason: reason})
}

// DeclineKey programs a single charge attempt (by idempotency key) to be
// declined.
func (g *MockGateway) DeclineKey(idempotencyKey, reason string) {
	g.set(idempotencyKey, domain.ChargeResult{Outcome: domain.ChargeDeclined, DeclineReason: reason})
}

// ErrorTenant programs every charge for a tenant to fail with a transient
// gateway error.
func (g *MockGateway) ErrorTenant(tenantID string) {
	g.set(tenantID, domain.ChargeResult{Outcome: domain.ChargeError})
}

// Reset clears every programmed rule, returning the gateway to approve-by-default.
func (g *MockGateway) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rules = map[string]domain.ChargeResult{}
}

func (g *MockGateway) set(key string, result domain.ChargeResult) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rules[key] = result
}

// Charge returns the programmed outcome for the request, defaulting to an
// approval. Rule precedence is idempotency key, then invoice, then tenant.
func (g *MockGateway) Charge(_ context.Context, req domain.ChargeRequest) (domain.ChargeResult, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, key := range []string{req.IdempotencyKey, req.InvoiceID, req.TenantID} {
		if key == "" {
			continue
		}
		if r, ok := g.rules[key]; ok {
			return finalize(r, req), nil
		}
	}
	return domain.ChargeResult{
		Outcome:          domain.ChargeApproved,
		GatewayReference: mockReference(req.IdempotencyKey),
	}, nil
}

// finalize stamps a deterministic gateway reference onto an approved result.
func finalize(r domain.ChargeResult, req domain.ChargeRequest) domain.ChargeResult {
	if r.Outcome == domain.ChargeApproved && r.GatewayReference == "" {
		r.GatewayReference = mockReference(req.IdempotencyKey)
	}
	return r
}

// mockReference derives a stable provider charge id from an idempotency key.
func mockReference(idempotencyKey string) string {
	sum := sha256.Sum256([]byte("mock_gateway:" + idempotencyKey))
	return "mock_" + hex.EncodeToString(sum[:8])
}

package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	billingcommand "github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	billingquery "github.com/nikolaymatrosov/nvelope/internal/billing/app/query"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// handleListPlans returns the published plan catalog.
func (s *Server) handleListPlans(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, iamdomain.PermBillingGet); !ok {
		return
	}
	view, err := s.billing.Queries.ListPlans.Handle(r.Context(), billingquery.ListPlans{})
	if err != nil {
		s.fail(w, "list plans", err)
		return
	}
	plans := make([]map[string]any, 0, len(view.Plans))
	for _, p := range view.Plans {
		plans = append(plans, map[string]any{
			"id":                p.ID,
			"code":              p.Code,
			"name":              p.Name,
			"priceMinor":        p.PriceMinor,
			"currency":          p.Currency,
			"billingPeriod":     p.BillingPeriod,
			"includedSends":     p.IncludedSends,
			"overageMode":       p.OverageMode,
			"overagePriceMinor": p.OveragePriceMinor,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": plans})
}

// handleSubscribe subscribes the tenant to a plan, charging the first invoice
// synchronously.
func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermBillingManage)
	if !ok {
		return
	}
	var req struct {
		PlanID string `json:"planId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.billing.Commands.Subscribe.Handle(r.Context(), billingcommand.Subscribe{
		TenantID: ws.ID, ActorID: principal.ActorID(), PlanID: req.PlanID,
	})
	if err != nil {
		s.fail(w, "subscribe", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"subscription": map[string]any{
			"id":                 res.SubscriptionID,
			"planId":             res.PlanID,
			"state":              res.State,
			"currentPeriodStart": res.CurrentPeriodStart,
			"currentPeriodEnd":   res.CurrentPeriodEnd,
			"cancelAtPeriodEnd":  res.CancelAtPeriodEnd,
		},
		"invoice": map[string]any{
			"id":         res.InvoiceID,
			"status":     res.InvoiceStatus,
			"totalMinor": res.InvoiceTotalMinor,
			"currency":   res.InvoiceCurrency,
		},
	})
}

// handleGetSubscription returns the tenant's current subscription with usage.
func (s *Server) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermBillingGet); !ok {
		return
	}
	view, err := s.billing.Queries.GetSubscription.Handle(r.Context(),
		billingquery.GetSubscription{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "get subscription", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subscription": map[string]any{
			"id": view.ID,
			"plan": map[string]any{
				"id":          view.Plan.ID,
				"code":        view.Plan.Code,
				"name":        view.Plan.Name,
				"overageMode": view.Plan.OverageMode,
			},
			"state":              view.State,
			"currentPeriodStart": view.CurrentPeriodStart,
			"currentPeriodEnd":   view.CurrentPeriodEnd,
			"cancelAtPeriodEnd":  view.CancelAtPeriodEnd,
		},
		"usage": map[string]any{
			"includedSends":  view.Usage.IncludedSends,
			"usedSends":      view.Usage.UsedSends,
			"overageSends":   view.Usage.OverageSends,
			"remainingSends": view.Usage.RemainingSends,
		},
	})
}

// handleCancelSubscription cancels the tenant's subscription at period end.
func (s *Server) handleCancelSubscription(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermBillingManage)
	if !ok {
		return
	}
	if err := s.billing.Commands.CancelSubscription.Handle(r.Context(), billingcommand.CancelSubscription{
		TenantID: ws.ID, ActorID: principal.ActorID(),
	}); err != nil {
		s.fail(w, "cancel subscription", err)
		return
	}
	view, err := s.billing.Queries.GetSubscription.Handle(r.Context(),
		billingquery.GetSubscription{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "cancel subscription", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subscription": map[string]any{
			"id":                 view.ID,
			"planId":             view.Plan.ID,
			"state":              view.State,
			"currentPeriodStart": view.CurrentPeriodStart,
			"currentPeriodEnd":   view.CurrentPeriodEnd,
			"cancelAtPeriodEnd":  view.CancelAtPeriodEnd,
		},
	})
}

// handleListInvoices returns a page of the tenant's invoices.
func (s *Server) handleListInvoices(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermBillingGet); !ok {
		return
	}
	limit, offset := 50, 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			offset = n
		}
	}
	view, err := s.billing.Queries.ListInvoices.Handle(r.Context(), billingquery.ListInvoices{
		TenantID: ws.ID, Limit: limit, Offset: offset,
	})
	if err != nil {
		s.fail(w, "list invoices", err)
		return
	}
	invoices := make([]map[string]any, 0, len(view.Invoices))
	for _, i := range view.Invoices {
		invoices = append(invoices, map[string]any{
			"id":          i.ID,
			"periodStart": i.PeriodStart,
			"periodEnd":   i.PeriodEnd,
			"totalMinor":  i.TotalMinor,
			"currency":    i.Currency,
			"status":      i.Status,
			"issuedAt":    i.IssuedAt,
			"paidAt":      i.PaidAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"invoices": invoices, "total": view.Total})
}

// handleGetInvoice returns one invoice with its line items and payment attempts.
func (s *Server) handleGetInvoice(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermBillingGet); !ok {
		return
	}
	view, err := s.billing.Queries.GetInvoice.Handle(r.Context(), billingquery.GetInvoice{
		TenantID: ws.ID, InvoiceID: chi.URLParam(r, "id"),
	})
	if err != nil {
		s.fail(w, "get invoice", err)
		return
	}
	writeJSON(w, http.StatusOK, invoicePayload(view))
}

// handleSettleInvoice charges an outstanding invoice immediately, reinstating a
// suspended subscription on success.
func (s *Server) handleSettleInvoice(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermBillingManage)
	if !ok {
		return
	}
	res, err := s.billing.Commands.SettleInvoice.Handle(r.Context(), billingcommand.SettleInvoice{
		TenantID: ws.ID, ActorID: principal.ActorID(), InvoiceID: chi.URLParam(r, "id"),
	})
	if err != nil {
		s.fail(w, "settle invoice", err)
		return
	}
	view, err := s.billing.Queries.GetInvoice.Handle(r.Context(), billingquery.GetInvoice{
		TenantID: ws.ID, InvoiceID: res.InvoiceID,
	})
	if err != nil {
		s.fail(w, "settle invoice", err)
		return
	}
	writeJSON(w, http.StatusOK, invoicePayload(view))
}

// invoicePayload renders a full invoice view as a JSON object.
func invoicePayload(view billingquery.InvoiceView) map[string]any {
	lineItems := make([]map[string]any, 0, len(view.LineItems))
	for _, li := range view.LineItems {
		lineItems = append(lineItems, map[string]any{
			"kind":           li.Kind,
			"description":    li.Description,
			"quantity":       li.Quantity,
			"unitPriceMinor": li.UnitPriceMinor,
			"amountMinor":    li.AmountMinor,
		})
	}
	attempts := make([]map[string]any, 0, len(view.PaymentAttempts))
	for _, a := range view.PaymentAttempts {
		attempts = append(attempts, map[string]any{
			"attemptNumber":    a.AttemptNumber,
			"status":           a.Status,
			"gatewayReference": a.GatewayReference,
			"failureReason":    a.FailureReason,
			"createdAt":        a.CreatedAt,
		})
	}
	return map[string]any{
		"id":              view.ID,
		"subscriptionId":  view.SubscriptionID,
		"periodStart":     view.PeriodStart,
		"periodEnd":       view.PeriodEnd,
		"totalMinor":      view.TotalMinor,
		"currency":        view.Currency,
		"status":          view.Status,
		"attemptCount":    view.AttemptCount,
		"nextAttemptAt":   view.NextAttemptAt,
		"issuedAt":        view.IssuedAt,
		"paidAt":          view.PaidAt,
		"lineItems":       lineItems,
		"paymentAttempts": attempts,
	}
}

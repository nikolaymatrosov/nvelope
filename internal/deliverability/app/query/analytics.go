package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// GetCampaignAnalytics is the request for one campaign's analytics view.
type GetCampaignAnalytics struct {
	TenantID   string
	CampaignID string
}

// CampaignAnalyticsView is one campaign's analytics shaped for the API.
// HasData is false before the first refresh — the counts are then all zero and
// RefreshedAt is the zero time.
type CampaignAnalyticsView struct {
	CampaignID  string
	Counts      domain.Counts
	HasData     bool
	RefreshedAt time.Time
}

// GetCampaignAnalyticsHandler handles GetCampaignAnalytics.
type GetCampaignAnalyticsHandler struct {
	analytics domain.AnalyticsRepository
}

// NewGetCampaignAnalyticsHandler builds the handler, failing fast on a nil
// dependency.
func NewGetCampaignAnalyticsHandler(analytics domain.AnalyticsRepository) GetCampaignAnalyticsHandler {
	if analytics == nil {
		panic("nil dependency")
	}
	return GetCampaignAnalyticsHandler{analytics: analytics}
}

// Handle returns one campaign's analytics. A campaign with no analytics row
// yet returns a zero-count view with HasData false.
func (h GetCampaignAnalyticsHandler) Handle(ctx context.Context, q GetCampaignAnalytics) (
	CampaignAnalyticsView, error) {

	a, ok, err := h.analytics.GetCampaign(ctx, q.TenantID, q.CampaignID)
	if err != nil {
		return CampaignAnalyticsView{}, err
	}
	return CampaignAnalyticsView{
		CampaignID:  q.CampaignID,
		Counts:      a.Counts,
		HasData:     ok,
		RefreshedAt: a.RefreshedAt,
	}, nil
}

// GetDashboard is the request for the workspace deliverability dashboard.
type GetDashboard struct {
	TenantID string
}

// DashboardView is the workspace dashboard shaped for the API.
type DashboardView struct {
	Totals domain.Counts
	Recent []domain.RecentCampaign
}

// GetDashboardHandler handles GetDashboard.
type GetDashboardHandler struct {
	analytics domain.AnalyticsRepository
}

// NewGetDashboardHandler builds the handler, failing fast on a nil dependency.
func NewGetDashboardHandler(analytics domain.AnalyticsRepository) GetDashboardHandler {
	if analytics == nil {
		panic("nil dependency")
	}
	return GetDashboardHandler{analytics: analytics}
}

// Handle returns the workspace dashboard.
func (h GetDashboardHandler) Handle(ctx context.Context, q GetDashboard) (DashboardView, error) {
	d, err := h.analytics.GetDashboard(ctx, q.TenantID)
	if err != nil {
		return DashboardView{}, err
	}
	return DashboardView{Totals: d.Totals, Recent: d.Recent}, nil
}

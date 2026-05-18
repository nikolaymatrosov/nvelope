package domain

import "time"

// Counts holds the six per-campaign delivery counts. It is a read-model value
// type with no invariants; the rate methods derive performance ratios on read.
type Counts struct {
	Sent       int
	Delivered  int
	Opened     int
	Clicked    int
	Bounced    int
	Complained int
}

// rate divides num by denom, yielding 0.0 for a zero denominator.
func rate(num, denom int) float64 {
	if denom <= 0 {
		return 0
	}
	return float64(num) / float64(denom)
}

// OpenRate is the fraction of delivered messages that were opened.
func (c Counts) OpenRate() float64 { return rate(c.Opened, c.Delivered) }

// ClickRate is the fraction of delivered messages that were clicked.
func (c Counts) ClickRate() float64 { return rate(c.Clicked, c.Delivered) }

// BounceRate is the fraction of sent messages that bounced.
func (c Counts) BounceRate() float64 { return rate(c.Bounced, c.Sent) }

// ComplaintRate is the fraction of sent messages that drew a complaint.
func (c Counts) ComplaintRate() float64 { return rate(c.Complained, c.Sent) }

// CampaignAnalytics is one campaign's pre-computed roll-up, served from the
// campaign_analytics summary table. It is a read-model value type built by the
// analytics query handler — it has no mutating behaviour.
type CampaignAnalytics struct {
	CampaignID  string
	Counts      Counts
	RefreshedAt time.Time
}

// RecentCampaign is one campaign's summary line on the workspace dashboard.
type RecentCampaign struct {
	CampaignID string
	Name       string
	Counts     Counts
}

// Dashboard is the workspace-level deliverability summary: the tenant totals
// across every campaign and the most recently sent campaigns.
type Dashboard struct {
	Totals Counts
	Recent []RecentCampaign
}

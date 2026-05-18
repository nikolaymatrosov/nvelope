package domain

import "time"

// BounceSettings is a tenant's bounce-action configuration: whether a hard
// bounce and a complaint each suppress the address. It is a tenant-plane
// entity reached only through its repository's RLS-bound transaction; a row is
// created lazily, and until then DefaultBounceSettings applies.
type BounceSettings struct {
	tenantID           string
	suppressHardBounce bool
	suppressComplaint  bool
	updatedAt          time.Time
}

// NewBounceSettings builds a tenant's bounce settings from the two toggles.
func NewBounceSettings(tenantID string, suppressHardBounce, suppressComplaint bool) *BounceSettings {
	return &BounceSettings{
		tenantID:           tenantID,
		suppressHardBounce: suppressHardBounce,
		suppressComplaint:  suppressComplaint,
	}
}

// DefaultBounceSettings returns the bounce settings applied to a tenant with no
// bounce_settings row: both suppression toggles on.
func DefaultBounceSettings(tenantID string) *BounceSettings {
	return &BounceSettings{
		tenantID:           tenantID,
		suppressHardBounce: true,
		suppressComplaint:  true,
	}
}

// HydrateBounceSettings reconstructs bounce settings from a persisted row.
// Persistence only — it performs no validation and is not a constructor.
func HydrateBounceSettings(tenantID string, suppressHardBounce, suppressComplaint bool,
	updatedAt time.Time) *BounceSettings {

	return &BounceSettings{
		tenantID: tenantID, suppressHardBounce: suppressHardBounce,
		suppressComplaint: suppressComplaint, updatedAt: updatedAt,
	}
}

// TenantID returns the owning tenant's id.
func (s *BounceSettings) TenantID() string { return s.tenantID }

// SuppressOnHardBounce reports whether a hard bounce suppresses the address.
func (s *BounceSettings) SuppressOnHardBounce() bool { return s.suppressHardBounce }

// SuppressOnComplaint reports whether a complaint suppresses the address.
func (s *BounceSettings) SuppressOnComplaint() bool { return s.suppressComplaint }

// UpdatedAt returns when the settings were last changed.
func (s *BounceSettings) UpdatedAt() time.Time { return s.updatedAt }

// ShouldSuppressHardBounce reports whether a hard bounce should suppress under
// these settings.
func (s *BounceSettings) ShouldSuppressHardBounce() bool { return s.suppressHardBounce }

// ShouldSuppressComplaint reports whether a complaint should suppress under
// these settings.
func (s *BounceSettings) ShouldSuppressComplaint() bool { return s.suppressComplaint }

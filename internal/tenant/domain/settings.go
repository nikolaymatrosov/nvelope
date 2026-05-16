package domain

import (
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// TenantSettings is a tenant's per-workspace configuration. It is the first
// tenant-plane entity — reached only through the Row-Level-Security-bound
// transaction owned by the settings adapter.
type TenantSettings struct {
	tenantID    string
	displayName string
	timezone    string
}

// NewTenantSettings builds the initial settings row created with a tenant. The
// timezone is left to its database-level default until first changed.
func NewTenantSettings(tenantID, displayName string) (*TenantSettings, error) {
	if tenantID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a tenant is required")
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "display name is required")
	}
	return &TenantSettings{tenantID: tenantID, displayName: displayName}, nil
}

// HydrateTenantSettings reconstructs settings from a persisted row.
// Persistence only — it is not a constructor.
func HydrateTenantSettings(tenantID, displayName, timezone string) *TenantSettings {
	return &TenantSettings{tenantID: tenantID, displayName: displayName, timezone: timezone}
}

// TenantID returns the owning tenant's id.
func (s *TenantSettings) TenantID() string { return s.tenantID }

// DisplayName returns the workspace display name.
func (s *TenantSettings) DisplayName() string { return s.displayName }

// Timezone returns the workspace timezone.
func (s *TenantSettings) Timezone() string { return s.timezone }

// Rename changes the workspace display name, rejecting an empty value.
func (s *TenantSettings) Rename(displayName string) error {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return apperr.NewIncorrectInput("validation_failed", "display name is required")
	}
	s.displayName = displayName
	return nil
}

// SetTimezone changes the workspace timezone, rejecting an empty value.
func (s *TenantSettings) SetTimezone(timezone string) error {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return apperr.NewIncorrectInput("validation_failed", "timezone is required")
	}
	s.timezone = timezone
	return nil
}

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

	rawHTMLSanitization RawHTMLSanitization
}

// RawHTMLSanitization is how a tenant's raw (non-visual) campaign and
// template HTML is treated on save.
type RawHTMLSanitization string

const (
	// RawHTMLStandard strips active content (scripts, frames, forms, event
	// handlers, unsafe URLs) and keeps the rest of the email document.
	RawHTMLStandard RawHTMLSanitization = "standard"
	// RawHTMLOff stores raw HTML verbatim. It is reachable only by explicitly
	// accepting the risk.
	RawHTMLOff RawHTMLSanitization = "off"
)

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
	return &TenantSettings{tenantID: tenantID, displayName: displayName, rawHTMLSanitization: RawHTMLStandard}, nil
}

// HydrateTenantSettings reconstructs settings from a persisted row.
// Persistence only — it is not a constructor.
func HydrateTenantSettings(tenantID, displayName, timezone string,
	rawHTMLSanitization RawHTMLSanitization) *TenantSettings {
	return &TenantSettings{tenantID: tenantID, displayName: displayName,
		timezone: timezone, rawHTMLSanitization: rawHTMLSanitization}
}

// RawHTMLSanitization returns how raw campaign and template HTML is treated
// on save.
func (s *TenantSettings) RawHTMLSanitization() RawHTMLSanitization { return s.rawHTMLSanitization }

// SetRawHTMLSanitization changes the raw-HTML level. Turning sanitization
// off requires acceptRisk; turning it back on never does.
func (s *TenantSettings) SetRawHTMLSanitization(level RawHTMLSanitization, acceptRisk bool) error {
	switch level {
	case RawHTMLStandard:
	case RawHTMLOff:
		if !acceptRisk {
			return ErrRiskNotAccepted
		}
	default:
		return apperr.NewIncorrectInput("validation_failed",
			"raw HTML sanitization must be standard or off")
	}
	s.rawHTMLSanitization = level
	return nil
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

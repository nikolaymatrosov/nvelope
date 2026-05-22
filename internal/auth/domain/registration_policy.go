package domain

import "strings"

// RegistrationPolicy decides whether an email address may register, based on a
// configured allowlist of email domains. An empty allowlist leaves
// registration unrestricted.
type RegistrationPolicy struct {
	allowed map[string]bool
}

// NewRegistrationPolicy builds a policy from a list of permitted email domains.
// Each entry is trimmed and lower-cased; blank entries are dropped. An empty
// resulting set means registration is unrestricted.
func NewRegistrationPolicy(domains []string) RegistrationPolicy {
	allowed := make(map[string]bool, len(domains))
	for _, d := range domains {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			allowed[d] = true
		}
	}
	return RegistrationPolicy{allowed: allowed}
}

// IsRestricted reports whether the policy limits registration at all.
func (p RegistrationPolicy) IsRestricted() bool { return len(p.allowed) > 0 }

// Allows reports whether email may register under this policy. An unrestricted
// policy admits any address; otherwise the email's domain must be on the
// allowlist. The comparison is case-insensitive.
func (p RegistrationPolicy) Allows(email Email) bool {
	if len(p.allowed) == 0 {
		return true
	}
	return p.allowed[email.Domain()]
}

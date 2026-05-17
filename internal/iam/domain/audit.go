package domain

import "time"

// AuditRecord is an append-only record of a privileged action — role and API
// key management — written for attributability (FR-028).
type AuditRecord struct {
	ID        string
	TenantID  string
	ActorID   string
	ActorKind PrincipalKind
	Action    string
	Target    string
	Metadata  map[string]any
	CreatedAt time.Time
}

// NewAuditRecord builds an audit record for a privileged action.
func NewAuditRecord(tenantID, actorID string, actorKind PrincipalKind,
	action, target string, metadata map[string]any) AuditRecord {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return AuditRecord{
		TenantID: tenantID, ActorID: actorID, ActorKind: actorKind,
		Action: action, Target: target, Metadata: metadata,
	}
}

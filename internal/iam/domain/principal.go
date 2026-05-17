package domain

// PrincipalKind is the kind of credential a principal was resolved from.
type PrincipalKind string

const (
	// PrincipalSession is a principal resolved from a workspace session.
	PrincipalSession PrincipalKind = "session"
	// PrincipalAPIKey is a principal resolved from a scoped API key.
	PrincipalAPIKey PrincipalKind = "api-key"
)

// Principal is the resolved actor of a request: who they are, the tenant they
// act in, their tenant-level permissions, and their per-list role permissions.
// It is a value object built fresh per request, so a role change takes effect
// on the holder's next request.
type Principal struct {
	kind              PrincipalKind
	tenantID          string
	actorID           string
	tenantPermissions []Permission
	listPermissions   map[string][]Permission
}

// NewPrincipal builds a principal from resolved credential data.
func NewPrincipal(kind PrincipalKind, tenantID, actorID string,
	tenantPermissions []Permission, listPermissions map[string][]Permission) Principal {
	if listPermissions == nil {
		listPermissions = map[string][]Permission{}
	}
	return Principal{
		kind: kind, tenantID: tenantID, actorID: actorID,
		tenantPermissions: tenantPermissions, listPermissions: listPermissions,
	}
}

// Kind returns the credential kind the principal was resolved from.
func (p Principal) Kind() PrincipalKind { return p.kind }

// TenantID returns the tenant the principal acts in.
func (p Principal) TenantID() string { return p.tenantID }

// ActorID returns the resolved actor's id — a user id for a session, an API
// key id for an API key.
func (p Principal) ActorID() string { return p.actorID }

// TenantPermissions returns the principal's tenant-level permissions.
func (p Principal) TenantPermissions() []Permission { return p.tenantPermissions }

// Can reports whether the principal holds a tenant-level permission — the
// check for an action that does not target a specific list.
func (p Principal) Can(required Permission) bool {
	return contains(p.tenantPermissions, required)
}

// CanOnList reports whether the principal may perform an action targeting list
// listID: the union of tenant-level permissions and the per-list role's
// permissions for that list (contracts/permissions.md).
func (p Principal) CanOnList(required Permission, listID string) bool {
	if contains(p.tenantPermissions, required) {
		return true
	}
	return contains(p.listPermissions[listID], required)
}

// contains reports whether perms includes target.
func contains(perms []Permission, target Permission) bool {
	for _, p := range perms {
		if p == target {
			return true
		}
	}
	return false
}

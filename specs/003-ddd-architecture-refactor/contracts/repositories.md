# Contract: Domain-owned repository & service interfaces

These interfaces are declared by the consuming layer (`<ctx>/domain` or
`<ctx>/app`) and implemented in `<ctx>/adapters` (FR-004, constitution
Principle VI). Signatures below are the *intended shape* — exact parameter and
return spelling is settled during implementation, but the ownership direction
and the operations are fixed. Domain entities are the types named here (`*User`,
`*Tenant`, …); persistence-shaped structs stay private to the adapters (FR-014).

## auth context

```go
// auth/domain/repository.go

type UserRepository interface {
    Create(ctx context.Context, u *User, passwordHash string) error // ErrEmailTaken on conflict
    GetByID(ctx context.Context, id string) (*User, error)          // ErrUserNotFound
    LookupByEmail(ctx context.Context, email string) (*User, error) // ErrUserNotFound
    // GetCredentials returns the user and bcrypt hash for an email, or
    // ErrUserNotFound. The hash never leaves the adapter/app boundary.
    GetCredentials(ctx context.Context, email string) (*User, string, error)
}

type SessionRepository interface {
    Issue(ctx context.Context, s *Session, tokenHash string) error
    ResolveByTokenHash(ctx context.Context, tokenHash string) (*Session, error) // ErrSessionInvalid
    RevokeByTokenHash(ctx context.Context, tokenHash string) error              // no-op if unknown
}
```

```go
// auth/app — declared by the app layer because hashing is a use-case concern.
type PasswordHasher interface {
    Hash(plaintext string) (string, error)
    Verify(hash, plaintext string) bool
}
```

**Transactional note**: `SignUp` creates a user *and* issues a session
atomically. The `UserRepository` exposes a closure-style transactional create
(`CreateWithSession(ctx, u, hash, func(userID) (*Session, tokenHash, error))`),
or the app layer composes the two repositories under one transaction supplied by
the adapter — settled in implementation; either way the atomicity of today's
`auth.Signup` is preserved.

## tenant context

```go
// tenant/domain/repository.go

type TenantRepository interface {
    // CreateWorkspace inserts the tenant, the owner Membership, and the initial
    // TenantSettings row in ONE transaction. The settings insert runs after the
    // new tenant id is bound to app.tenant_id so the RLS WITH CHECK passes.
    CreateWorkspace(ctx context.Context, t *Tenant, ownerID string, settings *TenantSettings) error // ErrSlugTaken
    GetBySlug(ctx context.Context, slug string) (*Tenant, error) // ErrTenantNotFound
    GetByID(ctx context.Context, id string) (*Tenant, error)     // ErrTenantNotFound
    ListMembershipsForUser(ctx context.Context, userID string) ([]Membership, error)
    GetMembershipRole(ctx context.Context, userID, tenantID string) (Role, error) // ErrNotMember
    AddMembership(ctx context.Context, m *Membership) error      // idempotent
    ListMembers(ctx context.Context, tenantID string) ([]Membership, error)
}

type InvitationRepository interface {
    Create(ctx context.Context, inv *Invitation, tokenHash string) error // ErrInvitationExists
    GetPendingByTokenHash(ctx context.Context, tokenHash string) (*Invitation, error) // ErrInvitationNotFound
    ListPending(ctx context.Context, tenantID string) ([]Invitation, error)
    // Update loads the invitation, runs fn, persists the result — the closure is
    // the transaction boundary (used by AcceptInvitation, RevokeInvitation).
    Update(ctx context.Context, id, tenantID string,
        fn func(*Invitation) (*Invitation, error)) error
}

type SettingsRepository interface {
    // Get and Update both run inside a WithTenant transaction (app.tenant_id
    // bound) — the adapter owns the RLS binding; the app layer never sees it.
    Get(ctx context.Context, tenantID string) (*TenantSettings, error) // ErrTenantNotFound
    Update(ctx context.Context, tenantID string,
        fn func(*TenantSettings) (*TenantSettings, error)) error
}
```

## Ownership & substitution rules

- The interface lives with its **consumer**; the pgx implementation lives in
  `<ctx>/adapters`. The app layer depends on the interface only.
- Adapters translate `pgx.ErrNoRows` and SQLSTATE codes (`23505` unique,
  `22P02` invalid-input) into the typed domain errors named above — the app and
  transport layers never see a raw `pgx` error.
- `internal/service.NewApplication` wires the pgx adapters; a parallel test
  constructor wires in-memory fakes implementing the same interfaces, so app and
  domain tests need no database (the substitution path is the same wiring).
- Cross-tenant isolation (`SettingsRepository`) is exercised by the unchanged
  real-database `test/isolation_test.go` as `nvelope_app`.

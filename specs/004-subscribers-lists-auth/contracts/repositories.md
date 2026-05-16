# Contract — Domain-Owned Repository & Service Interfaces

Every interface below is **declared by the consumer** — the `domain` or `app`
package that needs it — and **implemented by an adapter** (constitution VI).
Signatures are illustrative Go; the binding rule is the dependency direction and
the closure-based update boundary, not the exact names.

## Update closure pattern

Mutating repositories expose an `Update` taking a closure; the closure *is* the
transaction boundary. For tenant-plane repositories the transaction is also the
RLS-bound `WithTenant` transaction (research.md Decision 3), so a single
transaction both isolates the tenant and brackets the change.

```go
UpdateSubscriber(ctx context.Context, tenantID, id string,
    fn func(*subscriber.Subscriber) (*subscriber.Subscriber, error)) error
```

## `audience` domain interfaces

```go
// Declared in internal/audience/domain — implemented in audience/adapters.

type ListRepository interface {
    Add(ctx, tenantID string, l *List) error
    Update(ctx, tenantID, id string, fn func(*List) (*List, error)) error
    Delete(ctx, tenantID, id string) error
    Get(ctx, tenantID, id string) (*List, error)
    All(ctx, tenantID string, page Page) ([]*List, int, error)
}

type SubscriberRepository interface {
    Add(ctx, tenantID string, s *Subscriber) error
    Update(ctx, tenantID, id string, fn func(*Subscriber) (*Subscriber, error)) error
    UpsertByEmail(ctx, tenantID string, s *Subscriber) (created bool, err error)
    Delete(ctx, tenantID, id string) error
    Get(ctx, tenantID, id string) (*Subscriber, error)
    Search(ctx, tenantID, q string, page Page) ([]*Subscriber, int, error)
    // RunSegment translates a validated Segment to parameterized SQL.
    RunSegment(ctx, tenantID string, seg Segment, page Page) ([]*Subscriber, int, error)
    CountSegment(ctx, tenantID string, seg Segment) (int, error)
}

type MembershipRepository interface {
    Attach(ctx, tenantID, subscriberID, listID string, status SubscriptionStatus) error
    Detach(ctx, tenantID, subscriberID, listID string) error
    SetStatus(ctx, tenantID, subscriberID, listID string, status SubscriptionStatus) error
    ForSubscriber(ctx, tenantID, subscriberID string) ([]Membership, error)
}

type JobRepository interface {
    Add(ctx, tenantID string, j *ImportJob /* or *ExportJob */) error
    Update(ctx, tenantID, id string, fn ...) error   // counts, status, staged result
    Get(ctx, tenantID, id string) (Job, error)
    StagedFile(ctx, tenantID, id string) ([]byte, error)
}
```

The import/export **workers** are River workers (in `audience/adapters`); the
`StartImport` / `StartExport` commands depend on a small `JobEnqueuer` interface
(declared in `audience/app`) implemented by `internal/platform/jobs`.

## `iam` domain interfaces

```go
// Declared in internal/iam/domain — implemented in iam/adapters.

type UserRepository interface {
    Add(ctx, tenantID string, u *TenantUser) error
    Update(ctx, tenantID, id string, fn func(*TenantUser) (*TenantUser, error)) error
    Get(ctx, tenantID, id string) (*TenantUser, error)
    ByPlatformUser(ctx, tenantID, platformUserID string) (*TenantUser, error)
}

type SessionRepository interface {
    Add(ctx, tenantID string, s *Session) error
    Update(ctx, tenantID, id string, fn func(*Session) (*Session, error)) error
    ByTokenHash(ctx, tenantID, tokenHash string) (*Session, error)
}

type RoleRepository interface {
    Add(ctx, tenantID string, r *Role) error
    Update(ctx, tenantID, id string, fn func(*Role) (*Role, error)) error
    Delete(ctx, tenantID, id string) error
    Get(ctx, tenantID, id string) (*Role, error)
    All(ctx, tenantID string) ([]*Role, error)
    AssignTenantRole(ctx, tenantID, userID, roleID string) error
    AssignListRole(ctx, tenantID, userID, listID, roleID string) error
    RemoveListRole(ctx, tenantID, userID, listID string) error
    // EffectiveFor loads the data the Principal needs in one round trip.
    EffectiveFor(ctx, tenantID, userID string) (tenantPerms []Permission,
        listPerms map[string][]Permission, err error)
}

type APIKeyRepository interface {
    Add(ctx, tenantID string, k *APIKey) error
    Revoke(ctx, tenantID, id string) error
    Get(ctx, tenantID, id string) (*APIKey, error)
    ByTokenHash(ctx, tenantID, tokenHash string) (*APIKey, error)
    All(ctx, tenantID string) ([]*APIKey, error)
}

type RecoveryCodeRepository interface {
    Replace(ctx, tenantID, userID string, codeHashes []string) error
    Consume(ctx, tenantID, userID, codeHash string) (ok bool, err error)
}

type AuditRepository interface {
    Record(ctx, tenantID string, r AuditRecord) error
    All(ctx, tenantID string, page Page) ([]AuditRecord, int, error)
}

// TOTP is an external-capability interface declared by iam/app, implemented by
// an adapter over pquerna/otp; it also encrypts/decrypts the stored secret.
type TOTP interface {
    NewSecret() (secret string, provisioningURI string, err error)
    Validate(secret, code string) bool
    Encrypt(secret string) ([]byte, error)
    Decrypt(blob []byte) (string, error)
}
```

## Shared platform interfaces

```go
// internal/platform/tenantdb — the RLS-bound transaction helper, exported so
// the tenant, iam, and audience adapters share one implementation.
func WithTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string,
    fn func(ctx context.Context, tx pgx.Tx) error) error

// internal/platform/jobs — River client construction + enqueue/registration.
// audience/app depends only on the narrow JobEnqueuer it declares for itself.
```

## Dependency-direction rule

`adapters → app → domain`; `domain` imports nothing from the other layers.
Every interface above lives in `domain` (or `app` for the
external-capability/enqueuer interfaces) and is satisfied by a type in
`adapters`. The `go-cleanarch` CI linter (added in 003) continues to enforce
this; the two new contexts are added to its checked set.

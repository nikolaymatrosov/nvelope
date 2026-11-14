# Serverless On-Prem — Plan B2 (auth context on YDB) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- Port the `auth` context to YDB: `YDBUsers`, `YDBSessions` and `YDBEmailVerifications` behind the existing domain ports.
- Hold each new repository to the same contract tests as its Postgres counterpart.

**Architecture:**
- **Migration.** One file, `internal/db/ydbmigrations/000003_auth.up.sql`, creates `platform_users`, `platform_sessions` and `email_verification_tokens`.
- **Adapters.** The YDB adapters sit next to the pg adapters and call `db.Query()` / `DoTx` directly, as `internal/sending/adapters/domains_ydb.go` does.
- **Contract tests.** The existing `*_pg_test.go` files become pg+ydb contract suites through the `eachBackend` harness that Plan B1 introduced for `sending`.
- **Case-insensitive email.** Postgres gets it from `citext`. YDB stores the address as entered plus a lower-cased `email_key` that carries the unique index.
- **No cleanup job.** Sessions and verification tokens expire through table TTL, which replaces the cleanup job.

**Tech Stack:**
- Go 1.26
- `github.com/ydb-platform/ydb-go-sdk/v3` v3.136.4 (query service)
- testcontainers-go, local-ydb `26.3.1.23`
- testify, `github.com/google/uuid`

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md).
- The binding sections are "Storage layer (YDB)", "SQL translation rules", "Spike findings (Plan A)" (V4, V6) and "Plan B2 planning spike".
- The spike verified every YQL construct this plan uses that B1 did not: `VIEW` reads, `DELETE ... ON SELECT`, `UPDATE ... RETURNING`, the in-transaction INSERT conflict, and read-after-write in one `DoTx`.

**Scope:**
- This is the second Plan B plan. The spec's port order is now auth, tenant, iam, audience, campaign, deliverability, media. Auth moved ahead of tenant because `Tenants.ListMembers` joins `platform_users`.
- Not in scope:
  - Wiring the YDB adapters into the composition root (Plan C).
  - The resend throttle: `ydbthrottle.Keyed` already matches `domain.ResendThrottle`, and Plan C wires it.
  - The verification workers. Neither touches the database through a port this plan adds.

## Global Constraints

- **Branch.**
  - Work on `feat/serverless-onprem-b2` in the worktree `/Users/nikthespirit/experiments/nvelope/.claude/worktrees/serverless-onprem-b2`. Both already exist.
  - Do not create branches or worktrees.
  - The main checkout belongs to other sessions.
- **Docker.**
  - Docker on this machine is OrbStack. Prefix every `go test` / `make test` / `make ci` with `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && `.
  - Ryuk reaps test containers between runs, so every `go test` run starts fresh containers.
- **YDB container.** It is `ydbplatform/local-ydb:26.3.1.23`, amd64 under Rosetta. A fresh container takes about 45 s to start, so use `-timeout 8m` on runs that touch YDB, and `-timeout 15m` for the full suite.
- **Known environmental flakes. Neither is caused by this plan:**
  - `internal/platform/ratelimit` `TestAllowIsConsistentUnderConcurrency` can fail with `redis eval failed: i/o timeout` under full-suite load. Re-run that package alone and record both runs.
  - pg-backed packages can all fail with `no migration found for version N`, where N is above `22`. That means another worktree's shared `nvelope-test-pg` container is in use. Re-run once it is gone, and never `docker rm` it yourself.
- **Dependencies.** No new Go modules and no `go.mod` change.
- **SaaS must not change.** Postgres adapters, River, Redis and every existing test keep their behaviour. `go test ./...` passes after every task.
- **YQL conventions** (spec "SQL translation rules" and V4):
  - Every query `DECLARE`s each parameter: `DECLARE $id AS Utf8;`.
  - Literals that target a `Utf8` column take the `u` suffix (`""u`); a bare `"..."` is `String`.
  - Every DDL statement is idempotent (`CREATE TABLE IF NOT EXISTS`).
  - A statement ends on a line ending in `;`. Never end a line with `;` inside a string literal or a `--` comment.
  - A secondary index is read only through an explicit `VIEW <index_name>`.
- **Control-plane tables.** The auth tables are not tenant-scoped (Postgres gives them no RLS either), so they have no `tenant_id` column and no tenant predicate.
- **IDs** are UUIDv4 strings generated in Go (`uuid.NewString()`) and stored as `Utf8`.
- **Email matching.** `platform_users.email` keeps the address as entered. `email_key` holds `strings.ToLower(email)` and carries the `GLOBAL UNIQUE SYNC` index, and every email lookup goes through `email_key`. This reproduces Postgres `citext`.
- **TTL.** `platform_sessions` and `email_verification_tokens` carry `WITH (TTL = Interval("PT0S") ON expires_at)`. `YDBEmailVerifications` therefore has no `DeleteExpiredBefore`, and its pg-only test stays pg-only.
- **`DoTx` closures must be retry-safe** (V6):
  - only database operations inside, plus the pure `issueSession` / `issueVerification` callbacks the port already passes in;
  - values the caller reads are assigned at the end of the closure, or built after `DoTx` returns;
  - never `append` to a slice declared outside the closure.
- **Duplicate key mapping.** "Already exists" comes from `db.IsYDBConflict(err)`. Never match on the message text. Inside `DoTx` the conflict surfaces at the INSERT statement (spec "Plan B2 planning spike").
- **Commits.** Use conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.** `goimports` is not installed, so use `gofmt -w`. `golangci-lint` is on PATH. `make lint-arch` needs `export PATH="$HOME/go/bin:$PATH" && `.

## File structure

| File | Responsibility |
|------|----------------|
| `internal/db/ydbmigrations/000003_auth.up.sql` | `platform_users`, `platform_sessions`, `email_verification_tokens` |
| `internal/auth/adapters/sessions_ydb.go` | `YDBSessions`, the shared `insertYDBSession` helper, `ydbTokenHash` |
| `internal/auth/adapters/email_verifications_ydb.go` | `YDBEmailVerifications`, the shared `insertYDBVerificationToken` helper |
| `internal/auth/adapters/users_ydb.go` | `YDBUsers` |
| `internal/auth/adapters/helpers_test.go` (new) | `backend`, `backends`, `eachBackend`, `persistUser` |
| `internal/auth/adapters/sessions_test.go` (renamed from `sessions_pg_test.go`) | sessions contract suite |
| `internal/auth/adapters/email_verifications_test.go` (renamed from `email_verifications_pg_test.go`) | verifications contract suite + the pg-only `DeleteExpiredBefore` test |
| `internal/auth/adapters/users_test.go` (renamed from `users_pg_test.go`) | users contract suite |

---

### Task 1: The auth schema, `YDBSessions`, `YDBEmailVerifications` and their contract suites

**Files:**
- Create: `internal/db/ydbmigrations/000003_auth.up.sql`
- Create: `internal/auth/adapters/sessions_ydb.go`
- Create: `internal/auth/adapters/email_verifications_ydb.go`
- Create: `internal/auth/adapters/helpers_test.go`
- Rename: `internal/auth/adapters/sessions_pg_test.go` → `internal/auth/adapters/sessions_test.go` (rewritten)
- Rename: `internal/auth/adapters/email_verifications_pg_test.go` → `internal/auth/adapters/email_verifications_test.go` (rewritten)

**Interfaces:**
- Consumes:
  - `dbtest.YDBMigrated(t) *ydb.Driver` (Plan B1, `internal/dbtest/ydb.go`). It applies every `internal/db/ydbmigrations/*.up.sql` once per test binary, so the new file is picked up automatically.
  - `domain.SessionRepository` and `domain.EmailVerificationRepository` (`internal/auth/domain/repository.go`), unchanged.
  - `newUser(t, name) (*domain.User, string)` from `internal/auth/adapters/users_pg_test.go`. It stays there with that signature.
- Produces:
  ```go
  // package adapters (internal/auth/adapters)
  type YDBSessions struct{ /* unexported */ }
  func NewYDBSessions(d *ydb.Driver) *YDBSessions
  func insertYDBSession(ctx context.Context, q query.Executor, s *domain.Session, tokenHash string) error
  func ydbTokenHash(tokenHash string) query.ExecuteOption
  type YDBEmailVerifications struct{ /* unexported */ }
  func NewYDBEmailVerifications(d *ydb.Driver) *YDBEmailVerifications
  func insertYDBVerificationToken(ctx context.Context, q query.Executor, v *domain.EmailVerification, tokenHash string) (*domain.EmailVerification, error)
  // package adapters_test
  type backend struct {
      name          string
      sessions      func(t *testing.T) domain.SessionRepository
      verifications func(t *testing.T) domain.EmailVerificationRepository
      newUserID     func(t *testing.T) string
  }
  func eachBackend(t *testing.T, body func(t *testing.T, b backend))
  func persistUser(t *testing.T, users *adapters.Users) string
  ```
- Task 2 relies on:
  - `insertYDBSession` and `insertYDBVerificationToken` accepting a transaction (`query.TxActor` embeds `query.Executor`);
  - the `platform_users` table and its `idx_platform_users_email_key` index;
  - the harness above.

- [ ] **Step 1: Write the auth migration**

Create `internal/db/ydbmigrations/000003_auth.up.sql`:

```sql
-- Auth (internal/auth): platform users, login sessions and email-verification
-- challenges. Control-plane tables, not tenant-scoped, so there is no
-- tenant_id column.

-- email keeps the address as entered. email_key is its lower-cased form and
-- carries the uniqueness, standing in for the Postgres citext column.
CREATE TABLE IF NOT EXISTS platform_users (
    id                Utf8      NOT NULL,
    email             Utf8      NOT NULL,
    email_key         Utf8      NOT NULL,
    password_hash     Utf8      NOT NULL,
    name              Utf8      NOT NULL,
    locale            Utf8,
    email_verified_at Timestamp,
    created_at        Timestamp NOT NULL,
    updated_at        Timestamp NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_platform_users_email_key GLOBAL UNIQUE SYNC ON (email_key)
);

-- A session is only ever looked up by its token hash, so the hash is the
-- primary key. TTL deletes a session once it has expired.
CREATE TABLE IF NOT EXISTS platform_sessions (
    token_hash       Utf8      NOT NULL,
    id               Utf8      NOT NULL,
    platform_user_id Utf8      NOT NULL,
    created_at       Timestamp NOT NULL,
    expires_at       Timestamp NOT NULL,
    revoked_at       Timestamp,
    PRIMARY KEY (token_hash)
) WITH (TTL = Interval("PT0S") ON expires_at);

-- TTL replaces the verification-cleanup job: a challenge is deleted once its
-- validity window has closed, which is what the job's sweep does on Postgres.
CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id          Utf8      NOT NULL,
    user_id     Utf8      NOT NULL,
    token_hash  Utf8      NOT NULL,
    expires_at  Timestamp NOT NULL,
    created_at  Timestamp NOT NULL,
    consumed_at Timestamp,
    PRIMARY KEY (id),
    INDEX idx_email_verification_tokens_token_hash GLOBAL UNIQUE SYNC ON (token_hash),
    INDEX idx_email_verification_tokens_user GLOBAL SYNC ON (user_id)
) WITH (TTL = Interval("PT0S") ON expires_at);
```

- [ ] **Step 2: Write the harness**

Create `internal/auth/adapters/helpers_test.go`:

```go
package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// persistUser inserts a real platform user through the pg adapter and returns
// its id.
func persistUser(t *testing.T, users *adapters.Users) string {
	t.Helper()
	u, _ := newUser(t, "Session Owner")
	created, err := users.Create(context.Background(), u, "hash")
	require.NoError(t, err)
	return created.ID()
}

// backend is one storage implementation of the auth repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name          string
	sessions      func(t *testing.T) domain.SessionRepository
	verifications func(t *testing.T) domain.EmailVerificationRepository
	// newUserID returns a user id valid for the backend: a real platform_users
	// row for Postgres (sessions and verification tokens carry a foreign key to
	// it), a bare UUID for YDB (no foreign keys).
	newUserID func(t *testing.T) string
}

var backends = []backend{
	{
		name: "pg",
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewSessions(dbtest.AppPool(t))
		},
		verifications: func(t *testing.T) domain.EmailVerificationRepository {
			return adapters.NewEmailVerifications(dbtest.AppPool(t))
		},
		newUserID: func(t *testing.T) string { return persistUser(t, adapters.NewUsers(dbtest.AppPool(t))) },
	},
	{
		name: "ydb",
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewYDBSessions(dbtest.YDBMigrated(t))
		},
		verifications: func(t *testing.T) domain.EmailVerificationRepository {
			return adapters.NewYDBEmailVerifications(dbtest.YDBMigrated(t))
		},
		newUserID: func(*testing.T) string { return uuid.NewString() },
	},
}

// eachBackend runs body once per backend, as parallel subtests named after it.
func eachBackend(t *testing.T, body func(t *testing.T, b backend)) {
	t.Helper()
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			body(t, b)
		})
	}
}
```

- [ ] **Step 3: Rewrite the sessions tests as a contract suite**

```bash
git mv internal/auth/adapters/sessions_pg_test.go internal/auth/adapters/sessions_test.go
```

Replace the contents of `internal/auth/adapters/sessions_test.go` with the code below. `persistUser` moved to `helpers_test.go` in Step 2.

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestSessionsIssueAndResolve(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		sessions := b.sessions(t)
		ctx := context.Background()

		userID := b.newUserID(t)
		s, err := domain.NewSession(userID, time.Hour)
		require.NoError(t, err)

		tokenHash := dbtest.RandString()
		require.NoError(t, sessions.Issue(ctx, s, tokenHash))

		resolved, err := sessions.ResolveByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.NotEmpty(t, resolved.ID(), "the adapter assigns an id")
		require.Equal(t, userID, resolved.UserID())
		require.WithinDuration(t, s.ExpiresAt(), resolved.ExpiresAt(), time.Second)
		require.True(t, resolved.IsLive(time.Now()))
	})
}

func TestSessionsResolveRejectsExpired(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		sessions := b.sessions(t)
		ctx := context.Background()

		expired, err := domain.NewSession(b.newUserID(t), -time.Hour)
		require.NoError(t, err)

		tokenHash := dbtest.RandString()
		require.NoError(t, sessions.Issue(ctx, expired, tokenHash))

		_, err = sessions.ResolveByTokenHash(ctx, tokenHash)
		require.ErrorIs(t, err, domain.ErrSessionInvalid, "an expired session does not resolve")
	})
}

func TestSessionsResolveUnknownToken(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		_, err := b.sessions(t).ResolveByTokenHash(context.Background(), dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrSessionInvalid)
	})
}

func TestSessionsRevoke(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		sessions := b.sessions(t)
		ctx := context.Background()

		s, err := domain.NewSession(b.newUserID(t), time.Hour)
		require.NoError(t, err)

		tokenHash := dbtest.RandString()
		require.NoError(t, sessions.Issue(ctx, s, tokenHash))
		require.NoError(t, sessions.RevokeByTokenHash(ctx, tokenHash))

		_, err = sessions.ResolveByTokenHash(ctx, tokenHash)
		require.ErrorIs(t, err, domain.ErrSessionInvalid, "a revoked session does not resolve")

		// Revoking again, or revoking an unknown token, is a harmless no-op.
		require.NoError(t, sessions.RevokeByTokenHash(ctx, tokenHash))
		require.NoError(t, sessions.RevokeByTokenHash(ctx, dbtest.RandString()))
	})
}
```

- [ ] **Step 4: Rewrite the email-verification tests as a contract suite**

```bash
git mv internal/auth/adapters/email_verifications_pg_test.go internal/auth/adapters/email_verifications_test.go
```

Replace the contents of `internal/auth/adapters/email_verifications_test.go` with the code below.
- Token hashes are now `dbtest.RandString()`, so a repeated run (`-count=N`) on one container cannot collide on the unique hash.
- `TestEmailVerificationsDeleteExpiredBefore` stays a pg-only test. Its body is unchanged; it only gains a doc comment.

```go
package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestEmailVerificationsIssueGetConsume(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.verifications(t)
		ctx := context.Background()
		userID := b.newUserID(t)
		tokenHash := dbtest.RandString()

		v, err := domain.NewEmailVerification(userID, time.Hour)
		require.NoError(t, err)
		issued, err := repo.Issue(ctx, v, tokenHash)
		require.NoError(t, err)
		require.NotEmpty(t, issued.ID(), "the adapter assigns an id")

		got, err := repo.GetByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.Equal(t, issued.ID(), got.ID())
		require.Equal(t, userID, got.UserID())
		require.WithinDuration(t, v.ExpiresAt(), got.ExpiresAt(), time.Second)
		require.False(t, got.IsConsumed())
		require.False(t, got.IsExpired(time.Now()))

		require.NoError(t, repo.Consume(ctx, got.ID(), time.Now()))
		consumed, err := repo.GetByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.True(t, consumed.IsConsumed(), "the row reflects the consumption")

		// Consuming an already-consumed challenge is a harmless no-op.
		require.NoError(t, repo.Consume(ctx, got.ID(), time.Now()))
	})
}

func TestEmailVerificationsIssueSupersedesPending(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.verifications(t)
		ctx := context.Background()
		userID := b.newUserID(t)
		firstHash, secondHash := dbtest.RandString(), dbtest.RandString()

		first, err := domain.NewEmailVerification(userID, time.Hour)
		require.NoError(t, err)
		_, err = repo.Issue(ctx, first, firstHash)
		require.NoError(t, err)

		second, err := domain.NewEmailVerification(userID, time.Hour)
		require.NoError(t, err)
		_, err = repo.Issue(ctx, second, secondHash)
		require.NoError(t, err)

		// Issuing a fresh challenge deletes the prior unconsumed one (FR-012).
		_, err = repo.GetByTokenHash(ctx, firstHash)
		require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid)
		_, err = repo.GetByTokenHash(ctx, secondHash)
		require.NoError(t, err)
	})
}

func TestEmailVerificationsIssueKeepsConsumed(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.verifications(t)
		ctx := context.Background()
		userID := b.newUserID(t)
		consumedHash := dbtest.RandString()

		first, err := domain.NewEmailVerification(userID, time.Hour)
		require.NoError(t, err)
		issued, err := repo.Issue(ctx, first, consumedHash)
		require.NoError(t, err)
		require.NoError(t, repo.Consume(ctx, issued.ID(), time.Now()))

		second, err := domain.NewEmailVerification(userID, time.Hour)
		require.NoError(t, err)
		_, err = repo.Issue(ctx, second, dbtest.RandString())
		require.NoError(t, err)

		// Only unconsumed challenges are superseded: a consumed one is kept so a
		// reopened link can still report "already verified".
		got, err := repo.GetByTokenHash(ctx, consumedHash)
		require.NoError(t, err)
		require.True(t, got.IsConsumed())
	})
}

func TestEmailVerificationsGetUnknownToken(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		_, err := b.verifications(t).GetByTokenHash(context.Background(), dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid)
	})
}

// TestEmailVerificationsDeleteExpiredBefore is pg-only: DeleteExpiredBefore is
// not on the port. It backs the Postgres cleanup job, and YDB uses table TTL
// instead.
func TestEmailVerificationsDeleteExpiredBefore(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	users := adapters.NewUsers(pool)
	repo := adapters.NewEmailVerifications(pool)
	now := time.Now()

	// A token still within its validity window survives the sweep.
	liveUser, _ := newUser(t, "Ada")
	createdLive, err := users.Create(ctx, liveUser, "hash")
	require.NoError(t, err)
	live, err := domain.NewEmailVerification(createdLive.ID(), time.Hour)
	require.NoError(t, err)
	_, err = repo.Issue(ctx, live, "delete-test-live-hash")
	require.NoError(t, err)

	// A long-expired token is swept.
	expiredUser, _ := newUser(t, "Grace")
	createdExpired, err := users.Create(ctx, expiredUser, "hash")
	require.NoError(t, err)
	expired := domain.HydrateEmailVerification("", createdExpired.ID(),
		now.Add(-time.Hour), now.Add(-2*time.Hour), nil)
	_, err = repo.Issue(ctx, expired, "delete-test-expired-hash")
	require.NoError(t, err)

	// A cutoff in the recent past sweeps only the long-expired token; other
	// tests' tokens all expire in the future, so they are untouched.
	_, err = repo.DeleteExpiredBefore(ctx, now.Add(-30*time.Minute))
	require.NoError(t, err)

	_, err = repo.GetByTokenHash(ctx, "delete-test-live-hash")
	require.NoError(t, err, "the live token survives the sweep")
	_, err = repo.GetByTokenHash(ctx, "delete-test-expired-hash")
	require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid, "the expired token is gone")
}
```

- [ ] **Step 5: Run the suites to verify they fail**

Run: `go vet ./internal/auth/adapters/`

Expected: build failure `undefined: adapters.NewYDBSessions` and `undefined: adapters.NewYDBEmailVerifications`.

- [ ] **Step 6: Implement `YDBSessions`**

Create `internal/auth/adapters/sessions_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

// YDBSessions is the YDB-backed implementation of domain.SessionRepository,
// for the serverless runtime. Only the hash of a session token is ever stored.
// The hash is the table's primary key, because every lookup is by hash.
type YDBSessions struct {
	driver *ydb.Driver
}

var _ domain.SessionRepository = (*YDBSessions)(nil)

// NewYDBSessions builds a YDBSessions repository over the driver.
func NewYDBSessions(d *ydb.Driver) *YDBSessions {
	return &YDBSessions{driver: d}
}

// Issue persists a new session, storing only the token's hash.
func (r *YDBSessions) Issue(ctx context.Context, s *domain.Session, tokenHash string) error {
	return insertYDBSession(ctx, r.driver.Query(), s, tokenHash)
}

// insertYDBSession persists a session through q, which may be the query client
// or a transaction. A transaction lets YDBUsers issue a session atomically
// with the user it belongs to.
func insertYDBSession(ctx context.Context, q query.Executor, s *domain.Session, tokenHash string) error {
	err := q.Exec(ctx, `DECLARE $token_hash AS Utf8;
DECLARE $id AS Utf8;
DECLARE $platform_user_id AS Utf8;
DECLARE $expires_at AS Timestamp;
INSERT INTO platform_sessions (token_hash, id, platform_user_id, created_at, expires_at)
VALUES ($token_hash, $id, $platform_user_id, CurrentUtcTimestamp(), $expires_at);`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$token_hash").Text(tokenHash).
			Param("$id").Text(uuid.NewString()).
			Param("$platform_user_id").Text(s.UserID()).
			Param("$expires_at").Timestamp(s.ExpiresAt()).
			Build()))
	if err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}
	return nil
}

// ResolveByTokenHash returns the live session for a token hash, or
// domain.ErrSessionInvalid when the token is unknown, expired, or revoked.
func (r *YDBSessions) ResolveByTokenHash(ctx context.Context, tokenHash string) (*domain.Session, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $token_hash AS Utf8;
SELECT id, platform_user_id, expires_at, revoked_at
FROM platform_sessions
WHERE token_hash = $token_hash AND revoked_at IS NULL AND expires_at > CurrentUtcTimestamp();`,
		ydbTokenHash(tokenHash))
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrSessionInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("resolving session: %w", err)
	}
	var id, userID string
	var expiresAt time.Time
	var revokedAt *time.Time
	if err := row.Scan(&id, &userID, &expiresAt, &revokedAt); err != nil {
		return nil, fmt.Errorf("scanning session: %w", err)
	}
	return domain.HydrateSession(id, userID, expiresAt, revokedAt), nil
}

// RevokeByTokenHash revokes the session for a token hash. Revoking an unknown
// or already-revoked token is a no-op.
func (r *YDBSessions) RevokeByTokenHash(ctx context.Context, tokenHash string) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $token_hash AS Utf8;
UPDATE platform_sessions SET revoked_at = CurrentUtcTimestamp()
WHERE token_hash = $token_hash AND revoked_at IS NULL;`, ydbTokenHash(tokenHash)); err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	return nil
}

// ydbTokenHash binds the $token_hash parameter.
func ydbTokenHash(tokenHash string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$token_hash").Text(tokenHash).Build())
}
```

- [ ] **Step 7: Implement `YDBEmailVerifications`**

Create `internal/auth/adapters/email_verifications_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

// YDBEmailVerifications is the YDB-backed implementation of
// domain.EmailVerificationRepository, for the serverless runtime. Only the
// hash of a verification token is ever stored. Table TTL deletes a challenge
// once it expires, so there is no DeleteExpiredBefore and no cleanup job.
type YDBEmailVerifications struct {
	driver *ydb.Driver
}

var _ domain.EmailVerificationRepository = (*YDBEmailVerifications)(nil)

// NewYDBEmailVerifications builds a YDBEmailVerifications repository over the
// driver.
func NewYDBEmailVerifications(d *ydb.Driver) *YDBEmailVerifications {
	return &YDBEmailVerifications{driver: d}
}

// Issue persists a new verification challenge. It first deletes any
// unconsumed challenge for the same user, so a freshly issued link supersedes
// earlier ones. The SDK may re-run the transaction, so the result is assigned
// only at the end of the closure.
func (r *YDBEmailVerifications) Issue(ctx context.Context, v *domain.EmailVerification,
	tokenHash string) (*domain.EmailVerification, error) {

	var issued *domain.EmailVerification
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := tx.Exec(ctx, `DECLARE $user_id AS Utf8;
DELETE FROM email_verification_tokens ON
SELECT id FROM email_verification_tokens VIEW idx_email_verification_tokens_user
WHERE user_id = $user_id AND consumed_at IS NULL;`,
			query.WithParameters(ydb.ParamsBuilder().Param("$user_id").Text(v.UserID()).Build())); err != nil {
			return fmt.Errorf("clearing pending verifications: %w", err)
		}
		got, err := insertYDBVerificationToken(ctx, tx, v, tokenHash)
		if err != nil {
			return err
		}
		issued = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return issued, nil
}

// insertYDBVerificationToken inserts one challenge row through q, which may be
// the query client or a transaction. YDBUsers shares it to issue the first
// challenge atomically with the user it belongs to.
func insertYDBVerificationToken(ctx context.Context, q query.Executor, v *domain.EmailVerification,
	tokenHash string) (*domain.EmailVerification, error) {

	id := uuid.NewString()
	row, err := q.QueryRow(ctx, `DECLARE $id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $token_hash AS Utf8;
DECLARE $expires_at AS Timestamp;
INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at, created_at)
VALUES ($id, $user_id, $token_hash, $expires_at, CurrentUtcTimestamp())
RETURNING created_at;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(id).
			Param("$user_id").Text(v.UserID()).
			Param("$token_hash").Text(tokenHash).
			Param("$expires_at").Timestamp(v.ExpiresAt()).
			Build()))
	if err != nil {
		return nil, fmt.Errorf("inserting verification token: %w", err)
	}
	var createdAt time.Time
	if err := row.Scan(&createdAt); err != nil {
		return nil, fmt.Errorf("scanning verification token: %w", err)
	}
	return domain.HydrateEmailVerification(id, v.UserID(), v.ExpiresAt(), createdAt, nil), nil
}

// GetByTokenHash returns the verification for a token hash, or
// domain.ErrVerificationLinkInvalid when the token is unknown.
func (r *YDBEmailVerifications) GetByTokenHash(ctx context.Context, tokenHash string) (
	*domain.EmailVerification, error) {

	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $token_hash AS Utf8;
SELECT id, user_id, expires_at, created_at, consumed_at
FROM email_verification_tokens VIEW idx_email_verification_tokens_token_hash
WHERE token_hash = $token_hash;`, ydbTokenHash(tokenHash))
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrVerificationLinkInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("loading verification token: %w", err)
	}
	var id, userID string
	var expiresAt, createdAt time.Time
	var consumedAt *time.Time
	if err := row.Scan(&id, &userID, &expiresAt, &createdAt, &consumedAt); err != nil {
		return nil, fmt.Errorf("scanning verification token: %w", err)
	}
	return domain.HydrateEmailVerification(id, userID, expiresAt, createdAt, consumedAt), nil
}

// Consume marks the verification challenge used at now. Consuming an
// already-consumed challenge is a no-op.
func (r *YDBEmailVerifications) Consume(ctx context.Context, verificationID string, now time.Time) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $consumed_at AS Timestamp;
UPDATE email_verification_tokens SET consumed_at = $consumed_at
WHERE id = $id AND consumed_at IS NULL;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(verificationID).
			Param("$consumed_at").Timestamp(now).
			Build())); err != nil {
		return fmt.Errorf("consuming verification token: %w", err)
	}
	return nil
}
```

- [ ] **Step 8: Run the suites to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/auth/adapters/ -run 'TestSessions|TestEmailVerifications' -v -timeout 8m 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)' | grep -E 'FAIL|/pg|/ydb|^ok|DeleteExpired'`

Expected:
- Every `TestSessions*` and every eachBackend `TestEmailVerifications*` shows `--- PASS` for both `/pg` and `/ydb`.
- `TestEmailVerificationsDeleteExpiredBefore` passes, pg only.
- The package is `ok`.

If a `/ydb` subtest fails, fix only the YDB adapter or the migration, and record the verbatim error and the fix in the report. Two failures are not yours to fix: report BLOCKED with the verbatim output.
- a `/pg` subtest fails: the contract changed behaviour for SaaS;
- the migration is rejected by YDB.

- [ ] **Step 9: Full verification and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output besides the build. In other words, no failing package.

Run: `golangci-lint run && export PATH="$HOME/go/bin:$PATH" && make lint-arch`

Expected: `0 issues.` and `[arch] clean`.

Run: `git diff --stat HEAD -- go.mod go.sum`

Expected: no output.

```bash
gofmt -l internal/auth/
git add internal/db/ydbmigrations/000003_auth.up.sql internal/auth/adapters/sessions_ydb.go \
  internal/auth/adapters/email_verifications_ydb.go internal/auth/adapters/helpers_test.go \
  internal/auth/adapters/sessions_test.go internal/auth/adapters/email_verifications_test.go
git commit -m "feat(auth): add the YDB session and email-verification repositories under pg+ydb contract suites"
```

`gofmt -l` must print nothing. `git status` must show the two `*_pg_test.go` files as renamed, not deleted plus untracked.

---

### Task 2: `YDBUsers` and the users contract suite

**Files:**
- Create: `internal/auth/adapters/users_ydb.go`
- Rename: `internal/auth/adapters/users_pg_test.go` → `internal/auth/adapters/users_test.go` (rewritten)
- Modify: `internal/auth/adapters/helpers_test.go` (add the `users` backend field)

**Interfaces:**
- Consumes (Task 1):
  - `insertYDBSession(ctx, q query.Executor, s *domain.Session, tokenHash string) error`
  - `insertYDBVerificationToken(ctx, q query.Executor, v *domain.EmailVerification, tokenHash string) (*domain.EmailVerification, error)`
  - the `platform_users` table and its `idx_platform_users_email_key` index
  - the harness: `backend`, `backends`, `eachBackend` and `persistUser`
- Consumes (B1): `db.IsYDBConflict(err error) bool` (`internal/db/ydb.go`).
- Consumes the port `domain.UserRepository` (`internal/auth/domain/repository.go`), unchanged.
- Produces:
  ```go
  // package adapters (internal/auth/adapters)
  type YDBUsers struct{ /* unexported */ }
  func NewYDBUsers(d *ydb.Driver) *YDBUsers
  // package adapters_test: backend gains
  users func(t *testing.T) domain.UserRepository
  ```
- `newUser(t, name) (*domain.User, string)` keeps its signature, because `helpers_test.go` and `email_verifications_test.go` use it.

- [ ] **Step 1: Add the users backend field**

In `internal/auth/adapters/helpers_test.go`, add the field to `backend` as its first function field:

```go
	name          string
	users         func(t *testing.T) domain.UserRepository
	sessions      func(t *testing.T) domain.SessionRepository
```

Add it to the `pg` entry, right after `name: "pg",`:

```go
		users: func(t *testing.T) domain.UserRepository {
			return adapters.NewUsers(dbtest.AppPool(t))
		},
```

Add it to the `ydb` entry, right after `name: "ydb",`:

```go
		users: func(t *testing.T) domain.UserRepository {
			return adapters.NewYDBUsers(dbtest.YDBMigrated(t))
		},
```

- [ ] **Step 2: Rewrite the users tests as a contract suite**

```bash
git mv internal/auth/adapters/users_pg_test.go internal/auth/adapters/users_test.go
```

Replace the contents of `internal/auth/adapters/users_test.go` with the code below.
- `newUser` keeps its signature.
- Three tests are new: `EmailMatchesCaseInsensitively`, `CreateWithSession` and `CreateInTransactionRejectsDuplicateEmail`. The first pins the `citext` behaviour the YDB `email_key` must reproduce. The other two cover the transactional create paths, which had no pg test.

```go
package adapters_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func newUser(t *testing.T, name string) (*domain.User, string) {
	t.Helper()
	addr := dbtest.RandString() + "@example.com"
	email, err := domain.NewEmail(addr)
	require.NoError(t, err)
	u, err := domain.NewUser(email, name)
	require.NoError(t, err)
	return u, addr
}

func TestUsersCreateGetLookup(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, addr := newUser(t, "Ada Lovelace")
		created, err := repo.Create(ctx, u, "hashed-password")
		require.NoError(t, err)
		require.NotEmpty(t, created.ID(), "the adapter assigns an id")
		require.Equal(t, addr, created.Email().String())
		require.True(t, created.Locale().IsZero(), "a new user has no locale")
		require.False(t, created.IsEmailVerified(), "a new user is unverified")

		byID, err := repo.GetByID(ctx, created.ID())
		require.NoError(t, err)
		require.Equal(t, created.ID(), byID.ID())
		require.Equal(t, addr, byID.Email().String())
		require.Equal(t, "Ada Lovelace", byID.Name())
		require.True(t, byID.Locale().IsZero())
		require.False(t, byID.IsEmailVerified())

		byEmail, err := repo.LookupByEmail(ctx, addr)
		require.NoError(t, err)
		require.Equal(t, created.ID(), byEmail.ID())
	})
}

func TestUsersCreateRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, addr := newUser(t, "Ada")
		_, err := repo.Create(ctx, u, "hash")
		require.NoError(t, err)

		email, err := domain.NewEmail(addr)
		require.NoError(t, err)
		dup, err := domain.NewUser(email, "Imposter")
		require.NoError(t, err)

		_, err = repo.Create(ctx, dup, "hash")
		require.ErrorIs(t, err, domain.ErrEmailTaken)
	})
}

// TestUsersEmailMatchesCaseInsensitively pins what the citext column gives
// Postgres. Lookups and uniqueness ignore case, and the address is returned
// as it was entered.
func TestUsersEmailMatchesCaseInsensitively(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		mixed := "Ada." + dbtest.RandString() + "@Example.COM"
		email, err := domain.NewEmail(mixed)
		require.NoError(t, err)
		u, err := domain.NewUser(email, "Ada")
		require.NoError(t, err)
		created, err := repo.Create(ctx, u, "the-hash")
		require.NoError(t, err)

		byEmail, err := repo.LookupByEmail(ctx, strings.ToLower(mixed))
		require.NoError(t, err)
		require.Equal(t, created.ID(), byEmail.ID())
		require.Equal(t, mixed, byEmail.Email().String(), "the address is stored as entered")

		_, hash, err := repo.GetCredentials(ctx, strings.ToUpper(mixed))
		require.NoError(t, err)
		require.Equal(t, "the-hash", hash)

		lowerEmail, err := domain.NewEmail(strings.ToLower(mixed))
		require.NoError(t, err)
		dup, err := domain.NewUser(lowerEmail, "Imposter")
		require.NoError(t, err)
		_, err = repo.Create(ctx, dup, "hash")
		require.ErrorIs(t, err, domain.ErrEmailTaken, "emails differing only in case collide")
	})
}

func TestUsersGetCredentials(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, addr := newUser(t, "Grace")
		_, err := repo.Create(ctx, u, "the-stored-hash")
		require.NoError(t, err)

		got, hash, err := repo.GetCredentials(ctx, addr)
		require.NoError(t, err)
		require.Equal(t, addr, got.Email().String())
		require.Equal(t, "the-stored-hash", hash)
	})
}

func TestUsersNotFound(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		_, err := repo.LookupByEmail(ctx, dbtest.RandString()+"@example.com")
		require.ErrorIs(t, err, domain.ErrUserNotFound)

		_, _, err = repo.GetCredentials(ctx, dbtest.RandString()+"@example.com")
		require.ErrorIs(t, err, domain.ErrUserNotFound)

		_, err = repo.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, err, domain.ErrUserNotFound)
	})
}

func TestUsersUpdateLocale(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, _ := newUser(t, "Ada")
		created, err := repo.Create(ctx, u, "hash")
		require.NoError(t, err)

		ru, err := domain.NewLocale("ru")
		require.NoError(t, err)
		require.NoError(t, repo.UpdateLocale(ctx, created.ID(), ru))

		reloaded, err := repo.GetByID(ctx, created.ID())
		require.NoError(t, err)
		require.Equal(t, "ru", reloaded.Locale().String())

		// The preference can change between supported locales.
		en, err := domain.NewLocale("en")
		require.NoError(t, err)
		require.NoError(t, repo.UpdateLocale(ctx, created.ID(), en))
		reloaded, err = repo.GetByID(ctx, created.ID())
		require.NoError(t, err)
		require.Equal(t, "en", reloaded.Locale().String())
	})
}

func TestUsersUpdateLocaleUnknownUser(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ru, err := domain.NewLocale("ru")
		require.NoError(t, err)

		err = b.users(t).UpdateLocale(context.Background(), "00000000-0000-0000-0000-000000000000", ru)
		require.ErrorIs(t, err, domain.ErrUserNotFound)
	})
}

func TestUsersCreateWithSession(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tokenHash := dbtest.RandString()

		u, addr := newUser(t, "Ada")
		created, err := repo.CreateWithSession(ctx, u, "hashed-password",
			func(userID string) (*domain.Session, string, error) {
				s, err := domain.NewSession(userID, time.Hour)
				return s, tokenHash, err
			})
		require.NoError(t, err)
		require.NotEmpty(t, created.ID(), "the adapter assigns an id")
		require.Equal(t, addr, created.Email().String())

		// The session was inserted in the same transaction, for the new user.
		s, err := b.sessions(t).ResolveByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.Equal(t, created.ID(), s.UserID())
	})
}

func TestUsersCreateWithVerification(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, addr := newUser(t, "Ada")
		created, err := repo.CreateWithVerification(ctx, u, "hashed-password",
			func(userID string) (*domain.EmailVerification, string, error) {
				v, err := domain.NewEmailVerification(userID, time.Hour)
				return v, "token-hash-" + userID, err
			})
		require.NoError(t, err)
		require.NotEmpty(t, created.ID(), "the adapter assigns an id")
		require.Equal(t, addr, created.Email().String())
		require.False(t, created.IsEmailVerified(), "a freshly created account is unverified")

		// The first verification challenge was inserted in the same transaction.
		got, err := b.verifications(t).GetByTokenHash(ctx, "token-hash-"+created.ID())
		require.NoError(t, err)
		require.Equal(t, created.ID(), got.UserID())
	})
}

func TestUsersCreateWithVerificationRollsBackOnError(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, addr := newUser(t, "Ada")
		wantErr := errors.New("token generation failed")
		_, err := repo.CreateWithVerification(ctx, u, "hash",
			func(string) (*domain.EmailVerification, string, error) {
				return nil, "", wantErr
			})
		require.ErrorIs(t, err, wantErr)

		// The user insert was rolled back with the failed token issuance.
		_, err = repo.LookupByEmail(ctx, addr)
		require.ErrorIs(t, err, domain.ErrUserNotFound)
	})
}

// TestUsersCreateInTransactionRejectsDuplicateEmail covers the duplicate-email
// mapping inside the transactional create paths. On YDB the conflict surfaces
// at the INSERT statement inside DoTx.
func TestUsersCreateInTransactionRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, addr := newUser(t, "Ada")
		_, err := repo.Create(ctx, u, "hash")
		require.NoError(t, err)

		email, err := domain.NewEmail(addr)
		require.NoError(t, err)
		dup, err := domain.NewUser(email, "Imposter")
		require.NoError(t, err)

		_, err = repo.CreateWithSession(ctx, dup, "hash",
			func(userID string) (*domain.Session, string, error) {
				s, err := domain.NewSession(userID, time.Hour)
				return s, dbtest.RandString(), err
			})
		require.ErrorIs(t, err, domain.ErrEmailTaken)

		_, err = repo.CreateWithVerification(ctx, dup, "hash",
			func(userID string) (*domain.EmailVerification, string, error) {
				v, err := domain.NewEmailVerification(userID, time.Hour)
				return v, dbtest.RandString(), err
			})
		require.ErrorIs(t, err, domain.ErrEmailTaken)
	})
}

func TestUsersMarkEmailVerified(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, _ := newUser(t, "Ada")
		created, err := repo.Create(ctx, u, "hash")
		require.NoError(t, err)
		require.False(t, created.IsEmailVerified())

		verifiedAt := time.Now()
		require.NoError(t, repo.MarkEmailVerified(ctx, created.ID(), verifiedAt))

		reloaded, err := repo.GetByID(ctx, created.ID())
		require.NoError(t, err)
		require.True(t, reloaded.IsEmailVerified(), "the account is now verified")
		require.WithinDuration(t, verifiedAt, *reloaded.EmailVerifiedAt(), time.Second)

		// Marking again is an idempotent no-op — the original instant stands.
		require.NoError(t, repo.MarkEmailVerified(ctx, created.ID(), verifiedAt.Add(time.Hour)))
		again, err := repo.GetByID(ctx, created.ID())
		require.NoError(t, err)
		require.WithinDuration(t, *reloaded.EmailVerifiedAt(), *again.EmailVerifiedAt(), time.Second)
	})
}
```

- [ ] **Step 3: Run the suite to verify it fails**

Run: `go vet ./internal/auth/adapters/`

Expected: build failure `undefined: adapters.NewYDBUsers`.

- [ ] **Step 4: Implement `YDBUsers`**

Create `internal/auth/adapters/users_ydb.go`:

```go
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBUsers is the YDB-backed implementation of domain.UserRepository, for the
// serverless runtime. Emails match case-insensitively through the lower-cased
// email_key column, as the citext column does on Postgres.
type YDBUsers struct {
	driver *ydb.Driver
}

var _ domain.UserRepository = (*YDBUsers)(nil)

// NewYDBUsers builds a YDBUsers repository over the driver.
func NewYDBUsers(d *ydb.Driver) *YDBUsers {
	return &YDBUsers{driver: d}
}

// ydbUserColumns is the projection scanUser reads, in order.
const ydbUserColumns = `id, email, name, COALESCE(locale, ""u) AS locale, email_verified_at`

// Create inserts a new user with an already-hashed password and returns the
// persisted user. It returns domain.ErrEmailTaken when the email is already
// registered.
func (r *YDBUsers) Create(ctx context.Context, u *domain.User, passwordHash string) (*domain.User, error) {
	id := uuid.NewString()
	if err := insertYDBUser(ctx, r.driver.Query(), id, u, passwordHash); err != nil {
		return nil, err
	}
	return newlyCreatedUser(id, u), nil
}

// CreateWithSession atomically inserts a new user and issues an initial
// session. The SDK may re-run the transaction, so issueSession may run more
// than once. The id is fixed before the closure and the result is built after
// it, so nothing a retry repeats leaks out.
func (r *YDBUsers) CreateWithSession(ctx context.Context, u *domain.User, passwordHash string,
	issueSession func(userID string) (*domain.Session, string, error)) (*domain.User, error) {

	id := uuid.NewString()
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := insertYDBUser(ctx, tx, id, u, passwordHash); err != nil {
			return err
		}
		session, tokenHash, err := issueSession(id)
		if err != nil {
			return err
		}
		return insertYDBSession(ctx, tx, session, tokenHash)
	})
	if err != nil {
		return nil, err
	}
	return newlyCreatedUser(id, u), nil
}

// CreateWithVerification atomically inserts a new user and its first
// email-verification challenge. It has the same retry contract as
// CreateWithSession.
func (r *YDBUsers) CreateWithVerification(ctx context.Context, u *domain.User, passwordHash string,
	issueVerification func(userID string) (*domain.EmailVerification, string, error)) (*domain.User, error) {

	id := uuid.NewString()
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := insertYDBUser(ctx, tx, id, u, passwordHash); err != nil {
			return err
		}
		verification, tokenHash, err := issueVerification(id)
		if err != nil {
			return err
		}
		_, err = insertYDBVerificationToken(ctx, tx, verification, tokenHash)
		return err
	})
	if err != nil {
		return nil, err
	}
	return newlyCreatedUser(id, u), nil
}

// insertYDBUser inserts one platform_users row through q, which may be the
// query client or a transaction. A conflict on the unique email_key index maps
// to domain.ErrEmailTaken. YDB reports a primary-key collision the same way,
// which a random UUID makes negligible.
func insertYDBUser(ctx context.Context, q query.Executor, id string, u *domain.User, passwordHash string) error {
	err := q.Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $email_key AS Utf8;
DECLARE $password_hash AS Utf8;
DECLARE $name AS Utf8;
INSERT INTO platform_users (id, email, email_key, password_hash, name, created_at, updated_at)
VALUES ($id, $email, $email_key, $password_hash, $name, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(id).
			Param("$email").Text(u.Email().String()).
			Param("$email_key").Text(ydbEmailKey(u.Email().String())).
			Param("$password_hash").Text(passwordHash).
			Param("$name").Text(u.Name()).
			Build()))
	if db.IsYDBConflict(err) {
		return domain.ErrEmailTaken
	}
	if err != nil {
		return fmt.Errorf("inserting platform user: %w", err)
	}
	return nil
}

// newlyCreatedUser is the persisted form of a just-inserted user, which has no
// locale yet and is unverified, as on Postgres.
func newlyCreatedUser(id string, u *domain.User) *domain.User {
	return domain.HydrateUser(id, u.Email().String(), u.Name(), "", nil)
}

// MarkEmailVerified records that the user verified their email address at now.
// It is idempotent: the UPDATE only touches a still-unverified row, so a repeat
// call leaves the original instant untouched.
func (r *YDBUsers) MarkEmailVerified(ctx context.Context, userID string, now time.Time) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $email_verified_at AS Timestamp;
UPDATE platform_users SET email_verified_at = $email_verified_at, updated_at = CurrentUtcTimestamp()
WHERE id = $id AND email_verified_at IS NULL;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(userID).
			Param("$email_verified_at").Timestamp(now).
			Build())); err != nil {
		return fmt.Errorf("marking email verified: %w", err)
	}
	return nil
}

// GetByID returns the user with the given id, or domain.ErrUserNotFound.
func (r *YDBUsers) GetByID(ctx context.Context, id string) (*domain.User, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $id AS Utf8;
SELECT `+ydbUserColumns+`
FROM platform_users
WHERE id = $id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$id").Text(id).Build()))
	return scanYDBUser(row, err, "loading platform user")
}

// UpdateLocale persists the user's interface-language preference. It returns
// domain.ErrUserNotFound when no user has the given id: UPDATE ... RETURNING
// yields no row for a missing id.
func (r *YDBUsers) UpdateLocale(ctx context.Context, userID string, locale domain.Locale) error {
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $id AS Utf8;
DECLARE $locale AS Utf8;
UPDATE platform_users SET locale = $locale, updated_at = CurrentUtcTimestamp()
WHERE id = $id
RETURNING id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(userID).
			Param("$locale").Text(locale.String()).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("updating user locale: %w", err)
	}
	return nil
}

// LookupByEmail returns the user with the given email, matched ignoring case,
// or domain.ErrUserNotFound.
func (r *YDBUsers) LookupByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $email_key AS Utf8;
SELECT `+ydbUserColumns+`
FROM platform_users VIEW idx_platform_users_email_key
WHERE email_key = $email_key;`, ydbEmailKeyParam(email))
	return scanYDBUser(row, err, "looking up user")
}

// GetCredentials returns the user and the stored bcrypt hash for an email,
// matched ignoring case, or domain.ErrUserNotFound. The hash never leaves the
// adapter/app boundary.
func (r *YDBUsers) GetCredentials(ctx context.Context, email string) (*domain.User, string, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $email_key AS Utf8;
SELECT `+ydbUserColumns+`, password_hash
FROM platform_users VIEW idx_platform_users_email_key
WHERE email_key = $email_key;`, ydbEmailKeyParam(email))
	if errors.Is(err, query.ErrNoRows) {
		return nil, "", domain.ErrUserNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("loading credentials: %w", err)
	}
	var id, gotEmail, name, locale, hash string
	var emailVerifiedAt *time.Time
	if err := row.Scan(&id, &gotEmail, &name, &locale, &emailVerifiedAt, &hash); err != nil {
		return nil, "", fmt.Errorf("scanning credentials: %w", err)
	}
	return domain.HydrateUser(id, gotEmail, name, locale, emailVerifiedAt), hash, nil
}

// ydbEmailKey is the case-insensitive lookup key for an email address.
func ydbEmailKey(email string) string { return strings.ToLower(email) }

// ydbEmailKeyParam binds $email_key for an email lookup.
func ydbEmailKeyParam(email string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$email_key").Text(ydbEmailKey(email)).Build())
}

// scanYDBUser maps a single-row read in ydbUserColumns order to the user,
// translating "no row" into domain.ErrUserNotFound.
func scanYDBUser(row query.Row, err error, what string) (*domain.User, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	var id, email, name, locale string
	var emailVerifiedAt *time.Time
	if err := row.Scan(&id, &email, &name, &locale, &emailVerifiedAt); err != nil {
		return nil, fmt.Errorf("%s: scanning: %w", what, err)
	}
	return domain.HydrateUser(id, email, name, locale, emailVerifiedAt), nil
}
```

- [ ] **Step 5: Run the suite to verify it passes**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/auth/... -v -timeout 8m 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)' | grep -E 'FAIL|/pg|/ydb|^ok'`

Expected: every `TestUsers*`, `TestSessions*` and eachBackend `TestEmailVerifications*` shows `--- PASS` for both `/pg` and `/ydb`. Every `internal/auth/...` package is `ok`.

Two cases are not yours to fix. Report BLOCKED with the verbatim output:
- **A `/pg` subtest fails.** In particular, `TestUsersEmailMatchesCaseInsensitively/pg` failing means Postgres does not behave as the plan assumes, so the contract itself is wrong.
- **`TestUsersCreateInTransactionRejectsDuplicateEmail/ydb` gets an unmapped error.** Print it with `%+v` in the report. It contradicts the spec's "Plan B2 planning spike" finding.

Any other `/ydb` failure: fix only `users_ydb.go`, and record the verbatim error and the fix in the report.

- [ ] **Step 6: Full verification and commit**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go build ./... && go vet ./... && go test ./... -timeout 15m 2>&1 | grep -vE '^(ok|\?)\s'`

Expected: no output besides the build. In other words, no failing package.

Run: `golangci-lint run && export PATH="$HOME/go/bin:$PATH" && make lint-arch`

Expected: `0 issues.` and `[arch] clean`.

Run: `git diff --stat HEAD -- go.mod go.sum`

Expected: no output.

```bash
gofmt -l internal/auth/
git add internal/auth/adapters/users_ydb.go internal/auth/adapters/users_test.go \
  internal/auth/adapters/helpers_test.go
git commit -m "feat(auth): add the YDB user repository under a pg+ydb contract suite"
```

`gofmt -l` must print nothing, and `git status` must show `users_pg_test.go` → `users_test.go` as a rename.

---

## Done criteria for Plan B2

- `go build ./... && go vet ./... && go test ./...` passes. Every `TestUsers*`, `TestSessions*` and eachBackend `TestEmailVerifications*` passes on both `/pg` and `/ydb`.
- `golangci-lint run` reports `0 issues.`, and `make lint-arch` reports `[arch] clean`.
- No `go.mod` or `go.sum` change.
- `TestUsersEmailMatchesCaseInsensitively` passes on both backends: YDB reproduces `citext` matching through `email_key`.
- `TestUsersCreateInTransactionRejectsDuplicateEmail/ydb` passes: the in-`DoTx` conflict mapping holds against real YDB.
- The final report lists what stays open for the next plans:
  - Plan B3 (tenant) joins `platform_users` from `Tenants.ListMembers`. Its test helper seeds YDB users through `adapters.NewYDBUsers` from this plan.
  - The INSERT-conflict "continue the transaction" behaviour for `BulkInsert` is still open (campaign plan).

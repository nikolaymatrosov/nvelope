# Serverless On-Prem — Plan C2 (single-tenant bootstrap) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- A single-tenant install (`NVELOPE_RUNTIME=serverless`) has no open registration. The first account comes from a setup-token signup, which creates the account (already verified, signed in) and the install's one workspace. Everyone else joins by invitation.
- A public `GET /api/platform/config` tells the SPA which runtime it talks to. The SPA hides billing and workspace switching, and shows the setup-token field while setup is open.

**Architecture:**
- **Config.** `NVELOPE_RUNTIME` (`saas` default, `serverless`), `NVELOPE_TENANT_SLUG` and `NVELOPE_SETUP_TOKEN`, with `Config.SingleTenant()`.
- **auth `SetUp` command.** It checks the token in constant time and creates the account through the existing `UserRepository.CreateWithSession`, which no production code calls today. `CreateWithSession` now persists the user's verification state, on pg and YDB.
- **"Setup is open"** means the workspace at `NVELOPE_TENANT_SLUG` does not exist. It is read through the existing tenant `LocateWorkspace` query.
- **API.** `handleSignup` branches to a setup handler in single-tenant mode. `POST /api/platform/tenants` pins the slug to the configured one, so it answers `409 slug_taken` once the workspace exists. `GET /api/platform/config` is new.
- **Frontend.** One `usePlatformConfig` hook, read by the sidebar, app shell, top bar, landing, login and signup.
- **Unchanged.** SaaS behaviour, River, the pg/YDB repositories other than `CreateWithSession`, and the generated OpenAPI mounts.

**Tech Stack:** Go 1.26, chi, pgx v5, `ydb-go-sdk/v3` v3.136.4 (local-ydb `26.3.1.23`), testify, testcontainers-go; React 19 + TanStack Router/Query/Form, i18next, vitest.

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md). The binding sections are "On-prem packaging → Single tenant and first run", "UI feature flags" and "Testing → Bootstrap". Task 5 records this plan's decisions there.

**Scope:** The second of four Plan C plans. It runs in parallel with C3 (Terraform, images). C3 sets the three env vars this plan adds, and the C3 planner has been told their names. Out of scope:
- the serverless composition root and the relaxed serverless config validation (Redis, Postbox and stream keys stay required here; C4 relaxes them);
- the YDB wiring of `SetUp` (C4);
- a platform-plane OpenAPI document (see Decisions).

## Decisions (recorded in the spec by Task 5)

- **"Setup is open" = the configured workspace does not exist,** not "no user exists" (the spec's wording). Reasons:
  - It reuses `LocateWorkspace`; "no user exists" would need a new `UserRepository` method on two backends.
  - It can be tested on the shared test databases, where some user always exists. Each test gets its own slug.
  - It is the state that matters. The cron sweeps (C1) do nothing until the workspace exists, and invitations need it.
- **Partial failure.** If `SetUp` succeeds but `CreateWorkspace` fails, the owner is already signed in and verified. The landing page shows "Create workspace" for a user with no workspace, and `POST /api/platform/tenants` pins the slug, so the owner finishes setup from there. No cross-context transaction is needed.
- **`GET /api/platform/config` is hand-written, not in an OpenAPI document.** Only the tenant plane is spec-first today (`api/openapi/tenant.yaml`, audience only). Every `/api/platform/*` route is hand-written, and the spec-first design migrates the platform plane last. Building a `platform.yaml` and its codegen pipeline for one GET would be scaffolding. The route joins `platform.yaml` when the platform plane migrates.
- **JSON keys are snake_case:** `{"features":{"billing":bool,"multi_tenant":bool},"setup_required":bool}`. Platform-plane responses are snake_case (spec-first design). The spec's camelCase sketch is corrected.
- **The setup token is at least 32 characters** (config validation). Signup is public and not throttled, so the token must resist online guessing. The comparison hashes both sides with SHA-256 first, so it is constant-time and does not leak the length.
- **The workspace name starts as the slug.** The setup form asks only for the token, as the spec says. The owner renames the workspace in Settings.
- **Signup links are hidden** on the landing and login pages once a single-tenant install is set up, because signup would only answer 403.

## Global Constraints

- **Branch.** Work in the worktree `/Users/nikthespirit/experiments/nvelope-wt/serverless-c2` on branch `feat/serverless-c2`, created from `main`. The main checkout and the `nvelope-wt/serverless-onprem`, `serverless-c1` and `serverless-c3` worktrees belong to other sessions; do not touch them.
- **Docker.** Docker on this machine is OrbStack. Prefix every `go test` / `make test` with `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && `.
- **Shared test containers.** `nvelope-test-pg` and `nvelope-test-ydb` are reused by name across sessions. Pg failures like `no migration found for version N` or `unexpected postmaster exit` followed by a burst of 0.00 s failures are environmental: re-run, and never `docker rm` a container. `TestPublicSubscriptionCrossTenantIsolation` is a known flake on a reused DB: re-run it.
- **YDB tests.** A cold local-ydb start takes about 45 s, so use `-timeout 8m` on runs that touch `internal/auth/adapters`.
- **Node.** `node`/`pnpm` need `export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH" && ` in every frontend command. The worktree has no `frontend/node_modules` yet: run `cd frontend && pnpm install --frozen-lockfile` once before the first frontend step.
- **No new dependencies,** Go or npm.
- **SaaS must not change.** With `NVELOPE_RUNTIME` unset, every existing test passes unchanged (one existing vitest file gains a mock entry, in Task 4).
- **Never log secrets.** The setup token and passwords are never logged or echoed in errors. The command decorators log only the command name and duration.
- **Commits.** Conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.** `goimports` is not installed, so use `gofmt -w`. `golangci-lint` is on PATH. `make lint-arch` needs `export PATH="$HOME/go/bin:$PATH" && `.

## File structure

| File | Responsibility |
|------|----------------|
| `internal/config/config.go` | `Runtime`, `TenantSlug`, `SetupToken`, `SingleTenant()`, validation |
| `internal/config/config_test.go` | runtime parsing and validation |
| `internal/auth/domain/errors.go` | `ErrSetupTokenInvalid` |
| `internal/auth/domain/repository.go` | `CreateWithSession` doc: persists the verification state |
| `internal/auth/adapters/users_pg.go`, `users_ydb.go` | `CreateWithSession` (pg) and `InsertYDBUser` write `email_verified_at` |
| `internal/auth/adapters/users_test.go` | contract test: verification state survives `CreateWithSession` |
| `internal/auth/app/command/set_up.go` | the `SetUp` command |
| `internal/auth/app/command/set_up_test.go`, `fakes_test.go` | `SetUp` unit tests; the fake keeps the verification state |
| `internal/auth/app/application.go`, `internal/service/application.go` | `Commands.SetUp` and its wiring |
| `internal/api/platform_handlers.go`, `server.go` | setup signup, pinned tenant slug, `GET /api/platform/config` |
| `internal/api/helpers_test.go`, `setup_handlers_test.go` | `newTestServerWith`; the bootstrap tests |
| `frontend/src/lib/api.ts`, `api-types.ts`, `query.ts` | `api.config`, `PlatformConfig`, the setup signup shape, the query key |
| `frontend/src/hooks/use-platform-config.ts` | the flags, with SaaS defaults |
| `frontend/src/components/shell/{sidebar,app-shell,top-bar}.tsx` | hide billing and workspace switching |
| `frontend/src/routes/{index,login,signup}.tsx` | hide signup links; the setup-token field |
| `frontend/src/locales/{en,ru}/auth.json`, `frontend/src/i18n/resources.d.ts` | setup strings; regenerated types |
| `frontend/src/routes/signup.test.tsx`, `frontend/src/components/shell/app-shell.test.tsx` | setup-mode and billing-flag tests |

---

### Task 1: Runtime, tenant slug and setup token config

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.RuntimeSaaS = "saas"`, `config.RuntimeServerless = "serverless"`; fields `Config.Runtime`, `Config.TenantSlug`, `Config.SetupToken string`; method `func (c Config) SingleTenant() bool`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/config_test.go`:

```go
// testSetupToken is 32 characters, the minimum NVELOPE_SETUP_TOKEN length.
const testSetupToken = "0123456789abcdef0123456789abcdef"

func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_TOTP_ENCRYPTION_KEY", testTOTPKey)
	setPhase3Env(t)
}

func TestLoadRuntimeDefaultsToSaaS(t *testing.T) {
	setBaseEnv(t)

	cfg, err := Load("")
	require.NoError(t, err)
	require.False(t, cfg.SingleTenant(), "an unset NVELOPE_RUNTIME is SaaS")
}

func TestLoadServerlessRuntime(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("NVELOPE_RUNTIME", "serverless")
	t.Setenv("NVELOPE_TENANT_SLUG", "acme")
	t.Setenv("NVELOPE_SETUP_TOKEN", testSetupToken)

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, RuntimeServerless, cfg.Runtime)
	require.True(t, cfg.SingleTenant())
	require.Equal(t, "acme", cfg.TenantSlug)
	require.Equal(t, testSetupToken, cfg.SetupToken)
}

func TestLoadServerlessRequiresSlugAndToken(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("NVELOPE_RUNTIME", "serverless")
	t.Setenv("NVELOPE_SETUP_TOKEN", testSetupToken[:31])

	_, err := Load("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "NVELOPE_TENANT_SLUG")
	require.Contains(t, err.Error(), "NVELOPE_SETUP_TOKEN")
	require.NotContains(t, err.Error(), testSetupToken[:31], "the token value is never echoed")
}

func TestLoadRejectsUnknownRuntime(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("NVELOPE_RUNTIME", "lambda")

	_, err := Load("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "NVELOPE_RUNTIME")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ -run 'Runtime|Serverless' -v`
Expected: FAIL to compile (`cfg.SingleTenant undefined`, `RuntimeServerless undefined`).

- [ ] **Step 3: Implement**

In `internal/config/config.go`, after `const envPrefix = "NVELOPE_"`:

```go
// The deployments NVELOPE_RUNTIME selects. An unset value is RuntimeSaaS.
// RuntimeServerless is the on-prem install on Yandex Cloud serverless, which
// is always single-tenant.
const (
	RuntimeSaaS       = "saas"
	RuntimeServerless = "serverless"
)

// minSetupTokenLen bounds NVELOPE_SETUP_TOKEN from below. Signup is public and
// not throttled, so the token must resist online guessing.
const minSetupTokenLen = 32
```

At the end of the `Config` struct:

```go
	// Runtime selects the deployment: RuntimeSaaS (also the empty value) or
	// RuntimeServerless.
	Runtime string
	// TenantSlug is the slug of a single-tenant install's one workspace,
	// created at first-run setup. Required when Runtime is RuntimeServerless.
	TenantSlug string
	// SetupToken authorises the first-run signup of a single-tenant install.
	// Required (at least 32 characters) when Runtime is RuntimeServerless.
	// Secret — never log this value.
	SetupToken string
```

In `Load`, inside the `cfg := Config{...}` literal, after `VerificationResendLimit`:

```go
		Runtime:    k.String(envPrefix + "RUNTIME"),
		TenantSlug: k.String(envPrefix + "TENANT_SLUG"),
		SetupToken: k.String(envPrefix + "SETUP_TOKEN"),
```

In `Validate`, before the final `errors.Join` / return:

```go
	switch c.Runtime {
	case "", RuntimeSaaS:
	case RuntimeServerless:
		if c.TenantSlug == "" {
			errs = append(errs, errors.New("NVELOPE_TENANT_SLUG is required when NVELOPE_RUNTIME is serverless"))
		}
		if len(c.SetupToken) < minSetupTokenLen {
			errs = append(errs, fmt.Errorf(
				"NVELOPE_SETUP_TOKEN must be at least %d characters when NVELOPE_RUNTIME is serverless",
				minSetupTokenLen))
		}
	default:
		errs = append(errs, fmt.Errorf("NVELOPE_RUNTIME %q is invalid (want saas or serverless)", c.Runtime))
	}
```

After `Validate`:

```go
// SingleTenant reports whether this is a single-tenant install: one workspace,
// created at first-run setup, which everyone else joins by invitation.
func (c Config) SingleTenant() bool { return c.Runtime == RuntimeServerless }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all config tests, old and new).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/config
git add internal/config
git commit -m "feat(config): add the runtime, tenant slug and setup token settings"
```

---

### Task 2: The auth `SetUp` command

**Files:**
- Modify: `internal/auth/domain/errors.go`, `internal/auth/domain/repository.go`
- Modify: `internal/auth/adapters/users_pg.go` (`CreateWithSession`), `internal/auth/adapters/users_ydb.go` (`InsertYDBUser`, `newlyCreatedUser`)
- Create: `internal/auth/app/command/set_up.go`
- Modify: `internal/auth/app/application.go`, `internal/service/application.go`
- Test: `internal/auth/adapters/users_test.go`, `internal/auth/app/command/set_up_test.go`, `internal/auth/app/command/fakes_test.go`

**Interfaces:**
- Consumes: `config.Config.SetupToken` (Task 1); existing `domain.UserRepository.CreateWithSession`, `domain.NewSession(userID string, ttl time.Duration) (*Session, error)`, `(*domain.User).MarkEmailVerified(time.Time)`, `(*domain.User).EmailVerifiedAt() *time.Time`, `token.New() (string, error)`, `token.Hash(string) string`.
- Produces:
  - `authdomain.ErrSetupTokenInvalid`, a 403 with code `setup_token_invalid`;
  - `command.SetUp{Token, Email, Password, Name string}`;
  - `command.SetUpResult{UserID, UserEmail, UserName, SessionToken string}`;
  - `command.NewSetUpHandler(users domain.UserRepository, hasher PasswordHasher, setupToken string, sessionTTL time.Duration) SetUpHandler`;
  - `authapp.Commands.SetUp decorator.ResultCommandHandler[command.SetUp, command.SetUpResult]`.

- [ ] **Step 1: Write the failing contract test**

Append to `internal/auth/adapters/users_test.go`:

```go
// TestUsersCreateWithSessionKeepsVerification pins that CreateWithSession
// persists the user's verification state: first-run setup creates its owner
// account already verified, so a later login is not refused.
func TestUsersCreateWithSessionKeepsVerification(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, _ := newUser(t, "Owner")
		u.MarkEmailVerified(time.Now())
		created, err := repo.CreateWithSession(ctx, u, "hash",
			func(userID string) (*domain.Session, string, error) {
				s, err := domain.NewSession(userID, time.Hour)
				return s, dbtest.RandString(), err
			})
		require.NoError(t, err)
		require.True(t, created.IsEmailVerified())

		got, err := repo.GetByID(ctx, created.ID())
		require.NoError(t, err)
		require.True(t, got.IsEmailVerified(), "the verification state is persisted")
	})
}
```

And in the existing `TestUsersCreateWithSession`, after `require.Equal(t, addr, created.Email().String())`, add:

```go
		require.False(t, created.IsEmailVerified(), "an unverified user stays unverified")
```

- [ ] **Step 2: Run it to verify it fails**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/auth/adapters/ -run 'TestUsersCreateWithSession' -timeout 8m -v`
Expected: `TestUsersCreateWithSessionKeepsVerification/pg` and `/ydb` FAIL on `created.IsEmailVerified()`; `TestUsersCreateWithSession` passes.

- [ ] **Step 3: Persist the verification state**

In `internal/auth/adapters/users_pg.go` `CreateWithSession`, replace the insert and the hydrate:

```go
		var id, email, name string
		err := tx.QueryRow(ctx,
			`INSERT INTO platform_users (email, password_hash, name, email_verified_at)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id, email, name`,
			u.Email().String(), passwordHash, u.Name(), u.EmailVerifiedAt()).Scan(&id, &email, &name)
```

```go
		created = domain.HydrateUser(id, email, name, "", u.EmailVerifiedAt())
```

In `internal/auth/adapters/users_ydb.go`, `InsertYDBUser` becomes:

```go
func InsertYDBUser(ctx context.Context, q query.Executor, id string, u *domain.User, passwordHash string) error {
	err := q.Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $email_key AS Utf8;
DECLARE $password_hash AS Utf8;
DECLARE $name AS Utf8;
DECLARE $email_verified_at AS Optional<Timestamp>;
INSERT INTO platform_users (id, email, email_key, password_hash, name, email_verified_at, created_at, updated_at)
VALUES ($id, $email, $email_key, $password_hash, $name, $email_verified_at, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(id).
			Param("$email").Text(u.Email().String()).
			Param("$email_key").Text(ydbEmailKey(u.Email().String())).
			Param("$password_hash").Text(passwordHash).
			Param("$name").Text(u.Name()).
			Param("$email_verified_at").BeginOptional().Timestamp(u.EmailVerifiedAt()).EndOptional().
			Build()))
	if db.IsYDBConflict(err) {
		return domain.ErrEmailTaken
	}
	if err != nil {
		return fmt.Errorf("inserting platform user: %w", err)
	}
	return nil
}

// newlyCreatedUser is the persisted form of a just-inserted user: no locale
// yet, and the verification state the user was inserted with.
func newlyCreatedUser(id string, u *domain.User) *domain.User {
	return domain.HydrateUser(id, u.Email().String(), u.Name(), "", u.EmailVerifiedAt())
}
```

`InsertYDBUser` is also called by `Create`, `CreateWithVerification` and `internal/service/onboarding_ydb.go`. Every one of them passes an unverified user, so they still write NULL.

In `internal/auth/domain/repository.go`, add to the `CreateWithSession` doc comment, before "The whole operation is one transaction":

```go
	// The user's verification state is persisted as given, so a caller that
	// has already established email ownership can insert a verified account.
```

- [ ] **Step 4: Run the contract tests to verify they pass**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/auth/adapters/ ./internal/service/ -timeout 8m`
Expected: PASS (the onboarding suites in `internal/service` still pass).

- [ ] **Step 5: Write the failing command tests**

In `internal/auth/app/command/fakes_test.go`, `fakeUsers.Create` keeps the verification state. Replace its hydrate line:

```go
	stored := domain.HydrateUser(id, u.Email().String(), u.Name(), u.Locale().String(), u.EmailVerifiedAt())
```

Create `internal/auth/app/command/set_up_test.go`:

```go
package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

const setupToken = "0123456789abcdef0123456789abcdef"

func TestSetUpCreatesVerifiedSignedInOwner(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	h := command.NewSetUpHandler(users, stubHasher{}, setupToken, time.Hour)

	result, err := h.Handle(context.Background(), command.SetUp{
		Token: setupToken, Email: "owner@example.com", Password: "a-good-password", Name: "Owner",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.UserID)
	require.Equal(t, "owner@example.com", result.UserEmail)
	require.Equal(t, "Owner", result.UserName)
	require.NotEmpty(t, result.SessionToken, "setup signs the owner in")

	created, err := users.GetByID(context.Background(), result.UserID)
	require.NoError(t, err)
	require.True(t, created.IsEmailVerified(), "holding the setup token proves the right to install")
}

func TestSetUpRejectsWrongToken(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ configured, given string }{
		"missing":          {setupToken, ""},
		"wrong":            {setupToken, "not-the-token"},
		"longer":           {setupToken, setupToken + "x"},
		"setup not enabled": {"", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			users := newFakeUsers()
			h := command.NewSetUpHandler(users, stubHasher{}, tc.configured, time.Hour)

			_, err := h.Handle(context.Background(), command.SetUp{
				Token: tc.given, Email: "owner@example.com", Password: "a-good-password", Name: "Owner",
			})
			require.ErrorIs(t, err, domain.ErrSetupTokenInvalid)
			_, err = users.LookupByEmail(context.Background(), "owner@example.com")
			require.ErrorIs(t, err, domain.ErrUserNotFound, "a refused setup creates nothing")
		})
	}
}

func TestSetUpRejectsBadInput(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	h := command.NewSetUpHandler(users, stubHasher{}, setupToken, time.Hour)

	_, err := h.Handle(context.Background(), command.SetUp{
		Token: setupToken, Email: "owner@example.com", Password: "short", Name: "Owner",
	})
	require.Error(t, err)
	_, err = users.LookupByEmail(context.Background(), "owner@example.com")
	require.ErrorIs(t, err, domain.ErrUserNotFound)
}
```

- [ ] **Step 6: Run them to verify they fail**

Run: `go test ./internal/auth/app/command/ -run TestSetUp -v`
Expected: FAIL to compile (`command.NewSetUpHandler undefined`, `domain.ErrSetupTokenInvalid undefined`).

- [ ] **Step 7: Implement the command**

In `internal/auth/domain/errors.go`, inside the `var (...)` block after `ErrEmailNotVerified`:

```go
	// ErrSetupTokenInvalid is returned by first-run setup of a single-tenant
	// install when the setup token is missing or wrong, and on any deployment
	// without a setup token.
	ErrSetupTokenInvalid = apperr.NewForbidden(
		"setup_token_invalid", "the setup token is missing or wrong")
```

Create `internal/auth/app/command/set_up.go`:

```go
package command

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// SetUp is first-run setup of a single-tenant install: the account that will
// own the install's workspace, authorised by the setup token.
type SetUp struct {
	Token    string
	Email    string
	Password string
	Name     string
}

// SetUpResult carries the created account and the raw token of its session.
type SetUpResult struct {
	UserID       string
	UserEmail    string
	UserName     string
	SessionToken string
}

// SetUpHandler handles the SetUp command.
type SetUpHandler struct {
	users      domain.UserRepository
	hasher     PasswordHasher
	setupToken string
	sessionTTL time.Duration
}

// NewSetUpHandler builds a SetUpHandler, failing fast on nil dependencies. An
// empty setupToken (every SaaS deployment) refuses every request.
func NewSetUpHandler(users domain.UserRepository, hasher PasswordHasher,
	setupToken string, sessionTTL time.Duration) SetUpHandler {
	if users == nil {
		panic("nil users repository")
	}
	if hasher == nil {
		panic("nil password hasher")
	}
	return SetUpHandler{users: users, hasher: hasher, setupToken: setupToken, sessionTTL: sessionTTL}
}

// Handle checks the setup token, then creates the account already verified
// and signed in: nothing can send mail before the first login, and holding
// the token already proves the right to install. Whether setup is still open
// (the install's workspace does not exist yet) is the caller's check.
func (h SetUpHandler) Handle(ctx context.Context, cmd SetUp) (SetUpResult, error) {
	if !h.tokenMatches(cmd.Token) {
		return SetUpResult{}, domain.ErrSetupTokenInvalid
	}
	email, err := domain.NewEmail(cmd.Email)
	if err != nil {
		return SetUpResult{}, err
	}
	if _, err := domain.NewPassword(cmd.Password); err != nil {
		return SetUpResult{}, err
	}
	user, err := domain.NewUser(email, cmd.Name)
	if err != nil {
		return SetUpResult{}, err
	}
	user.MarkEmailVerified(time.Now())
	hash, err := h.hasher.Hash(cmd.Password)
	if err != nil {
		return SetUpResult{}, fmt.Errorf("hashing password: %w", err)
	}

	var rawToken string
	created, err := h.users.CreateWithSession(ctx, user, hash,
		func(userID string) (*domain.Session, string, error) {
			session, err := domain.NewSession(userID, h.sessionTTL)
			if err != nil {
				return nil, "", err
			}
			raw, err := token.New()
			if err != nil {
				return nil, "", err
			}
			rawToken = raw
			return session, token.Hash(raw), nil
		})
	if err != nil {
		return SetUpResult{}, err
	}
	return SetUpResult{
		UserID:       created.ID(),
		UserEmail:    created.Email().String(),
		UserName:     created.Name(),
		SessionToken: rawToken,
	}, nil
}

// tokenMatches compares the given token with the configured one in constant
// time. Hashing both first gives them equal length, so the comparison does
// not leak the token's length either.
func (h SetUpHandler) tokenMatches(given string) bool {
	if h.setupToken == "" {
		return false
	}
	want := sha256.Sum256([]byte(h.setupToken))
	got := sha256.Sum256([]byte(given))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}
```

In `internal/auth/app/application.go`, add to `Commands` after `SignUp`:

```go
	SetUp                   decorator.ResultCommandHandler[command.SetUp, command.SetUpResult]
```

In `internal/service/application.go`, in the `authapp.Commands{...}` literal after `SignUp`:

```go
			SetUp: decorator.ApplyResultCommandDecorators(
				authcommand.NewSetUpHandler(users, hasher, cfg.SetupToken, cfg.SessionTTL),
				"SetUp", logger),
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `gofmt -w internal/auth internal/service && go build ./... && go test ./internal/auth/app/command/ -v`
Expected: PASS. If `go build` reports another `authapp.Commands{...}` literal, give it the same `SetUp` field. Today `internal/service/application.go` is the only one.

- [ ] **Step 9: Commit**

```bash
git add internal/auth internal/service/application.go
git commit -m "feat(auth): add first-run setup that creates a verified, signed-in owner"
```

---

### Task 3: The setup signup, the pinned workspace slug and `GET /api/platform/config`

**Files:**
- Modify: `internal/api/platform_handlers.go`, `internal/api/server.go`
- Modify: `internal/api/helpers_test.go` (`newTestServerWith`)
- Create: `internal/api/setup_handlers_test.go`

**Interfaces:**
- Consumes:
  - `config.Config.SingleTenant()`, `.TenantSlug`, `config.RuntimeServerless` (Task 1);
  - `s.auth.Commands.SetUp` and `authcommand.SetUp`/`SetUpResult` (Task 2);
  - existing `s.tenant.Queries.LocateWorkspace` (`tenantquery.LocateWorkspace{Slug}`), `tenantdomain.ErrTenantNotFound` and `s.tenant.Commands.CreateWorkspace`;
  - existing helpers `setSessionCookie`, `resolveAuthLocale`, `userPayload`, `tenantPayload`, `writeJSON`, `writeError` and `s.fail`.
- Produces the wire contract the frontend (Task 4) and C3/C4 rely on:
  - `GET /api/platform/config` (public) → `200 {"features":{"billing":bool,"multi_tenant":bool},"setup_required":bool}`.
  - `POST /api/platform/signup` in single-tenant mode, body `{email,password,name,setup_token}`:
    - `201 {"user":{id,email,name,locale},"tenant":{id,slug,name,status}}`, with the session cookie set;
    - `403 setup_token_invalid`;
    - `403 setup_complete`;
    - `409 email_taken`;
    - `400` on invalid input.
  - `POST /api/platform/tenants` in single-tenant mode: the slug is always `NVELOPE_TENANT_SLUG`. `409 slug_taken` once the workspace exists.

- [ ] **Step 1: Add the config hook to the test server**

In `internal/api/helpers_test.go`, turn `newTestServer` into a wrapper. Rename the existing function to `newTestServerWith(t *testing.T, adjust func(*config.Config)) *testServer`, and right after the `cfg := config.Config{...}` literal add:

```go
	if adjust != nil {
		adjust(&cfg)
	}
```

Above it:

```go
func newTestServer(t *testing.T) *testServer { return newTestServerWith(t, nil) }
```

Give `newTestServerWith` this doc comment: `// newTestServerWith is newTestServer with a hook that adjusts the config before the application is built.` Move the existing `newTestServer` doc comment onto the wrapper.

- [ ] **Step 2: Write the failing tests**

Create `internal/api/setup_handlers_test.go`:

```go
package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/config"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

const testSetupToken = "setup-token-0123456789abcdef-0123456789"

// newSingleTenantServer is a single-tenant install whose workspace slug is
// unique to the test, so first-run setup is open on the shared database.
func newSingleTenantServer(t *testing.T) (*testServer, string) {
	t.Helper()
	slug := "solo-" + dbtest.RandString()
	ts := newTestServerWith(t, func(c *config.Config) {
		c.Runtime = config.RuntimeServerless
		c.TenantSlug = slug
		c.SetupToken = testSetupToken
	})
	return ts, slug
}

func setupBody(email, token string) map[string]string {
	return map[string]string{
		"email": email, "password": "correct-horse-battery", "name": "Owner", "setup_token": token,
	}
}

func TestPlatformConfigSaaS(t *testing.T) {
	ts := newTestServer(t)

	status, body := ts.do(ts.newClient(), http.MethodGet, "/api/platform/config", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, map[string]any{"billing": true, "multi_tenant": true}, body["features"])
	require.Equal(t, false, body["setup_required"])
}

func TestSingleTenantSetup(t *testing.T) {
	ts, slug := newSingleTenantServer(t)
	anon := ts.newClient()

	status, body := ts.do(anon, http.MethodGet, "/api/platform/config", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, map[string]any{"billing": false, "multi_tenant": false}, body["features"])
	require.Equal(t, true, body["setup_required"])

	// No token and a wrong token are refused, and create nothing: the same
	// email signs up below.
	email := dbtest.RandString() + "@example.com"
	for _, token := range []string{"", "wrong-token"} {
		status, body = ts.do(anon, http.MethodPost, "/api/platform/signup", setupBody(email, token))
		require.Equal(t, http.StatusForbidden, status, "token %q", token)
		require.Equal(t, "setup_token_invalid", body["code"])
	}

	// The right token creates the owner, signed in, and the workspace.
	status, body = ts.request(http.MethodPost, "/api/platform/signup", setupBody(email, testSetupToken))
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, slug, body["tenant"].(map[string]any)["slug"])
	require.Equal(t, email, body["user"].(map[string]any)["email"])

	status, body = ts.request(http.MethodGet, "/api/platform/me", nil)
	require.Equal(t, http.StatusOK, status)
	tenants := body["tenants"].([]any)
	require.Len(t, tenants, 1)
	require.Equal(t, slug, tenants[0].(map[string]any)["slug"])
	require.Equal(t, "owner", tenants[0].(map[string]any)["role"])
	ts.enterWorkspace(slug)

	// Setup is closed now, even with the right token.
	status, body = ts.do(anon, http.MethodGet, "/api/platform/config", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, false, body["setup_required"])
	status, body = ts.do(anon, http.MethodPost, "/api/platform/signup",
		setupBody("second-"+email, testSetupToken))
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, "setup_complete", body["code"])

	// The one workspace exists, so another cannot be created.
	status, body = ts.request(http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Other", "slug": "other-" + dbtest.RandString(),
	})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "slug_taken", body["code"])

	// The owner was created verified, so a fresh login works.
	status, _ = ts.do(ts.newClient(), http.MethodPost, "/api/platform/login", map[string]string{
		"email": email, "password": "correct-horse-battery",
	})
	require.Equal(t, http.StatusOK, status)
}

// TestSingleTenantCreateTenantPinsSlug covers recovery from a setup that
// created the owner but not the workspace: the owner creates it from the
// landing page and gets the configured slug, whatever they typed.
func TestSingleTenantCreateTenantPinsSlug(t *testing.T) {
	ts, slug := newSingleTenantServer(t)
	// A verified account with no workspace. A SaaS server on the same
	// database makes it, since single-tenant signup always creates both.
	email := newTestServer(t).signup()
	status, _ := ts.request(http.MethodPost, "/api/platform/login", map[string]string{
		"email": email, "password": "a-good-password",
	})
	require.Equal(t, http.StatusOK, status)

	status, body := ts.request(http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Mine", "slug": "typed-" + dbtest.RandString(),
	})
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, slug, body["tenant"].(map[string]any)["slug"])

	status, body = ts.request(http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Again", "slug": "again-" + dbtest.RandString(),
	})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "slug_taken", body["code"])
}
```

- [ ] **Step 3: Run them to verify they fail**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/api/ -run 'TestPlatformConfigSaaS|TestSingleTenant' -v`
Expected: FAIL. `/api/platform/config` answers 404 or 405, and the setup signup answers 201 with the SaaS `verification` body.

- [ ] **Step 4: Implement the handlers**

In `internal/api/server.go`, in the public part of the `/api/platform` route (after `r.Post("/signup", s.handleSignup)`):

```go
		r.Get("/config", s.handlePlatformConfig)
```

In `internal/api/platform_handlers.go`, add `"context"` to the imports along with `tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"`, and make `handleSignup` begin with:

```go
func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	// A single-tenant install has no open registration: the first account
	// comes from first-run setup, everyone else from an invitation.
	if s.cfg.SingleTenant() {
		s.handleSetupSignup(w, r)
		return
	}
```

In `handleCreateTenant`, right after the body is decoded:

```go
	// A single-tenant install has exactly one workspace, at the configured
	// slug, so once it exists this answers 409 slug_taken.
	if s.cfg.SingleTenant() {
		req.Slug = s.cfg.TenantSlug
	}
```

Add after `handleSignup`:

```go
// handleSetupSignup is first-run setup of a single-tenant install. With the
// setup token, and only while the install's workspace does not exist yet, it
// creates the owner's account (verified and signed in) and the workspace.
func (s *Server) handleSetupSignup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		credentialsRequest
		SetupToken string `json:"setup_token"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	done, err := s.setupDone(r.Context())
	if err != nil {
		s.fail(w, r, "setup", err)
		return
	}
	if done {
		writeError(w, r, http.StatusForbidden, "setup_complete",
			"this install is already set up; ask a workspace admin for an invitation")
		return
	}
	// ponytail: two concurrent setups with the right token both pass the check
	// above; the second CreateWorkspace then fails on the slug and leaves a
	// verified account with no workspace. Only the token holder can race, so
	// it is not guarded; the upgrade path is one transaction across auth and
	// tenant, as onboarding does.
	account, err := s.auth.Commands.SetUp.Handle(r.Context(), authcommand.SetUp{
		Token: req.SetupToken, Email: req.Email, Password: req.Password, Name: req.Name,
	})
	if err != nil {
		s.fail(w, r, "setup", err)
		return
	}
	// The session is set before the workspace is created: if that step fails,
	// the owner is still signed in and creates the workspace from the landing
	// page (handleCreateTenant pins the slug).
	s.setSessionCookie(w, account.SessionToken)
	locale := s.resolveAuthLocale(w, r, account.UserID, "")
	ws, err := s.tenant.Commands.CreateWorkspace.Handle(r.Context(), tenantcommand.CreateWorkspace{
		OwnerID: account.UserID, Name: s.cfg.TenantSlug, Slug: s.cfg.TenantSlug,
	})
	if err != nil {
		s.fail(w, r, "setup workspace", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":   userPayload(account.UserID, account.UserEmail, account.UserName, locale),
		"tenant": tenantPayload(ws.TenantID, ws.Slug, ws.Name, ws.Status),
	})
}

// setupDone reports whether first-run setup has created the single-tenant
// install's workspace.
func (s *Server) setupDone(ctx context.Context) (bool, error) {
	_, err := s.tenant.Queries.LocateWorkspace.Handle(ctx,
		tenantquery.LocateWorkspace{Slug: s.cfg.TenantSlug})
	if errors.Is(err, tenantdomain.ErrTenantNotFound) {
		return false, nil
	}
	return err == nil, err
}

// handlePlatformConfig tells the SPA which runtime it talks to. It is public:
// the signup page reads setup_required before anyone has signed in.
func (s *Server) handlePlatformConfig(w http.ResponseWriter, r *http.Request) {
	single := s.cfg.SingleTenant()
	setupRequired := false
	if single {
		done, err := s.setupDone(r.Context())
		if err != nil {
			s.fail(w, r, "platform config", err)
			return
		}
		setupRequired = !done
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"features":       map[string]bool{"billing": !single, "multi_tenant": !single},
		"setup_required": setupRequired,
	})
}
```

If `LocateWorkspace` is not decorated as `decorator.QueryHandler[tenantquery.LocateWorkspace, tenantquery.ResolvedWorkspace]` in `tenantapp.Queries`, adapt the call to its real signature. It is used the same way in `internal/api/middleware.go` and `public_middleware.go`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -w internal/api && export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/api/ -run 'TestPlatformConfigSaaS|TestSingleTenant|TestSignup|TestCreate' -v`
Expected: PASS, including the existing SaaS signup and tenant tests.

- [ ] **Step 6: Run the whole Go suite and the linters**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./... -timeout 15m && golangci-lint run ./... && export PATH="$HOME/go/bin:$PATH" && make lint-arch`
Expected: PASS, with no lint findings.

- [ ] **Step 7: Commit**

```bash
git add internal/api
git commit -m "feat(api): add first-run setup signup and the public platform config endpoint"
```

---

### Task 4: Frontend flags and the setup-token field

**Files:**
- Modify: `frontend/src/lib/api-types.ts`, `frontend/src/lib/api.ts`, `frontend/src/lib/query.ts`
- Create: `frontend/src/hooks/use-platform-config.ts`
- Modify: `frontend/src/components/shell/sidebar.tsx`, `app-shell.tsx`, `top-bar.tsx`
- Modify: `frontend/src/routes/index.tsx`, `login.tsx`, `signup.tsx`
- Modify: `frontend/src/locales/en/auth.json`, `frontend/src/locales/ru/auth.json`, regenerate `frontend/src/i18n/resources.d.ts`
- Test: `frontend/src/routes/signup.test.tsx`, `frontend/src/routes/login.test.tsx`, `frontend/src/components/shell/app-shell.test.tsx`

**Interfaces:**
- Consumes: the wire contract of Task 3.
- Produces: `api.config(): Promise<ApiResult<PlatformConfig>>`, `usePlatformConfig(): { billing: boolean; multiTenant: boolean; setupRequired: boolean }`, `queryKeys.platformConfig()`.

All commands below run from `frontend/` with `export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH" && `. Run `pnpm install --frozen-lockfile` once first if `node_modules` is missing.

- [ ] **Step 1: Write the failing tests**

In `frontend/src/routes/signup.test.tsx`:

1. Replace the router mock and the api mock, and add a `beforeEach`:

```tsx
const navigate = vi.hoisted(() => vi.fn())

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: unknown) => opts,
  useNavigate: () => navigate,
  Link: ({ children, to, ...rest }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"} {...rest}>
      {children as never}
    </a>
  ),
}))

vi.mock("@/lib/api", () => ({ api: { signup: vi.fn(), config: vi.fn() } }))

const saasConfig = {
  status: 200,
  ok: true,
  data: { features: { billing: true, multi_tenant: true }, setup_required: false },
}

beforeEach(() => {
  vi.mocked(api.config).mockResolvedValue(saasConfig)
})
```

Add `beforeEach` to the `vitest` import.

2. In the first test ("registers an account…"), after `renderWithClient(<Signup />)`, add:

```tsx
    expect(screen.queryByLabelText(/setup token/i)).toBeNull()
```

3. Add a test inside `describe("Signup", …)`:

```tsx
  it("in first-run setup, sends the setup token and enters the new workspace", async () => {
    vi.mocked(api.config).mockResolvedValue({
      status: 200,
      ok: true,
      data: { features: { billing: false, multi_tenant: false }, setup_required: true },
    })
    vi.mocked(api.signup).mockResolvedValue({
      status: 201,
      ok: true,
      data: {
        user: { id: "u1", name: "Ann Lee", email: "ann@example.com", locale: null },
        tenant: { id: "t1", slug: "acme", name: "acme", status: "active" },
      },
    })
    renderWithClient(<Signup />)
    const token = await screen.findByLabelText(/setup token/i)
    fill()
    fireEvent.change(token, { target: { value: "the-setup-token" } })
    fireEvent.click(screen.getByRole("button", { name: /^set up$/i }))

    await waitFor(() =>
      expect(api.signup).toHaveBeenCalledWith(
        "ann@example.com",
        "supersecret",
        "Ann Lee",
        "the-setup-token",
      ),
    )
    await waitFor(() =>
      expect(navigate).toHaveBeenCalledWith({
        to: "/t/$slug",
        params: { slug: "acme" },
      }),
    )
  })
```

In `frontend/src/routes/login.test.tsx`, add `config: vi.fn().mockResolvedValue({ status: 200, ok: true, data: { features: { billing: true, multi_tenant: true }, setup_required: false } })` to its `@/lib/api` mock, since `Login` now reads the flags.

In `frontend/src/components/shell/app-shell.test.tsx`, add a flag mock and a test:

```tsx
const flags = vi.hoisted(() => ({ billing: true }))
vi.mock("@/hooks/use-platform-config", () => ({
  usePlatformConfig: () => ({ billing: flags.billing, multiTenant: true, setupRequired: false }),
}))
```

Change the suspension-banner mock to render a marker: `vi.mock("./suspension-banner", () => ({ SuspensionBanner: () => <div data-testid="suspension-banner" /> }))`. Then add:

```tsx
  it("drops the billing banner when billing is off", () => {
    state.fullscreen = false
    flags.billing = false
    render(
      <AppShell slug="acme" workspaceName="Acme">
        <div data-testid="page" />
      </AppShell>,
    )
    expect(screen.queryByTestId("suspension-banner")).toBeNull()
    flags.billing = true
  })
```

- [ ] **Step 2: Run them to verify they fail**

Run: `pnpm exec vitest run src/routes/signup.test.tsx src/components/shell/app-shell.test.tsx`
Expected: FAIL. There is no setup-token field, and the banner still renders.

- [ ] **Step 3: API types, client and the hook**

`frontend/src/lib/api-types.ts`, after `PlatformAccount`:

```ts
// GET /api/platform/config — which runtime the SPA talks to. A single-tenant
// (serverless) install has no billing and one workspace; setup_required is
// true until first-run setup has created that workspace.
export type PlatformConfig = {
  features: { billing: boolean; multi_tenant: boolean }
  setup_required: boolean
}

// The workspace object the platform plane returns (tenantPayload in Go).
export type PlatformTenant = {
  id: string
  slug: string
  name: string
  status: string
}

// POST /api/platform/signup: SaaS asks the user to verify their email;
// first-run setup signs the owner in and creates the workspace.
export type SignupResult =
  | { verification: { required: boolean; email: string } }
  | { user: PlatformUser; tenant: PlatformTenant }
```

`frontend/src/lib/api.ts`: add `PlatformConfig` and `SignupResult` to the type import, and replace `signup` with:

```ts
  config: () => request<PlatformConfig>("GET", "/api/platform/config"),
  signup: (email: string, password: string, name: string, setupToken?: string) =>
    request<SignupResult>(
      "POST",
      "/api/platform/signup",
      setupToken === undefined
        ? { email, password, name }
        : { email, password, name, setup_token: setupToken },
    ),
```

`frontend/src/lib/query.ts`, in `queryKeys` after `tenants`:

```ts
  platformConfig: () => ["platform-config"] as const,
```

Create `frontend/src/hooks/use-platform-config.ts`:

```ts
// The runtime flags from GET /api/platform/config. Until the answer arrives,
// or if it fails, the SaaS defaults apply, so SaaS never flickers.

import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"

export function usePlatformConfig() {
  const query = useQuery({
    queryKey: queryKeys.platformConfig(),
    queryFn: async () => (await api.config()).data,
    staleTime: Infinity,
  })
  return {
    billing: query.data?.features.billing ?? true,
    multiTenant: query.data?.features.multi_tenant ?? true,
    setupRequired: query.data?.setup_required ?? false,
  }
}
```

- [ ] **Step 4: Hide billing and workspace switching**

`frontend/src/components/shell/sidebar.tsx`: import `usePlatformConfig`, read `const { billing } = usePlatformConfig()` in `WorkspaceSidebar`, and extend the filter:

```tsx
              {NAV.filter(
                (entry) =>
                  (billing || entry.segment !== "billing") &&
                  (entry.requires.length === 0 || canAny(entry.requires)),
              ).map((entry) => {
```

`frontend/src/components/shell/app-shell.tsx`: import `usePlatformConfig`. Read `const { billing } = usePlatformConfig()` at the top of `AppShell`, before the `fullscreen` early return so the hook order is stable. Render `{billing && <SuspensionBanner slug={slug} />}`.

`frontend/src/components/shell/top-bar.tsx`: import `usePlatformConfig`, read `const { multiTenant } = usePlatformConfig()`, and wrap the "switch workspace" item:

```tsx
            {multiTenant && (
              <DropdownMenuItem onClick={() => navigate({ to: "/" })}>
                {t("account.switchWorkspace")}
              </DropdownMenuItem>
            )}
```

`frontend/src/routes/index.tsx`: import `usePlatformConfig` and read `const { multiTenant, setupRequired } = usePlatformConfig()` in `Home`, next to `useSession()`.
- Render the unauthenticated card's "Sign up" button only when `multiTenant || setupRequired`.
- Render the "Create workspace" button below the workspace list only when `multiTenant`.
- Keep the empty-state "Create workspace" button: it is the single-tenant recovery path.

`frontend/src/routes/login.tsx`: import `usePlatformConfig`, read `const { multiTenant, setupRequired } = usePlatformConfig()`, and render the `CardFooter` "need an account? Sign up" paragraph only when `multiTenant || setupRequired`.

- [ ] **Step 5: The setup-token field on the signup page**

In `frontend/src/locales/en/auth.json`, add to `"signup"`:

```json
    "setupTitle": "Set up nvelope",
    "setupDescription": "Create the owner account. Your workspace is created with it.",
    "setupToken": "Setup token",
    "setupTokenHint": "Read it with the command the Terraform output prints.",
    "setupTokenRequired": "Enter the setup token.",
    "setupTokenInvalid": "The setup token is wrong.",
    "setupComplete": "This install is already set up. Ask a workspace admin for an invitation.",
    "setupSubmit": "Set up"
```

In `frontend/src/locales/ru/auth.json`, add to `"signup"`:

```json
    "setupTitle": "Настройка nvelope",
    "setupDescription": "Создайте аккаунт владельца. Рабочее пространство будет создано вместе с ним.",
    "setupToken": "Токен установки",
    "setupTokenHint": "Его можно получить командой из вывода Terraform.",
    "setupTokenRequired": "Введите токен установки.",
    "setupTokenInvalid": "Неверный токен установки.",
    "setupComplete": "Установка уже настроена. Попросите администратора рабочего пространства прислать приглашение.",
    "setupSubmit": "Настроить"
```

Run `pnpm i18n:types` to regenerate `src/i18n/resources.d.ts`.

In `frontend/src/routes/signup.tsx`:
- import `useNavigate` from `@tanstack/react-router`, `usePlatformConfig` from `@/hooks/use-platform-config`, and `queryClient`, `queryKeys` from `@/lib/query`;
- in `Signup`: `const navigate = useNavigate()` and `const { setupRequired } = usePlatformConfig()`;
- the mutation becomes:

```tsx
  const signup = useMutation({
    mutationFn: (v: { name: string; email: string; password: string; setupToken: string }) =>
      setupRequired
        ? api.signup(v.email.trim(), v.password, v.name.trim(), v.setupToken.trim())
        : api.signup(v.email.trim(), v.password, v.name.trim()),
    onSuccess: async (res) => {
      if ("tenant" in res.data) {
        // First-run setup signs the owner in and creates the workspace.
        await queryClient.invalidateQueries({ queryKey: queryKeys.me() })
        await queryClient.invalidateQueries({ queryKey: queryKeys.platformConfig() })
        await navigate({ to: "/t/$slug", params: { slug: res.data.tenant.slug } })
        return
      }
      // No session is issued — the account must verify its email first.
      setSentTo(res.data.verification.email)
    },
    onError: (e) => {
      if (isConflict(e)) {
        setEmailTaken(true)
        return
      }
      if (e instanceof ApiError && e.slug === "email_domain_not_allowed") {
        setFormError(t("signup.emailDomainNotAllowed"))
        return
      }
      if (e instanceof ApiError && e.slug === "setup_token_invalid") {
        setFormError(t("signup.setupTokenInvalid"))
        return
      }
      if (e instanceof ApiError && e.slug === "setup_complete") {
        setFormError(t("signup.setupComplete"))
        return
      }
      setFormError(errorMessage(e))
    },
  })
```

- `useForm` `defaultValues` becomes `{ name: "", email: "", password: "", setupToken: "" }`;
- the title and description use `t(setupRequired ? "signup.setupTitle" : "signup.title")` and `t(setupRequired ? "signup.setupDescription" : "signup.description")`;
- after the password field, render the token field only in setup mode:

```tsx
            {setupRequired && (
              <form.Field
                name="setupToken"
                validators={{
                  onBlur: compose(rules.required(t("signup.setupTokenRequired"))),
                }}
              >
                {(field) => (
                  <FormField
                    label={t("signup.setupToken")}
                    type="password"
                    required
                    autoComplete="off"
                    hint={t("signup.setupTokenHint")}
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    error={fieldError(field.state.meta.errors)}
                  />
                )}
              </form.Field>
            )}
```

- the submit button label is `t(setupRequired ? "signup.setupSubmit" : "signup.submit")` when not pending.

- [ ] **Step 6: Run the tests, the type check and the linters**

Run: `pnpm exec vitest run src/routes/signup.test.tsx src/components/shell/app-shell.test.tsx && pnpm test && pnpm typecheck && pnpm lint && pnpm i18n:lint`
Expected: PASS. Any other vitest file that renders `Signup`, `Login`, `Home`, `TopBar`, `WorkspaceSidebar` or `AppShell` and mocks `@/lib/api` without `config` falls back to the SaaS defaults through the failed query. If one fails on an unhandled rejection, add `config: vi.fn().mockResolvedValue(<saasConfig>)` to its api mock.

- [ ] **Step 7: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): hide billing and workspace switching on single-tenant installs and add the setup-token signup"
```

---

### Task 5: Record the decisions in the spec, final verification

**Files:**
- Modify: `docs/superpowers/specs/2026-11-14-serverless-onprem-design.md`

- [ ] **Step 1: Update the spec**

In "On-prem packaging → Single tenant and first run":
- Replace the bullet "While no user exists, `POST /api/platform/signup` requires the setup token…" and its sub-bullets with:

```markdown
- While the workspace at `NVELOPE_TENANT_SLUG` does not exist, `POST /api/platform/signup`
  requires the setup token (`setup_token` in the body, at least 32 characters by config
  validation). A valid token:
  - creates the first user, **skipping email verification**, because nothing can
    send mail before the first login and holding the token already proves the
    right to install. The user is signed in at once (`UserRepository.CreateWithSession`);
  - creates the tenant through the existing `create_workspace` command, with the
    user as owner. Its name starts as the slug; the owner renames it in Settings.

  Without the token: 403 `setup_token_invalid`. Once the workspace exists: 403
  `setup_complete`. Plan C2 keyed "setup is open" on the workspace, not on "no user
  exists": it reuses the `LocateWorkspace` query, it is testable on a shared database,
  and the cron sweeps and invitations need the workspace anyway.
- If the account is created but the workspace is not, the owner is already signed in:
  the landing page offers "Create workspace", and `POST /api/platform/tenants` always
  uses `NVELOPE_TENANT_SLUG` in single-tenant mode.
```

- Replace "`POST /api/platform/tenants` returns 409 once the tenant exists." with "`POST /api/platform/tenants` pins the slug to `NVELOPE_TENANT_SLUG`, so it returns 409 `slug_taken` once the tenant exists."
- Replace "The setup-token comparison is constant-time." with "The setup-token comparison is constant-time: both sides are hashed with SHA-256 first, so the length does not leak either."

In "UI feature flags", replace the first bullet with:

```markdown
- A public `GET /api/platform/config` returns
  `{"features": {"billing": bool, "multi_tenant": bool}, "setup_required": bool}`
  (snake_case, like the rest of the platform plane). It is a hand-written route:
  only the tenant plane is spec-first so far, and the route moves into
  `api/openapi/platform.yaml` when the platform plane migrates (Plan C2).
```

and replace "No other frontend changes." with "The signup links on the landing and login pages are hidden once a single-tenant install is set up. No other frontend changes."

In "Ceilings", add after the fixed-window throttle entry:

```markdown
- Two concurrent first-run setups with the right token both pass the "workspace does
  not exist" check; the second then fails on the slug and leaves a verified account
  with no workspace. Only the token holder can race. The upgrade path is one
  transaction across auth and tenant, as invitation onboarding does.
```

- [ ] **Step 2: Final verification**

Run from the worktree root:

```bash
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./... -timeout 15m
golangci-lint run ./...
export PATH="$HOME/go/bin:$PATH" && make lint-arch
cd frontend && export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH" && pnpm test && pnpm typecheck && pnpm lint && cd ..
```

Expected: everything passes.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-11-14-serverless-onprem-design.md
git commit -m "docs: record the plan C2 bootstrap decisions in the serverless spec"
```

## Done criteria for Plan C2

- With `NVELOPE_RUNTIME` unset, SaaS behaves as before. The whole Go suite and the vitest suite pass, and the linters are clean.
- In single-tenant mode:
  - signup without the token or with a wrong token answers 403 `setup_token_invalid` and creates nothing;
  - the right token creates a verified, signed-in owner and the workspace at `NVELOPE_TENANT_SLUG`;
  - after that, signup answers 403 `setup_complete` and tenant creation answers 409 `slug_taken`.
- `GET /api/platform/config` is public and reports the flags and `setup_required`.
- The SPA hides billing and workspace switching on a single-tenant install, and shows the setup-token field while setup is open.

## Handover to Plan C4

- C4's serverless composition root wires `authcommand.NewSetUpHandler(ydbUsers, hasher, cfg.SetupToken, cfg.SessionTTL)` into `authapp.Commands.SetUp`. `YDBUsers.CreateWithSession` already persists the verification state.
- `NVELOPE_RUNTIME=serverless` currently still requires the SaaS keys (`NVELOPE_REDIS_URL`, Postbox, the feedback stream). C4 relaxes `Config.Validate` for the serverless runtime.
- C3 sets three variables. `NVELOPE_RUNTIME=serverless` and `NVELOPE_TENANT_SLUG` go on `api` and `worker`. `NVELOPE_SETUP_TOKEN` goes on `api` only, from Lockbox, a `random_password` of at least 32 characters. The API Gateway routes `/api/platform/*`, including the public `/api/platform/config`, to `api`.

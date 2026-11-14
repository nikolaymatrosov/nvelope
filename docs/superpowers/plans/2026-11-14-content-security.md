# Email Content Security Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the content-injection holes from the 2026-11-14 audit: raw HTML is sanitized on save (tenant-configurable), public pages get CSP with a sandboxed archive, merge values are HTML-escaped, and tracking attribution is bound to the campaign.

**Architecture:**
- A second bluemonday policy, `visualrender.SanitizeDocument`, keeps whole email documents and strips only active content.
- Raw campaign and template saves call it through a consumer-owned `RawHTMLPolicy` port, which is bridged to a new tenant setting.
- A static CSP middleware wraps the anonymous HTML routes, and the archive page swaps in a `sandbox` CSP.
- Substitution escapes values in the HTML part; transactional sends gain a raw `{{{key}}}` form whose output is always sanitized.

**Tech Stack:** Go 1.x, chi, pgx v5, bluemonday v1.0.27, testify, PostgreSQL 17 via testcontainers. Frontend: React 19, TanStack Query/Form, react-i18next, radix-ui, vitest.

**Spec:** `docs/superpowers/specs/2026-11-14-content-security-design.md`

## Global Constraints

- No production data: no backfill, no feature flag, no compatibility window.
- Raw-HTML levels are exactly `standard` (default) and `off`. Switching to `off` requires `accept_risk: true`, error code `risk_acceptance_required`.
- The visual save path keeps the strict `visualrender.Sanitize` under every level.
- `{{{key}}}` output in transactional HTML is always passed through `SanitizeDocument`.
- Body cap: 1 MiB (`1 << 20` bytes) for `bodyHTML` and `bodyText`. Error code `body-too-large`.
- pgx convention: 4 or more placeholders use `pgx.NamedArgs` with `@snake_case`; fewer use positional `$N`.
- Paths in commands are relative to the repo root. Never prefix commands with `cd <repo> &&`.
- Integration tests need Docker. Prefix Go test commands with `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock";`.
- Frontend commands need node on PATH. Prefix them with `export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH";`. Run frontend tests with `.claude/scripts/test-frontend.sh <path>` from the repo root.
- Commit messages: Conventional Commits. Never add a `Co-Authored-By` trailer.
- `make lint-arch` needs `go-cleanarch`: prefix with `export PATH="$HOME/go/bin:$PATH";`.

## File map

| File | Change | Responsibility |
|------|--------|----------------|
| `internal/campaign/visualrender/sanitize.go` | modify | add `SanitizeDocument` and its policy |
| `internal/campaign/visualrender/sanitize_document_test.go` | create | policy tests |
| `internal/tenant/domain/settings.go`, `errors.go` | modify | `RawHTMLSanitization` level on `TenantSettings` |
| `internal/db/migrations/000023_raw_html_sanitization.{up,down}.sql` | create | column |
| `internal/tenant/adapters/settings_pg.go` | modify | persist the column |
| `internal/tenant/app/query/{views,get_settings}.go` | modify | expose the level |
| `internal/tenant/app/command/set_raw_html_sanitization.go` | create | change the level |
| `internal/tenant/app/application.go`, `internal/service/application.go` | modify | wiring |
| `internal/api/tenant_handlers.go`, `internal/api/server.go` | modify | `PUT /settings/raw-html-sanitization` |
| `internal/campaign/app/command/raw_html_policy.go` | create | `RawHTMLPolicy` port and helper |
| `internal/campaign/app/command/{campaigns,templates}.go` | modify | sanitize raw saves, return warnings |
| `internal/campaign/app/application.go` | modify | update handlers become result commands |
| `internal/service/bridges.go` | modify | `RawHTMLPolicy` bridge over tenant settings |
| `internal/api/campaign_handlers.go` | modify | `warnings` in create/update responses |
| `internal/campaign/domain/body_size.go` | create | 1 MiB cap |
| `internal/campaign/domain/{campaign,template,errors}.go` | modify | enforce the cap |
| `internal/api/public_security.go` | create | CSP and hardening headers |
| `internal/api/archive_handlers.go` | modify | sandbox CSP on archive pages |
| `internal/sending/domain/substitution.go` | modify | escape the HTML part, add `SubstituteSubject` |
| `internal/campaign/adapters/batch_worker.go` | modify | use `SubstituteSubject` |
| `internal/campaign/domain/tracking.go` | modify | single-pass `ApplyVariables` / `ApplyHTMLVariables` |
| `internal/campaign/app/command/transactional.go` | modify | escape, raw form, sanitize |
| `internal/campaign/adapters/tracking_pg.go` | modify | campaign-bound attribution |
| `internal/api/tracking_handlers.go` | modify | redirect even when attribution fails |
| `internal/tenant/domain/branding.go` | modify | https-only logo |
| `frontend/src/lib/{api,api-types}.ts` | modify | settings envelope, new endpoint |
| `frontend/src/routes/t/$slug/settings/index.tsx` | modify | sanitization card and risk dialog |
| `frontend/src/routes/t/$slug/transactional/index.tsx` | modify | variables note |
| `frontend/src/locales/{en,ru}/{settings,transactional}.json`, `frontend/src/i18n/resources.d.ts` | modify | copy |

---

### Task 1: `SanitizeDocument` policy

**Files:**
- Modify: `internal/campaign/visualrender/sanitize.go` (append below `sanitizeHTML`)
- Create: `internal/campaign/visualrender/sanitize_document_test.go`

**Interfaces:**
- Consumes: existing `onEventAttrRE` (`sanitize.go`), `domain.RenderWarning{Kind, Detail}`.
- Produces: `func SanitizeDocument(bodyHTML string) (sanitized string, warnings []domain.RenderWarning)`. It returns at most one warning, with `Kind == "sanitizer_stripped"`, and returns `("", nil)` for empty input.

- [ ] **Step 1: Write the failing tests**

Create `internal/campaign/visualrender/sanitize_document_test.go`:

```go
package visualrender

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// emailDoc is a typical hand-written email: doctype, head with meta and a
// responsive <style>, an MSO conditional comment, legacy table attributes,
// unrestricted inline style, a data: image, and an outbound link.
const emailDoc = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Hi</title>
<style>@media (max-width:600px){.col{width:100%!important}} td > p{margin:0}</style>
<!--[if mso]><table><tr><td width="600"><![endif]--></head>
<body bgcolor="#ffffff"><table width="600" cellpadding="0"><tr><td bgcolor="#eeeeee" style="padding:8px;position:relative">
<center><font color="#333333" face="Arial">Hello</font></center>
<img src="data:image/png;base64,iVBORw0KGgo=" alt="dot"><a href="https://acme.example/sale">Sale</a>
</td></tr></table></body></html>`

func TestSanitizeDocument_KeepsEmailDocument(t *testing.T) {
	t.Parallel()
	out, warnings := SanitizeDocument(emailDoc)
	require.Empty(t, warnings, "a clean email document raises no warning")
	for _, want := range []string{
		`<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN"`,
		`<meta charset="utf-8">`,
		`<meta name="viewport" content="width=device-width">`,
		`<style>@media (max-width:600px){.col{width:100%!important}} td > p{margin:0}</style>`,
		`<!--[if mso]>`,
		`<body bgcolor="#ffffff">`,
		`style="padding:8px;position:relative"`,
		`<center><font color="#333333" face="Arial">Hello</font></center>`,
		`src="data:image/png;base64,iVBORw0KGgo="`,
		`href="https://acme.example/sale"`,
	} {
		require.Contains(t, out, want)
	}
}

func TestSanitizeDocument_StripsActiveContent(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"script":          `<p>x</p><script>alert(1)</script>`,
		"event handler":   `<body onload="steal()"><p>x</p></body>`,
		"javascript href": `<a href="javascript:alert(1)">x</a>`,
		"data html href":  `<a href="data:text/html,<b>x</b>">x</a>`,
		"form":            `<form action="https://evil.example"><input name="card"><button>Pay</button></form>`,
		"iframe":          `<iframe src="https://evil.example"></iframe>`,
		"meta refresh":    `<head><meta http-equiv="refresh" content="0;url=https://evil.example"></head>`,
		"base":            `<head><base href="https://evil.example/"></head>`,
		"svg script":      `<svg><script>alert(1)</script></svg>`,
		"link":            `<link rel="stylesheet" href="https://evil.example/x.css">`,
		"js background":   `<table><tr><td background="javascript:alert(1)">x</td></tr></table>`,
	}
	banned := []string{"<script", "alert(", "onload", "javascript:", "data:text", "<form",
		"<input", "<button", "<iframe", "http-equiv", "<base", "<svg", "<link"}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out, warnings := SanitizeDocument(in)
			require.Len(t, warnings, 1, "stripping must raise the sanitizer_stripped warning")
			require.Equal(t, "sanitizer_stripped", warnings[0].Kind)
			for _, b := range banned {
				require.NotContains(t, out, b)
			}
		})
	}
}

func TestSanitizeDocument_NoFalseWarnings(t *testing.T) {
	t.Parallel()
	_, warnings := SanitizeDocument(`<p>Update your profile: metadata: none</p>`)
	require.Empty(t, warnings, "words ending in 'file:' or 'data:' are not URL schemes")
}

func TestSanitizeDocument_DoctypeCannotCarryMarkup(t *testing.T) {
	t.Parallel()
	out, _ := SanitizeDocument(`<!DOCTYPE html "><script>alert(1)</script>`)
	require.NotContains(t, out, "<script")
}

func TestSanitizeDocument_Idempotent(t *testing.T) {
	t.Parallel()
	once, _ := SanitizeDocument(emailDoc + `<script>x</script>`)
	twice, warnings := SanitizeDocument(once)
	require.Equal(t, once, twice)
	require.Empty(t, warnings)
}

func TestSanitizeDocument_Empty(t *testing.T) {
	t.Parallel()
	out, warnings := SanitizeDocument("")
	require.Empty(t, out)
	require.Nil(t, warnings)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/campaign/visualrender/ -run TestSanitizeDocument -count=1`
Expected: FAIL to compile with `undefined: SanitizeDocument`.

- [ ] **Step 3: Implement**

Append to `internal/campaign/visualrender/sanitize.go`. `strings` and `regexp` are already imported.

```go
// SanitizeDocument is the save-time gate for raw (non-visual) campaign and
// template HTML under a tenant's "standard" raw-HTML level.
//
// Unlike Sanitize, it keeps a full email document intact: doctype, <head>,
// <style> blocks with media queries, MSO conditional comments, legacy table
// attributes, and unrestricted inline style.
//
// It strips only active content: scripts, frames, plugins, forms, <base>,
// <link>, inline SVG/MathML, event handlers, meta refresh, and script or
// non-image data: URLs. It returns one sanitizer_stripped warning when the
// input held any such construct.
func SanitizeDocument(bodyHTML string) (sanitized string, warnings []domain.RenderWarning) {
	if bodyHTML == "" {
		return "", nil
	}
	out := documentPolicy.Sanitize(bodyHTML)
	// bluemonday drops the doctype, and without one the archive renders in
	// quirks mode. The match excludes '<' and '>', so it cannot carry markup.
	if m := doctypeRE.FindStringSubmatch(bodyHTML); m != nil {
		out = m[1] + out
	}
	if documentStripped(bodyHTML) {
		warnings = append(warnings, domain.RenderWarning{
			Kind:   "sanitizer_stripped",
			Detail: "removed active HTML (scripts, frames, forms, event handlers, or unsafe URLs)",
		})
	}
	return out, warnings
}

var (
	doctypeRE               = regexp.MustCompile(`(?i)^\s*(<!doctype\s+html[^<>]*>)`)
	documentDisallowedTagRE = regexp.MustCompile(`(?i)<\s*(script|iframe|frame|frameset|object|embed|applet|form|input|button|select|textarea|base|link|svg|math)\b`)
	metaRefreshRE           = regexp.MustCompile(`(?i)http-equiv\s*=\s*["']?\s*refresh`)
	scriptSchemeRE          = regexp.MustCompile(`(?i)\b(?:javascript|vbscript|file):`)
	dataURIRE               = regexp.MustCompile(`(?i)\bdata:[a-z0-9.+-]*/?`)
	httpURLRE               = regexp.MustCompile(`(?i)^https?://`)
	metaNameRE              = regexp.MustCompile(`(?i)^(viewport|format-detection|x-apple-disable-message-reformatting|color-scheme|supported-color-schemes)$`)
	metaHTTPEquivRE         = regexp.MustCompile(`(?i)^(content-type|x-ua-compatible)$`)
)

// documentStripped reports whether the input held a construct the document
// policy removes. It drives the operator-facing warning only. The policy
// does the stripping.
func documentStripped(in string) bool {
	if documentDisallowedTagRE.MatchString(in) || onEventAttrRE.MatchString(in) ||
		metaRefreshRE.MatchString(in) || scriptSchemeRE.MatchString(in) {
		return true
	}
	for _, m := range dataURIRE.FindAllString(in, -1) {
		if !strings.HasPrefix(strings.ToLower(m), "data:image/") {
			return true
		}
	}
	return false
}

// documentPolicy is the bluemonday policy behind SanitizeDocument. It is
// built once; a Policy is safe for concurrent use.
var documentPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// Keeping <style> elements requires AllowUnsafe. <script> is never on the
	// allow-list, so it is still removed together with its content. CSS
	// cannot run script in current engines, and the tokenizer applies the
	// browser's raw-text rules, so the first </style> always closes the
	// element.
	p.AllowUnsafe(true)
	p.AllowElements("html", "head", "body", "title", "style", "meta", "center", "font")
	p.AllowNoAttrs().OnElements("center", "font")
	p.AllowComments() // MSO conditional comments
	p.AllowAttrs("style", "class", "id").Globally()
	p.AllowAttrs("color", "face", "size").OnElements("font")
	p.AllowAttrs("bgcolor", "align", "valign", "width", "height", "border",
		"cellpadding", "cellspacing").
		OnElements("body", "table", "thead", "tbody", "tfoot", "tr", "td", "th",
			"img", "div", "p", "center")
	p.AllowAttrs("background").Matching(httpURLRE).OnElements("body", "table", "td", "th")
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")
	p.AllowAttrs("charset").OnElements("meta")
	p.AllowAttrs("name").Matching(metaNameRE).OnElements("meta")
	p.AllowAttrs("http-equiv").Matching(metaHTTPEquivRE).OnElements("meta")
	p.AllowAttrs("content").OnElements("meta")
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	p.AllowDataURIImages()
	return p
}()
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/campaign/visualrender/ -count=1`
Expected: PASS, including the existing `Sanitize` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/campaign/visualrender/sanitize.go internal/campaign/visualrender/sanitize_document_test.go
git commit -m "feat(campaign): add a document-preserving sanitizer for raw email HTML"
```

---

### Task 2: Tenant raw-HTML sanitization level

**Files:**
- Modify: `internal/tenant/domain/settings.go`, `internal/tenant/domain/errors.go`
- Create: `internal/db/migrations/000023_raw_html_sanitization.up.sql`, `internal/db/migrations/000023_raw_html_sanitization.down.sql`
- Modify: `internal/tenant/adapters/settings_pg.go`
- Modify: `internal/tenant/app/query/views.go`, `internal/tenant/app/query/get_settings.go`
- Create: `internal/tenant/app/command/set_raw_html_sanitization.go`
- Modify: `internal/tenant/app/application.go`, `internal/service/application.go` (line ~210)
- Test: `internal/tenant/domain/settings_test.go`, `internal/tenant/adapters/settings_pg_test.go`, `internal/tenant/app/command/command_test.go`

**Interfaces:**
- Produces:
  - `type RawHTMLSanitization string` and the constants `RawHTMLStandard = "standard"`, `RawHTMLOff = "off"`, all in `tenant/domain`.
  - `func (s *TenantSettings) RawHTMLSanitization() RawHTMLSanitization`.
  - `func (s *TenantSettings) SetRawHTMLSanitization(level RawHTMLSanitization, acceptRisk bool) error`.
  - `var ErrRiskNotAccepted` with code `risk_acceptance_required`.
  - `HydrateTenantSettings(tenantID, displayName, timezone string, rawHTMLSanitization RawHTMLSanitization)`, which gains a 4th parameter.
  - `query.SettingsView.RawHTMLSanitization string` with JSON key `raw_html_sanitization`.
  - In `tenant/app/command`: `SetRawHTMLSanitization{TenantID, Level string; AcceptRisk bool}` and `SetRawHTMLSanitizationResult{Previous, Current string}`.
  - `NewSetRawHTMLSanitizationHandler(settings domain.SettingsRepository)`.
  - `tenantapp.Commands.SetRawHTMLSanitization`.

- [ ] **Step 1: Write the failing domain tests**

Append to `internal/tenant/domain/settings_test.go`:

```go
func TestRawHTMLSanitizationDefaultsToStandard(t *testing.T) {
	t.Parallel()
	s, err := domain.NewTenantSettings("tenant-1", "Acme")
	require.NoError(t, err)
	require.Equal(t, domain.RawHTMLStandard, s.RawHTMLSanitization())
}

func TestSetRawHTMLSanitization(t *testing.T) {
	t.Parallel()
	s := domain.HydrateTenantSettings("tenant-1", "Acme", "UTC", domain.RawHTMLStandard)

	require.ErrorIs(t, s.SetRawHTMLSanitization(domain.RawHTMLOff, false), domain.ErrRiskNotAccepted)
	require.Equal(t, domain.RawHTMLStandard, s.RawHTMLSanitization(), "a refused change leaves the level alone")

	require.NoError(t, s.SetRawHTMLSanitization(domain.RawHTMLOff, true))
	require.Equal(t, domain.RawHTMLOff, s.RawHTMLSanitization())

	require.NoError(t, s.SetRawHTMLSanitization(domain.RawHTMLStandard, false),
		"turning protection back on needs no acceptance")
	require.Equal(t, domain.RawHTMLStandard, s.RawHTMLSanitization())

	require.Error(t, s.SetRawHTMLSanitization("lenient", true))
}
```

Then update the two existing `HydrateTenantSettings(...)` calls in this file (lines ~29 and ~40) by adding `, domain.RawHTMLStandard` as the 4th argument.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tenant/domain/ -count=1`
Expected: FAIL to compile with `undefined: domain.RawHTMLStandard`.

- [ ] **Step 3: Implement the domain**

In `internal/tenant/domain/errors.go`, inside the existing `var (...)` block, add:

```go
	// ErrRiskNotAccepted is returned when a tenant turns raw-HTML
	// sanitization off without explicitly accepting the risk.
	ErrRiskNotAccepted = apperr.NewIncorrectInput("risk_acceptance_required",
		"turning raw HTML sanitization off requires accepting the risk")
```

In `internal/tenant/domain/settings.go`:

```go
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
```

- Add the field `rawHTMLSanitization RawHTMLSanitization` to `TenantSettings`.
- In `NewTenantSettings`, return `&TenantSettings{tenantID: tenantID, displayName: displayName, rawHTMLSanitization: RawHTMLStandard}`.
- Change `HydrateTenantSettings` to:

```go
func HydrateTenantSettings(tenantID, displayName, timezone string,
	rawHTMLSanitization RawHTMLSanitization) *TenantSettings {
	return &TenantSettings{tenantID: tenantID, displayName: displayName,
		timezone: timezone, rawHTMLSanitization: rawHTMLSanitization}
}
```

Add:

```go
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
```

- [ ] **Step 4: Run the domain tests**

Run: `go test ./internal/tenant/domain/ -count=1`
Expected: PASS.

- [ ] **Step 5: Migration**

`internal/db/migrations/000023_raw_html_sanitization.up.sql`:

```sql
-- raw_html_sanitization is how a tenant's raw (non-visual) campaign and
-- template HTML is treated on save: 'standard' strips active content, 'off'
-- stores it verbatim (only after the tenant explicitly accepted the risk).
ALTER TABLE tenant_settings
    ADD COLUMN raw_html_sanitization text NOT NULL DEFAULT 'standard'
        CHECK (raw_html_sanitization IN ('standard', 'off'));
```

`internal/db/migrations/000023_raw_html_sanitization.down.sql`:

```sql
ALTER TABLE tenant_settings DROP COLUMN raw_html_sanitization;
```

- [ ] **Step 6: Write the failing adapter test**

Append to `internal/tenant/adapters/settings_pg_test.go`:

```go
func TestSettingsRawHTMLSanitizationRoundTrips(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSettings(pool)
	ctx := context.Background()
	ownerID := insertUser(t, pool)
	ws := createWorkspace(t, adapters.NewTenants(pool), ownerID)

	settings, err := repo.Get(ctx, ws.ID())
	require.NoError(t, err)
	require.Equal(t, domain.RawHTMLStandard, settings.RawHTMLSanitization(), "new tenants default to standard")

	require.NoError(t, repo.Update(ctx, ws.ID(), func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
		return s, s.SetRawHTMLSanitization(domain.RawHTMLOff, true)
	}))
	settings, err = repo.Get(ctx, ws.ID())
	require.NoError(t, err)
	require.Equal(t, domain.RawHTMLOff, settings.RawHTMLSanitization())
	require.Equal(t, "Workspace", settings.DisplayName(), "other fields survive the update")
}
```

- [ ] **Step 7: Implement the adapter**

In `internal/tenant/adapters/settings_pg.go`, in both `Get` and `Update`:

- Read the column: `var displayName, timezone, rawHTML string` and `SELECT display_name, timezone, raw_html_sanitization FROM tenant_settings` (`Update` keeps its `FOR UPDATE`), scanning into `&displayName, &timezone, &rawHTML`.
- Hydrate with `domain.HydrateTenantSettings(tenantID, displayName, timezone, domain.RawHTMLSanitization(rawHTML))`.
- Write it in `Update`:

```go
		if _, err := tx.Exec(ctx,
			`UPDATE tenant_settings
			    SET display_name = $1, timezone = $2, raw_html_sanitization = $3, updated_at = now()`,
			updated.DisplayName(), updated.Timezone(), string(updated.RawHTMLSanitization())); err != nil {
			return fmt.Errorf("updating tenant settings: %w", err)
		}
```

- [ ] **Step 8: Run the adapter tests**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./internal/tenant/adapters/ -run TestSettings -count=1`
Expected: PASS.

- [ ] **Step 9: Write the failing command test**

Append to `internal/tenant/app/command/command_test.go`. Also update the existing `HydrateTenantSettings("tenant-1", "Old", "UTC")` call (line ~89) to pass `domain.RawHTMLStandard` as the 4th argument.

```go
func TestSetRawHTMLSanitization(t *testing.T) {
	t.Parallel()
	settings := newFakeSettings()
	settings.byTenant["tenant-1"] = domain.HydrateTenantSettings("tenant-1", "Acme", "UTC", domain.RawHTMLStandard)
	h := command.NewSetRawHTMLSanitizationHandler(settings)

	_, err := h.Handle(context.Background(), command.SetRawHTMLSanitization{TenantID: "tenant-1", Level: "off"})
	require.ErrorIs(t, err, domain.ErrRiskNotAccepted)

	res, err := h.Handle(context.Background(), command.SetRawHTMLSanitization{
		TenantID: "tenant-1", Level: "off", AcceptRisk: true,
	})
	require.NoError(t, err)
	require.Equal(t, command.SetRawHTMLSanitizationResult{Previous: "standard", Current: "off"}, res)

	stored, err := settings.Get(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Equal(t, domain.RawHTMLOff, stored.RawHTMLSanitization())
}
```

Run: `go test ./internal/tenant/app/command/ -count=1`
Expected: FAIL to compile with `undefined: command.NewSetRawHTMLSanitizationHandler`.

- [ ] **Step 10: Implement the command, query view, and wiring**

Create `internal/tenant/app/command/set_raw_html_sanitization.go`:

```go
package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// SetRawHTMLSanitization is the request to change how a workspace's raw
// campaign and template HTML is sanitized on save.
type SetRawHTMLSanitization struct {
	TenantID   string
	Level      string
	AcceptRisk bool
}

// SetRawHTMLSanitizationResult carries the level before and after, for the
// audit trail.
type SetRawHTMLSanitizationResult struct {
	Previous string
	Current  string
}

// SetRawHTMLSanitizationHandler handles the SetRawHTMLSanitization command.
type SetRawHTMLSanitizationHandler struct {
	settings domain.SettingsRepository
}

// NewSetRawHTMLSanitizationHandler builds the handler, failing fast on a nil
// dependency.
func NewSetRawHTMLSanitizationHandler(settings domain.SettingsRepository) SetRawHTMLSanitizationHandler {
	if settings == nil {
		panic("nil settings repository")
	}
	return SetRawHTMLSanitizationHandler{settings: settings}
}

// Handle applies the new level inside the tenant-bound transaction.
func (h SetRawHTMLSanitizationHandler) Handle(ctx context.Context,
	cmd SetRawHTMLSanitization) (SetRawHTMLSanitizationResult, error) {
	var res SetRawHTMLSanitizationResult
	err := h.settings.Update(ctx, cmd.TenantID,
		func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
			res.Previous = string(s.RawHTMLSanitization())
			if err := s.SetRawHTMLSanitization(domain.RawHTMLSanitization(cmd.Level), cmd.AcceptRisk); err != nil {
				return nil, err
			}
			res.Current = string(s.RawHTMLSanitization())
			return s, nil
		})
	if err != nil {
		return SetRawHTMLSanitizationResult{}, err
	}
	return res, nil
}
```

In `internal/tenant/app/query/views.go`, add a field to `SettingsView`:

```go
	RawHTMLSanitization string `json:"raw_html_sanitization"`
```

In `internal/tenant/app/query/get_settings.go`:

```go
	return SettingsView{DisplayName: s.DisplayName(), Timezone: s.Timezone(),
		RawHTMLSanitization: string(s.RawHTMLSanitization())}, nil
```

In `internal/tenant/app/application.go`, add to `Commands`:

```go
	SetRawHTMLSanitization decorator.ResultCommandHandler[command.SetRawHTMLSanitization, command.SetRawHTMLSanitizationResult]
```

In `internal/service/application.go`, next to `UpdateSettings:` (line ~210):

```go
			SetRawHTMLSanitization: decorator.ApplyResultCommandDecorators(
				tenantcommand.NewSetRawHTMLSanitizationHandler(settings), "SetRawHTMLSanitization", logger),
```

- [ ] **Step 11: Run the tenant tests and build**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go build ./... && go test ./internal/tenant/... -count=1`
Expected: build succeeds; PASS. If the build reports another `HydrateTenantSettings` caller, add `domain.RawHTMLStandard` as its 4th argument.

- [ ] **Step 12: Commit**

```bash
git add internal/tenant internal/db/migrations/000023_raw_html_sanitization.up.sql internal/db/migrations/000023_raw_html_sanitization.down.sql internal/service/application.go
git commit -m "feat(tenant): add a raw-HTML sanitization level that needs explicit risk acceptance to turn off"
```

---

### Task 3: `PUT /settings/raw-html-sanitization` endpoint

**Files:**
- Modify: `internal/api/tenant_handlers.go`, `internal/api/server.go` (authz group, around line 124)
- Test: `internal/api/handlers_test.go`

**Interfaces:**
- Consumes: `s.tenant.Commands.SetRawHTMLSanitization` (Task 2), `s.requirePermission`, `iamdomain.PermSettingsManage`, `s.recordAudit`.
- Produces:
  - `PUT /t/{slug}/api/settings/raw-html-sanitization`, body `{"level": "standard"|"off", "accept_risk": bool}`, responds `200 {"raw_html_sanitization": "<level>"}`.
  - `GET /t/{slug}/api/settings` now includes `settings.raw_html_sanitization`.

- [ ] **Step 1: Write the failing API test**

Append to `internal/api/handlers_test.go`:

```go
func TestSetRawHTMLSanitization(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodGet, base+"/settings", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "standard", body["settings"].(map[string]any)["raw_html_sanitization"])

	status, body = ts.request(http.MethodPut, base+"/settings/raw-html-sanitization",
		map[string]any{"level": "off"})
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "risk_acceptance_required", body["code"])

	status, body = ts.request(http.MethodPut, base+"/settings/raw-html-sanitization",
		map[string]any{"level": "off", "accept_risk": true})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "off", body["raw_html_sanitization"])

	status, body = ts.request(http.MethodGet, base+"/audit", nil)
	require.Equal(t, http.StatusOK, status)
	var found map[string]any
	for _, rec := range body["records"].([]any) {
		if r := rec.(map[string]any); r["Action"] == "settings.raw_html_sanitization" {
			found = r
		}
	}
	require.NotNil(t, found, "the change is audited")
	require.Equal(t, map[string]any{"from": "standard", "to": "off"}, found["Metadata"])
}
```

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./internal/api/ -run TestSetRawHTMLSanitization -count=1`
Expected: FAIL. The GET assertion passes; the PUT returns 404 or 405.

- [ ] **Step 2: Implement the handler and route**

In `internal/api/tenant_handlers.go`, add the import `iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"` and append:

```go
// handleSetRawHTMLSanitization changes how the workspace's raw campaign and
// template HTML is sanitized on save. Turning it off requires accept_risk;
// every change is audited.
func (s *Server) handleSetRawHTMLSanitization(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSettingsManage); !ok {
		return
	}
	var req struct {
		Level      string `json:"level"`
		AcceptRisk bool   `json:"accept_risk"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.tenant.Commands.SetRawHTMLSanitization.Handle(r.Context(),
		tenantcommand.SetRawHTMLSanitization{TenantID: ws.ID, Level: req.Level, AcceptRisk: req.AcceptRisk})
	if err != nil {
		s.fail(w, r, "set raw html sanitization", err)
		return
	}
	if res.Previous != res.Current {
		s.recordAudit(r.Context(), "settings.raw_html_sanitization", ws.ID,
			map[string]any{"from": res.Previous, "to": res.Current})
	}
	writeJSON(w, http.StatusOK, map[string]any{"raw_html_sanitization": res.Current})
}
```

In `internal/api/server.go`, inside the `r.Group(func(r chi.Router) { r.Use(s.authz) ...` block, right after `s.mountGeneratedTenantAPI(r)`, add:

```go
			// Workspace content-safety setting (feature: content security).
			r.Put("/settings/raw-html-sanitization", s.handleSetRawHTMLSanitization)
```

- [ ] **Step 3: Run the test**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./internal/api/ -run 'TestSetRawHTMLSanitization|TestGetAndUpdateSettings' -count=1`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/api/tenant_handlers.go internal/api/server.go internal/api/handlers_test.go
git commit -m "feat(api): expose the raw-HTML sanitization level behind settings:manage"
```

---

### Task 4: Sanitize raw campaign and template saves

**Files:**
- Create: `internal/campaign/app/command/raw_html_policy.go`
- Modify: `internal/campaign/app/command/campaigns.go`, `internal/campaign/app/command/templates.go`
- Modify: `internal/campaign/app/application.go`
- Modify: `internal/service/bridges.go`, `internal/service/application.go` (`buildCampaign`, lines ~436-501)
- Modify: `internal/api/campaign_handlers.go`
- Test: `internal/campaign/app/command/fakes_test.go`, `campaigns_test.go`, `templates_test.go`; `internal/api/campaign_handlers_test.go`

**Interfaces:**
- Consumes:
  - `visualrender.SanitizeDocument` (Task 1).
  - `tenantdomain.RawHTMLOff` and `TenantSettings.RawHTMLSanitization()` (Task 2).
  - `PUT /settings/raw-html-sanitization` (Task 3, used by the API test).
- Produces:
  - `type RawHTMLPolicy interface { SanitizesRawHTML(ctx context.Context, tenantID string) (bool, error) }` in `campaign/app/command`.
  - Handler constructors:
    - `NewCreateCampaignHandler(campaigns, templates, policy RawHTMLPolicy)`
    - `NewUpdateCampaignHandler(campaigns, policy RawHTMLPolicy)`
    - `NewCreateTemplateHandler(templates, policy RawHTMLPolicy)`
    - `NewUpdateTemplateHandler(templates, policy RawHTMLPolicy)`
  - New result fields and types:
    - `CreateCampaignResult.Warnings`
    - `UpdateCampaignResult{Warnings []domain.RenderWarning}`
    - `CreateTemplateResult.Warnings`
    - `UpdateTemplateResult{Warnings []domain.RenderWarning}`
  - `UpdateCampaign` and `UpdateTemplate` become `ResultCommandHandler`s.
  - `service.NewRawHTMLPolicy(settings tenantdomain.SettingsRepository)`.

- [ ] **Step 1: Add the test fake**

Append to `internal/campaign/app/command/fakes_test.go`:

```go
// fakeRawHTMLPolicy is a RawHTMLPolicy with a fixed answer.
type fakeRawHTMLPolicy struct {
	sanitize bool
	err      error
}

func (p fakeRawHTMLPolicy) SanitizesRawHTML(context.Context, string) (bool, error) {
	return p.sanitize, p.err
}
```

- [ ] **Step 2: Write the failing command tests**

Append to `internal/campaign/app/command/campaigns_test.go`, adding `"errors"` to its imports:

```go
func TestCreateCampaignSanitizesRawHTML(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	h := command.NewCreateCampaignHandler(campaigns, newFakeTemplateRepo(), fakeRawHTMLPolicy{sanitize: true})
	res, err := h.Handle(context.Background(), command.CreateCampaign{
		TenantID: "tenant-1", Name: "Spring",
		BodyHTML: `<style>p{color:red}</style><p onclick="x()">Hi</p><script>alert(1)</script>`,
	})
	require.NoError(t, err)
	stored := campaigns.byID[res.CampaignID].BodyHTML()
	require.Contains(t, stored, "<style>p{color:red}</style>", "the standard level keeps style blocks")
	require.NotContains(t, stored, "<script")
	require.NotContains(t, stored, "onclick")
	require.Len(t, res.Warnings, 1)
}

func TestCreateCampaignStoresRawHTMLVerbatimWhenOff(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	h := command.NewCreateCampaignHandler(campaigns, newFakeTemplateRepo(), fakeRawHTMLPolicy{sanitize: false})
	body := `<p>Hi</p><script>alert(1)</script>`
	res, err := h.Handle(context.Background(), command.CreateCampaign{
		TenantID: "tenant-1", Name: "Spring", BodyHTML: body,
	})
	require.NoError(t, err)
	require.Equal(t, body, campaigns.byID[res.CampaignID].BodyHTML())
	require.Empty(t, res.Warnings)
}

func TestCreateCampaignFailsClosedWhenPolicyUnreadable(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	boom := errors.New("settings unavailable")
	h := command.NewCreateCampaignHandler(campaigns, newFakeTemplateRepo(), fakeRawHTMLPolicy{err: boom})
	_, err := h.Handle(context.Background(), command.CreateCampaign{
		TenantID: "tenant-1", Name: "Spring", BodyHTML: "<p>x</p>",
	})
	require.ErrorIs(t, err, boom)
	require.Empty(t, campaigns.byID, "nothing is stored")
}

func TestUpdateCampaignSanitizesRawHTML(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	id := addDraftCampaign(t, campaigns, "dom-1", nil)
	h := command.NewUpdateCampaignHandler(campaigns, fakeRawHTMLPolicy{sanitize: true})
	res, err := h.Handle(context.Background(), command.UpdateCampaign{
		TenantID: "tenant-1", CampaignID: id, Name: "N", Subject: "S",
		BodyHTML: `<p>ok</p><iframe src="https://evil.example"></iframe>`,
	})
	require.NoError(t, err)
	require.NotContains(t, campaigns.byID[id].BodyHTML(), "<iframe")
	require.Len(t, res.Warnings, 1)
}
```

Append to `internal/campaign/app/command/templates_test.go`:

```go
func TestCreateTemplateSanitizesRawHTML(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	h := command.NewCreateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true})
	res, err := h.Handle(context.Background(), command.CreateTemplate{
		TenantID: "tenant-1", Name: "Reset", Kind: "transactional", Subject: "Reset",
		BodyHTML: `<p>Reset</p><form action="https://evil.example"><input name="pw"></form>`,
	})
	require.NoError(t, err)
	tpl, err := templates.Get(context.Background(), "tenant-1", res.TemplateID)
	require.NoError(t, err)
	require.NotContains(t, tpl.BodyHTML(), "<form")
	require.Len(t, res.Warnings, 1)
}

func TestUpdateTemplateSanitizesRawHTML(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	id := seedTemplate(t, templates, domain.KindCampaign)
	h := command.NewUpdateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true})
	res, err := h.Handle(context.Background(), command.UpdateTemplate{
		TenantID: "tenant-1", TemplateID: id, Name: "Tpl", Subject: "S",
		BodyHTML: `<p>ok</p><script>alert(1)</script>`,
	})
	require.NoError(t, err)
	tpl, err := templates.Get(context.Background(), "tenant-1", id)
	require.NoError(t, err)
	require.NotContains(t, tpl.BodyHTML(), "<script")
	require.Len(t, res.Warnings, 1)
}
```

Update the existing calls in `campaigns_test.go`:
- Lines ~29 and ~49: `command.NewCreateCampaignHandler(campaigns, templates)` becomes `command.NewCreateCampaignHandler(campaigns, templates, fakeRawHTMLPolicy{sanitize: true})`.
- Line ~154: `command.NewUpdateCampaignHandler(campaigns)` becomes `command.NewUpdateCampaignHandler(campaigns, fakeRawHTMLPolicy{sanitize: true})`.
- In that same test, change `require.NoError(t, h.Handle(context.Background(), command.UpdateCampaign{...}))` to `_, err := h.Handle(...)` followed by `require.NoError(t, err)`.
- Change the later `err := h.Handle(...)` to `_, err = h.Handle(...)`.

Run: `go test ./internal/campaign/app/command/ -count=1`
Expected: FAIL to compile, with errors about too many arguments to the constructors.

- [ ] **Step 3: Implement the port and handlers**

Create `internal/campaign/app/command/raw_html_policy.go`:

```go
package command

import (
	"context"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/visualrender"
)

// RawHTMLPolicy reports whether a tenant's raw (non-visual) campaign and
// template HTML is sanitized on save. The tenant context owns the setting;
// the service layer bridges it to this consumer-owned port.
type RawHTMLPolicy interface {
	SanitizesRawHTML(ctx context.Context, tenantID string) (bool, error)
}

// applyRawHTMLPolicy runs body through SanitizeDocument unless the tenant
// has turned raw-HTML sanitization off. A policy lookup failure fails the
// save: HTML is never stored unsanitized because the setting could not be
// read.
func applyRawHTMLPolicy(ctx context.Context, policy RawHTMLPolicy, tenantID, body string) (
	string, []domain.RenderWarning, error) {
	if strings.TrimSpace(body) == "" {
		return body, nil, nil
	}
	sanitize, err := policy.SanitizesRawHTML(ctx, tenantID)
	if err != nil {
		return "", nil, err
	}
	if !sanitize {
		return body, nil, nil
	}
	out, warnings := visualrender.SanitizeDocument(body)
	return out, warnings, nil
}
```

In `internal/campaign/app/command/campaigns.go`:

- `CreateCampaignResult` gains `Warnings []domain.RenderWarning`.
- `CreateCampaignHandler` gains a `policy RawHTMLPolicy` field. Its constructor becomes `NewCreateCampaignHandler(campaigns domain.CampaignRepository, templates domain.TemplateRepository, policy RawHTMLPolicy)`, with `policy == nil` added to the nil-dependency panic.
- In `CreateCampaignHandler.Handle`, right after the `if cmd.TemplateID != "" { ... }` block and before `domain.NewCampaign(...)`, insert:

```go
	bodyHTML, warnings, err := applyRawHTMLPolicy(ctx, h.policy, cmd.TenantID, bodyHTML)
	if err != nil {
		return CreateCampaignResult{}, err
	}
```

  Then change the final return to `return CreateCampaignResult{CampaignID: id, Warnings: warnings}, nil`.
- Add:

```go
// UpdateCampaignResult carries the sanitizer's warnings for the saved body.
type UpdateCampaignResult struct {
	Warnings []domain.RenderWarning
}
```

- `UpdateCampaignHandler` gains `policy RawHTMLPolicy`. Its constructor becomes `NewUpdateCampaignHandler(campaigns domain.CampaignRepository, policy RawHTMLPolicy)`, panicking on nil for either.
- Replace `UpdateCampaignHandler.Handle` with:

```go
// Handle sanitizes the raw body per the tenant's policy, applies the new
// content to a draft campaign, and replaces its targets.
func (h UpdateCampaignHandler) Handle(ctx context.Context, cmd UpdateCampaign) (UpdateCampaignResult, error) {
	bodyHTML, warnings, err := applyRawHTMLPolicy(ctx, h.policy, cmd.TenantID, cmd.BodyHTML)
	if err != nil {
		return UpdateCampaignResult{}, err
	}
	if err := h.campaigns.Update(ctx, cmd.TenantID, cmd.CampaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, c.Recompose(cmd.Name, cmd.Subject, bodyHTML, cmd.BodyText,
				cmd.FromName, cmd.FromLocalPart, cmd.SendingDomainID)
		}); err != nil {
		return UpdateCampaignResult{}, err
	}
	if err := h.campaigns.SaveTargets(ctx, cmd.TenantID, cmd.CampaignID,
		buildTargets(cmd.ListIDs, cmd.Segments)); err != nil {
		return UpdateCampaignResult{}, err
	}
	return UpdateCampaignResult{Warnings: warnings}, nil
}
```

In `internal/campaign/app/command/templates.go`:

- `CreateTemplateResult` gains `Warnings []domain.RenderWarning`.
- `CreateTemplateHandler` gains `policy RawHTMLPolicy`, with constructor `NewCreateTemplateHandler(templates domain.TemplateRepository, policy RawHTMLPolicy)`.
- Its `Handle` sanitizes `cmd.BodyHTML` with `applyRawHTMLPolicy(ctx, h.policy, cmd.TenantID, cmd.BodyHTML)` before `domain.NewTemplate`, passes the result, and returns `CreateTemplateResult{TemplateID: id, Warnings: warnings}`.
- Add `type UpdateTemplateResult struct { Warnings []domain.RenderWarning }`.
- `UpdateTemplateHandler` gains `policy RawHTMLPolicy`, with constructor `NewUpdateTemplateHandler(templates domain.TemplateRepository, policy RawHTMLPolicy)`.
- Its `Handle` becomes:

```go
// Handle sanitizes the raw body per the tenant's policy and applies the new
// template content inside the tenant-bound transaction.
func (h UpdateTemplateHandler) Handle(ctx context.Context, cmd UpdateTemplate) (UpdateTemplateResult, error) {
	bodyHTML, warnings, err := applyRawHTMLPolicy(ctx, h.policy, cmd.TenantID, cmd.BodyHTML)
	if err != nil {
		return UpdateTemplateResult{}, err
	}
	if err := h.templates.Update(ctx, cmd.TenantID, cmd.TemplateID,
		func(tpl *domain.Template) (*domain.Template, error) {
			return tpl, tpl.Recompose(cmd.Name, cmd.Subject, bodyHTML, cmd.BodyText)
		}); err != nil {
		return UpdateTemplateResult{}, err
	}
	return UpdateTemplateResult{Warnings: warnings}, nil
}
```

- [ ] **Step 4: Run the command tests**

Run: `go test ./internal/campaign/app/command/ -count=1`
Expected: PASS.

- [ ] **Step 5: Wire the application, bridge, and API**

In `internal/campaign/app/application.go`:

```go
	UpdateTemplate    decorator.ResultCommandHandler[command.UpdateTemplate, command.UpdateTemplateResult]
	...
	UpdateCampaign    decorator.ResultCommandHandler[command.UpdateCampaign, command.UpdateCampaignResult]
```

Append to `internal/service/bridges.go`, adding the import `campaigncommand "github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"` if absent:

```go
// rawHTMLPolicy adapts the tenant context's settings to the campaign
// context's RawHTMLPolicy port.
type rawHTMLPolicy struct {
	settings tenantdomain.SettingsRepository
}

var _ campaigncommand.RawHTMLPolicy = (*rawHTMLPolicy)(nil)

// NewRawHTMLPolicy builds the bridge over the tenant settings repository.
func NewRawHTMLPolicy(settings tenantdomain.SettingsRepository) *rawHTMLPolicy {
	return &rawHTMLPolicy{settings: settings}
}

// SanitizesRawHTML reports whether the tenant keeps raw-HTML sanitization on.
func (p *rawHTMLPolicy) SanitizesRawHTML(ctx context.Context, tenantID string) (bool, error) {
	s, err := p.settings.Get(ctx, tenantID)
	if err != nil {
		return false, err
	}
	return s.RawHTMLSanitization() != tenantdomain.RawHTMLOff, nil
}
```

In `internal/service/application.go` `buildCampaign`, next to `mediaRefs := ...` (line ~445), add `rawPolicy := NewRawHTMLPolicy(tenantadapters.NewSettings(pool))`. Then change four entries:

```go
			CreateTemplate: decorator.ApplyResultCommandDecorators(
				campaigncommand.NewCreateTemplateHandler(templates, rawPolicy), "CreateTemplate", logger),
			UpdateTemplate: decorator.ApplyResultCommandDecorators(
				campaigncommand.NewUpdateTemplateHandler(templates, rawPolicy), "UpdateTemplate", logger),
			...
			CreateCampaign: decorator.ApplyResultCommandDecorators(
				campaigncommand.NewCreateCampaignHandler(campaigns, templates, rawPolicy), "CreateCampaign", logger),
			UpdateCampaign: decorator.ApplyResultCommandDecorators(
				campaigncommand.NewUpdateCampaignHandler(campaigns, rawPolicy), "UpdateCampaign", logger),
```

In `internal/api/campaign_handlers.go`:

- Give `respondTemplate` and `respondCampaign` a variadic warnings tail. Replace the final `writeJSON(w, status, view)` in each with the block below. Use `campaignquery.TemplateView` in `respondTemplate` and `campaignquery.CampaignView` in `respondCampaign`.

```go
func (s *Server) respondTemplate(w http.ResponseWriter, r *http.Request, tenantID, id string, status int,
	warnings ...campaigndomain.RenderWarning) {
	view, err := s.campaign.Queries.GetTemplate.Handle(r.Context(), campaignquery.GetTemplate{
		TenantID: tenantID, TemplateID: id,
	})
	if err != nil {
		s.fail(w, r, "get template", err)
		return
	}
	if len(warnings) == 0 {
		writeJSON(w, status, view)
		return
	}
	// Embedding flattens the view, so warnings is an additive top-level key.
	writeJSON(w, status, struct {
		campaignquery.TemplateView
		Warnings []campaigndomain.RenderWarning `json:"warnings"`
	}{view, warnings})
}
```

- `handleCreateTemplate`: `s.respondTemplate(w, r, ws.ID, res.TemplateID, http.StatusCreated, res.Warnings...)`.
- `handleUpdateTemplate`: replace `if err := s.campaign.Commands.UpdateTemplate.Handle(...); err != nil {` with `res, err := s.campaign.Commands.UpdateTemplate.Handle(...)` followed by `if err != nil {`. End with `s.respondTemplate(w, r, ws.ID, chi.URLParam(r, "id"), http.StatusOK, res.Warnings...)`.
- `handleCreateCampaign`: `s.respondCampaign(w, r, ws.ID, res.CampaignID, http.StatusCreated, res.Warnings...)`.
- `handleUpdateCampaign`: same change as `handleUpdateTemplate`, ending in `s.respondCampaign(..., http.StatusOK, res.Warnings...)`.

- [ ] **Step 6: Write the failing API test**

Append to `internal/api/campaign_handlers_test.go`:

```go
func TestRawCampaignHTMLIsSanitizedOnSave(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name": "Spring", "subject": "S", "list_ids": []string{},
		"body_html": `<style>p{color:red}</style><p>Hi</p><script>fetch("/t/x/api/users")</script>`,
	})
	require.Equal(t, http.StatusCreated, status)
	require.NotContains(t, body["body_html"], "<script")
	require.Contains(t, body["body_html"], "<style>p{color:red}</style>")
	require.NotEmpty(t, body["warnings"], "the caller learns what was stripped")

	// With sanitization off (risk accepted) the same kind of body is stored verbatim.
	status, _ = ts.request(http.MethodPut, base+"/settings/raw-html-sanitization",
		map[string]any{"level": "off", "accept_risk": true})
	require.Equal(t, http.StatusOK, status)
	raw := `<p>Hi</p><script>alert(1)</script>`
	status, body = ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name": "Raw", "subject": "S", "list_ids": []string{}, "body_html": raw,
	})
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, raw, body["body_html"])
	require.Nil(t, body["warnings"])
}
```

- [ ] **Step 7: Run the build and tests**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go build ./... && go test ./internal/campaign/... ./internal/service/... -count=1 && go test ./internal/api/ -run 'TestRawCampaignHTMLIsSanitizedOnSave|TestCampaignTemplateCRUD' -count=1`
Expected: build OK; PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/campaign internal/service internal/api/campaign_handlers.go internal/api/campaign_handlers_test.go
git commit -m "fix(campaign): sanitize raw campaign and template HTML on save"
```

---

### Task 5: 1 MiB body cap

**Files:**
- Create: `internal/campaign/domain/body_size.go`
- Modify: `internal/campaign/domain/errors.go`, `internal/campaign/domain/campaign.go` (`NewCampaign`, `ApplyVisualSave`), `internal/campaign/domain/template.go` (`NewTemplate`, `ApplyVisualSave`)
- Test: `internal/campaign/domain/campaign_test.go`

**Interfaces:**
- Produces: `const MaxBodyBytes = 1 << 20` and `var ErrBodyTooLarge` (code `body-too-large`) in `campaign/domain`.

- [ ] **Step 1: Write the failing test**

Append to `internal/campaign/domain/campaign_test.go`, adding `"strings"` to the imports if absent:

```go
func TestMessageBodiesAreCappedAtOneMiB(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("a", domain.MaxBodyBytes+1)

	_, err := domain.NewCampaign("tenant-1", "Big", "S", big, "", "", "", "", "", 0)
	require.ErrorIs(t, err, domain.ErrBodyTooLarge)
	_, err = domain.NewCampaign("tenant-1", "Big", "S", "", big, "", "", "", "", 0)
	require.ErrorIs(t, err, domain.ErrBodyTooLarge)
	_, err = domain.NewTemplate("tenant-1", "Big", domain.KindCampaign, "S", big, "")
	require.ErrorIs(t, err, domain.ErrBodyTooLarge)

	_, err = domain.NewCampaign("tenant-1", "Fits", "S", strings.Repeat("a", domain.MaxBodyBytes), "", "", "", "", "", 0)
	require.NoError(t, err, "exactly 1 MiB is allowed")
}
```

Run: `go test ./internal/campaign/domain/ -run TestMessageBodiesAreCappedAtOneMiB -count=1`
Expected: FAIL to compile with `undefined: domain.MaxBodyBytes`.

- [ ] **Step 2: Implement**

`internal/campaign/domain/body_size.go`:

```go
package domain

// MaxBodyBytes caps a campaign or template body, HTML or text. Gmail clips
// messages past ~102 KB, so legitimate mail sits far below this; the cap
// only stops abuse.
const MaxBodyBytes = 1 << 20

// checkBodySize rejects an HTML or text body over MaxBodyBytes.
func checkBodySize(bodyHTML, bodyText string) error {
	if len(bodyHTML) > MaxBodyBytes || len(bodyText) > MaxBodyBytes {
		return ErrBodyTooLarge
	}
	return nil
}
```

In `internal/campaign/domain/errors.go`, inside the `var (...)` block:

```go
	// ErrBodyTooLarge is returned when a campaign or template body exceeds
	// MaxBodyBytes.
	ErrBodyTooLarge = apperr.NewIncorrectInput("body-too-large",
		"a message body may not exceed 1 MiB")
```

Insert `if err := checkBodySize(bodyHTML, bodyText); err != nil { return nil, err }` in `NewCampaign` (after the `tenantID` check) and in `NewTemplate` (after the `tenantID` check). `Recompose` calls these constructors and inherits the check.

Insert `if err := checkBodySize(bodyHTML, bodyText); err != nil { return err }` in `Campaign.ApplyVisualSave` and `Template.ApplyVisualSave`, right after their subject check.

- [ ] **Step 3: Run the domain tests**

Run: `go test ./internal/campaign/... -count=1` (prefix with `DOCKER_HOST` as in Global Constraints)
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/campaign/domain
git commit -m "feat(campaign): cap campaign and template bodies at 1 MiB"
```

---

### Task 6: Public-page security headers and sandboxed archive

**Files:**
- Create: `internal/api/public_security.go`, `internal/api/public_security_test.go`
- Modify: `internal/api/server.go` (`mountPublicRoutes`, and the `/p` and `/u` routes at lines ~258-261), `internal/api/archive_handlers.go` (`handleArchiveCampaign`)

**Interfaces:**
- Produces: `const publicCSP`, `const archiveCSP`, `func publicSecurityHeaders(next http.Handler) http.Handler`, all in package `api`.

- [ ] **Step 1: Write the failing tests**

`internal/api/public_security_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicSecurityHeaders(t *testing.T) {
	t.Parallel()
	h := publicSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/acme/subscribe/news", nil))

	require.Equal(t, publicCSP, rec.Header().Get("Content-Security-Policy"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	require.NotContains(t, publicCSP, "script-src", "scripts fall back to default-src 'none'")
	require.Contains(t, archiveCSP, "sandbox ")
}

func TestPublicPagesCarrySecurityHeaders(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	anon := ts.newClient()

	get := func(path string) *http.Response {
		t.Helper()
		resp, err := anon.Get(ts.URL + path)
		require.NoError(t, err)
		_ = resp.Body.Close()
		return resp
	}

	require.Equal(t, publicCSP, get("/t/"+slug+"/archive").Header.Get("Content-Security-Policy"))
	require.Equal(t, publicCSP, get("/p/not-a-token").Header.Get("Content-Security-Policy"))

	archived := get("/t/" + slug + "/archive/00000000-0000-0000-0000-000000000000")
	require.Equal(t, http.StatusNotFound, archived.StatusCode)
	require.Equal(t, archiveCSP, archived.Header.Get("Content-Security-Policy"),
		"the archive page is sandboxed even when it renders an error")
}
```

Run: `go test ./internal/api/ -run TestPublicSecurityHeaders -count=1`
Expected: FAIL to compile with `undefined: publicSecurityHeaders`.

- [ ] **Step 2: Implement**

`internal/api/public_security.go`:

```go
package api

import "net/http"

// publicCSP is the Content-Security-Policy for every anonymous HTML page:
// subscription, confirmation, preferences, unsubscribe, archive index, RSS.
// The pages ship no JavaScript, so script-src falls back to default-src
// 'none'. Images and fonts may come from any https host because the tenant
// logo and tenant CSS reference arbitrary https URLs.
const publicCSP = "default-src 'none'; style-src 'unsafe-inline'; " +
	"img-src 'self' https: data:; font-src 'self' https: data:; " +
	"form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

// archiveCSP replaces publicCSP on an archived campaign page. sandbox gives
// the document an opaque origin, so campaign HTML that slipped past the
// sanitizer, or was saved with sanitization off, can neither run script nor
// reach the API with the visitor's session. allow-popups and
// allow-popups-to-escape-sandbox keep target=_blank links working.
const archiveCSP = "sandbox allow-popups allow-popups-to-escape-sandbox; " +
	"default-src 'none'; style-src 'unsafe-inline'; img-src https: data:; " +
	"font-src https: data:; form-action 'none'; base-uri 'none'; frame-ancestors 'none'"

// publicSecurityHeaders sets the browser-hardening headers on anonymous HTML
// routes. no-referrer keeps /p/{token} and /u/{token} out of Referer headers
// sent to the logo host and to outbound links.
func publicSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", publicCSP)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
```

In `internal/api/server.go` `mountPublicRoutes`, make the headers the first middleware, so that the unknown-slug pages rendered by `resolvePublicTenant` carry them too:

```go
	r.Route("/t/{slug}", func(r chi.Router) {
		r.Use(publicSecurityHeaders)
		r.Use(s.resolvePublicTenant)
```

Replace the four `/p` and `/u` routes (lines ~258-261) with:

```go
	r.Group(func(r chi.Router) {
		r.Use(publicSecurityHeaders)
		r.Get("/p/{token}", s.handlePreferencesForm)
		r.Post("/p/{token}", s.handlePreferencesSubmit)
		r.Get("/u/{token}", s.handleUnsubscribe)
		r.Post("/u/{token}", s.handleUnsubscribe)
	})
```

In `internal/api/archive_handlers.go`, make the first statement of `handleArchiveCampaign`:

```go
	// Campaign HTML is tenant-authored; render it in a sandboxed, opaque
	// origin (overrides the default public CSP).
	w.Header().Set("Content-Security-Policy", archiveCSP)
```

- [ ] **Step 3: Run the tests**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./internal/api/ -run 'TestPublic' -count=1`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/api/public_security.go internal/api/public_security_test.go internal/api/server.go internal/api/archive_handlers.go
git commit -m "fix(api): add CSP to public pages and sandbox the campaign archive"
```

---

### Task 7: Escape merge values in campaign HTML

**Files:**
- Modify: `internal/sending/domain/substitution.go`
- Modify: `internal/campaign/adapters/batch_worker.go:219-221`
- Test: `internal/sending/domain/substitution_test.go`

**Interfaces:**
- Produces:
  - `Substitute(html, text string, sub SubscriberView, ctx CampaignContext) (string, string)` keeps its signature. HTML-part values are now `html.EscapeString`-ed; text stays raw.
  - New: `func SubstituteSubject(subject string, sub SubscriberView, ctx CampaignContext) string`, which inserts raw values.

- [ ] **Step 1: Write the failing tests**

Append to `internal/sending/domain/substitution_test.go`:

```go
func TestSubstituteEscapesValuesInHTMLOnly(t *testing.T) {
	t.Parallel()
	s := sub()
	s.Name = `<a href="https://evil.example">Tom & Jerry</a>`
	html, text := domain.Substitute(`<p>Hi {{ subscriber.name }}</p>`, `Hi {{ subscriber.name }}`, s, ctx())
	require.Equal(t, `<p>Hi &lt;a href=&#34;https://evil.example&#34;&gt;Tom &amp; Jerry&lt;/a&gt;</p>`, html)
	require.Equal(t, `Hi <a href="https://evil.example">Tom & Jerry</a>`, text)
}

func TestSubstituteSubjectInsertsRawValues(t *testing.T) {
	t.Parallel()
	s := sub()
	s.Name = "Tom & Jerry"
	require.Equal(t, "Hi Tom & Jerry", domain.SubstituteSubject("Hi {{ subscriber.name }}", s, ctx()))
}
```

Run: `go test ./internal/sending/domain/ -count=1`
Expected: FAIL to compile with `undefined: domain.SubstituteSubject`.

- [ ] **Step 2: Implement**

In `internal/sending/domain/substitution.go`, add the import `stdhtml "html"`. Replace `Substitute`'s doc comment and body, and add the two functions below it:

```go
// Substitute replaces every `{{ subscriber.<slug> }}` and
// `{{ campaign.<name> }}` placeholder in html and text with its concrete
// value for the recipient. Values are HTML-escaped in html: subscriber data
// (a name, a custom attribute) is untrusted and lands after the body was
// sanitized. text gets the raw value. Unknown slugs are left as the literal
// placeholder; validation already rejected such documents at save time (per
// FR-016c), and silently leaving them at send avoids a per-recipient hard
// fail when a custom field has been deleted between save and send.
func Substitute(html, text string, sub SubscriberView, ctx CampaignContext) (string, string) {
	return replacePlaceholders(html, sub, ctx, stdhtml.EscapeString),
		replacePlaceholders(text, sub, ctx, nil)
}

// SubstituteSubject resolves placeholders in a subject line. Values are
// inserted raw: the subject is a header, Q-encoded by the messenger, and is
// never parsed as HTML.
func SubstituteSubject(subject string, sub SubscriberView, ctx CampaignContext) string {
	return replacePlaceholders(subject, sub, ctx, nil)
}

// replacePlaceholders substitutes every known placeholder in one part.
// encode, when non-nil, is applied to each value before insertion.
func replacePlaceholders(in string, sub SubscriberView, ctx CampaignContext,
	encode func(string) string) string {
	return placeholderRE.ReplaceAllStringFunc(in, func(match string) string {
		m := placeholderRE.FindStringSubmatch(match)
		if len(m) != 3 {
			return match
		}
		value, ok := resolve(m[1], m[2], sub, ctx)
		if !ok {
			return match
		}
		if encode != nil {
			return encode(value)
		}
		return value
	})
}
```

In `internal/campaign/adapters/batch_worker.go`, replace:

```go
			html, text = sendingdomain.Substitute(html, text, sub, cctx)
			subjectHTML, _ := sendingdomain.Substitute(subject, "", sub, cctx)
			subject = subjectHTML
```

with:

```go
			html, text = sendingdomain.Substitute(html, text, sub, cctx)
			subject = sendingdomain.SubstituteSubject(subject, sub, cctx)
```

- [ ] **Step 3: Run the affected suites**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./internal/sending/... ./internal/campaign/adapters/... -count=1 && go test ./internal/api/ -run 'Substitute|Send|Visual' -count=1`
Expected: PASS. If an existing assertion expected a raw `&`, `<`, `>`, `"` or `'` from a substituted value in the HTML part, update it to the escaped form: it was pinning the bug.

- [ ] **Step 4: Commit**

```bash
git add internal/sending/domain internal/campaign/adapters/batch_worker.go
git commit -m "fix(sending): HTML-escape merge values in the HTML part"
```

---

### Task 8: Transactional variables: escape, `{{{raw}}}`, single pass

**Files:**
- Modify: `internal/campaign/domain/tracking.go` (`ApplyVariables`)
- Modify: `internal/campaign/app/command/transactional.go:119-122`
- Test: `internal/campaign/domain/tracking_test.go`, `internal/campaign/app/command/transactional_test.go`

**Interfaces:**
- Consumes: `visualrender.SanitizeDocument` (Task 1).
- Produces:
  - `func ApplyVariables(text string, vars map[string]string) string`: plain parts, raw values, single pass.
  - `func ApplyHTMLVariables(html string, vars map[string]string) (out string, rawUsed bool)`.

- [ ] **Step 1: Write the failing domain tests**

Append to `internal/campaign/domain/tracking_test.go`:

```go
func TestApplyHTMLVariablesEscapesDoubleBraces(t *testing.T) {
	t.Parallel()
	out, raw := domain.ApplyHTMLVariables(`<p>Hi {{name}}</p>`, map[string]string{"name": `<b>Sam</b> & co`})
	require.Equal(t, `<p>Hi &lt;b&gt;Sam&lt;/b&gt; &amp; co</p>`, out)
	require.False(t, raw)
}

func TestApplyHTMLVariablesInsertsTripleBracesRaw(t *testing.T) {
	t.Parallel()
	out, raw := domain.ApplyHTMLVariables(`<table>{{{rows}}}</table>`, map[string]string{"rows": `<tr><td>1</td></tr>`})
	require.Equal(t, `<table><tr><td>1</td></tr></table>`, out)
	require.True(t, raw)
}

func TestApplyVariablesIsSinglePass(t *testing.T) {
	t.Parallel()
	vars := map[string]string{"a": "{{b}}", "b": "leaked"}
	require.Equal(t, "x {{b}} y leaked", domain.ApplyVariables("x {{a}} y {{b}}", vars))
	out, _ := domain.ApplyHTMLVariables("{{a}}", vars)
	require.Equal(t, "{{b}}", out, "an inserted value is never rescanned")
}

func TestApplyVariablesKeepsUnknownKeysAndIsRawInText(t *testing.T) {
	t.Parallel()
	vars := map[string]string{"name": "Tom & Jerry"}
	require.Equal(t, "Hi Tom & Jerry {{missing}}", domain.ApplyVariables("Hi {{name}} {{missing}}", vars))
	require.Equal(t, "Hi Tom & Jerry", domain.ApplyVariables("Hi {{{name}}}", vars))
}
```

Run: `go test ./internal/campaign/domain/ -run ApplyVariables -count=1`
Expected: FAIL to compile with `undefined: domain.ApplyHTMLVariables`.

- [ ] **Step 2: Implement the domain**

In `internal/campaign/domain/tracking.go`, add the import `stdhtml "html"` and replace `ApplyVariables` with:

```go
// variablePattern matches a transactional placeholder: {{{key}}} (group 1,
// raw) or {{key}} (group 2).
var variablePattern = regexp.MustCompile(`\{\{\{([^{}]+)\}\}\}|\{\{([^{}]+)\}\}`)

// ApplyVariables substitutes {{key}} and {{{key}}} placeholders in a plain
// part (subject or text body) with the raw value. It is a single pass, so an
// inserted value is never rescanned. A placeholder with no matching
// variable is left untouched.
func ApplyVariables(text string, vars map[string]string) string {
	out, _ := applyVariables(text, vars, false)
	return out
}

// ApplyHTMLVariables substitutes placeholders in an HTML part: {{key}} is
// HTML-escaped and {{{key}}} is inserted raw. rawUsed reports whether any
// raw placeholder was substituted, so the caller can sanitize the result.
func ApplyHTMLVariables(html string, vars map[string]string) (out string, rawUsed bool) {
	return applyVariables(html, vars, true)
}

func applyVariables(in string, vars map[string]string, escapeHTML bool) (string, bool) {
	rawUsed := false
	out := variablePattern.ReplaceAllStringFunc(in, func(match string) string {
		m := variablePattern.FindStringSubmatch(match)
		key, raw := m[2], false
		if m[1] != "" {
			key, raw = m[1], true
		}
		value, ok := vars[key]
		if !ok {
			return match
		}
		if raw {
			rawUsed = true
			return value
		}
		if escapeHTML {
			return stdhtml.EscapeString(value)
		}
		return value
	})
	return out, rawUsed
}
```

If `strings` is no longer used in `tracking.go` after this change, remove it from the imports.

- [ ] **Step 3: Write the failing command test**

Append to `internal/campaign/app/command/transactional_test.go`:

```go
func TestSendTransactionalEscapesVariablesInHTML(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	tpl, err := domain.NewTemplate("tenant-1", "Order", domain.KindTransactional,
		"Order for {{name}}", `<p>Hi {{name}}</p><table>{{{rows}}}</table>`, "Hi {{name}}")
	require.NoError(t, err)
	tplID, err := templates.Add(context.Background(), "tenant-1", tpl)
	require.NoError(t, err)
	msgr := &txMessenger{}

	h := command.NewSendTransactionalHandler(templates, fakeDomainLookup{verified: true},
		msgr, txLimiter{allow: true}, &fakeTxMessages{}, stubSuppression{}, nil, nil, perTenant())
	_, err = h.Handle(context.Background(), command.SendTransactional{
		TenantID: "tenant-1", TemplateID: tplID, To: "sam@example.com",
		SendingDomainID: "dom-1", FromName: "Acme", FromLocalPart: "noreply",
		Variables: map[string]string{
			"name": `<a href="https://evil.example">Sam</a>`,
			"rows": `<tr><td>1 × Mug</td></tr><script>alert(1)</script>`,
		},
	})
	require.NoError(t, err)
	require.Len(t, msgr.sent, 1)
	msg := msgr.sent[0]
	require.Contains(t, msg.HTMLBody, `Hi &lt;a href=&#34;https://evil.example&#34;&gt;Sam&lt;/a&gt;`)
	require.Contains(t, msg.HTMLBody, `<tr><td>1 × Mug</td></tr>`, "triple braces insert HTML")
	require.NotContains(t, msg.HTMLBody, "<script", "raw insertions are sanitized")
	require.Equal(t, `Order for <a href="https://evil.example">Sam</a>`, msg.Subject, "subjects stay raw")
	require.Equal(t, `Hi <a href="https://evil.example">Sam</a>`, msg.TextBody)
}
```

Run: `go test ./internal/campaign/app/command/ -run TestSendTransactionalEscapesVariablesInHTML -count=1`
Expected: FAIL. `HTMLBody` contains the unescaped `<a href=` and the `<script>`.

- [ ] **Step 4: Implement the handler change**

In `internal/campaign/app/command/transactional.go`, add the import `"github.com/nikolaymatrosov/nvelope/internal/campaign/visualrender"`. Before `ref, err := h.messenger.Send(...)`, insert:

```go
	htmlBody, rawUsed := domain.ApplyHTMLVariables(tpl.BodyHTML(), cmd.Variables)
	if rawUsed {
		// {{{key}}} inserted caller-supplied HTML. Whatever the tenant's
		// raw-HTML level, runtime values go through the standard policy:
		// they usually carry the tenant's end-user input.
		htmlBody, _ = visualrender.SanitizeDocument(htmlBody)
	}
```

Then set `HTMLBody: htmlBody,` in the `OutboundMessage` literal. `Subject` and `TextBody` keep `domain.ApplyVariables(...)`.

- [ ] **Step 5: Run the tests**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./internal/campaign/... -count=1 && go test ./internal/api/ -run 'Tx|Transactional' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/campaign/domain/tracking.go internal/campaign/domain/tracking_test.go internal/campaign/app/command/transactional.go internal/campaign/app/command/transactional_test.go
git commit -m "fix(campaign): escape transactional variables and add a sanitized {{{raw}}} form"
```

---

### Task 9: Campaign-bound click and open attribution

**Files:**
- Modify: `internal/campaign/adapters/tracking_pg.go` (`RecordClick`, `RecordView`)
- Modify: `internal/api/tracking_handlers.go` (`handleTrackClick`)
- Test: `internal/campaign/adapters/tracking_pg_test.go`

**Interfaces:**
- Produces:
  - `RecordClick(ctx, tenantID, linkID, recipientID string) (string, error)` keeps its signature. It now returns the URL even when recording fails, records nothing for a foreign, missing, or malformed recipient, and returns `domain.ErrLinkNotFound` with an empty URL for an unknown link.
  - `RecordView` records nothing for a foreign or malformed recipient.

- [ ] **Step 1: Write the failing tests**

Append to `internal/campaign/adapters/tracking_pg_test.go`, adding the imports `"github.com/jackc/pgx/v5"`, `"github.com/jackc/pgx/v5/pgxpool"`, and `"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"`:

```go
// countRows returns the row count of a tracking table for one key column,
// read inside the tenant binding (the tables are under RLS).
func countRows(t *testing.T, pool *pgxpool.Pool, tenantID, query, arg string) int {
	t.Helper()
	var n int
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, query, arg).Scan(&n)
		}))
	return n
}

func TestTrackingRecordClickAttributesOnlyOwnRecipients(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTracking(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	campaignID, ownRecipient := seedCampaignWithRecipient(t, tenantID)
	_, otherRecipient := seedCampaignWithRecipient(t, tenantID)

	links, err := repo.UpsertLinks(ctx, tenantID, campaignID, []string{"https://acme.com/sale"})
	require.NoError(t, err)
	linkID := links["https://acme.com/sale"]
	const clicks = "SELECT count(*) FROM link_clicks WHERE link_id = $1"

	for _, s := range []string{otherRecipient, "", "not-a-uuid"} {
		url, err := repo.RecordClick(ctx, tenantID, linkID, s)
		require.NoError(t, err, "recipient %q", s)
		require.Equal(t, "https://acme.com/sale", url, "the redirect target is returned regardless")
	}
	require.Zero(t, countRows(t, pool, tenantID, clicks, linkID),
		"no click is attributed to a foreign, missing, or malformed recipient")

	_, err = repo.RecordClick(ctx, tenantID, linkID, ownRecipient)
	require.NoError(t, err)
	require.Equal(t, 1, countRows(t, pool, tenantID, clicks, linkID), "the campaign's own recipient is attributed")
}

func TestTrackingRecordViewIgnoresForeignRecipients(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTracking(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	campaignID, ownRecipient := seedCampaignWithRecipient(t, tenantID)
	_, otherRecipient := seedCampaignWithRecipient(t, tenantID)
	const views = "SELECT count(*) FROM campaign_views WHERE campaign_id = $1"

	require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, otherRecipient))
	require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, "not-a-uuid"))
	require.Zero(t, countRows(t, pool, tenantID, views, campaignID))

	require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, ownRecipient))
	require.Equal(t, 1, countRows(t, pool, tenantID, views, campaignID))
}
```

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./internal/campaign/adapters/ -run 'TestTrackingRecord' -count=1`
Expected: FAIL. The foreign recipient is attributed (count is not zero), and `""` or `not-a-uuid` return an error.

- [ ] **Step 2: Implement the adapter**

In `internal/campaign/adapters/tracking_pg.go`, add the import `"github.com/google/uuid"`. Replace `RecordClick` and `RecordView`:

```go
// RecordClick returns the link's original URL and records the click when the
// recipient belongs to the link's campaign.
//
// A missing, malformed, or foreign recipient id records nothing: the
// redirect must still work for forwarded or hand-copied links. An unknown
// link is domain.ErrLinkNotFound with an empty URL. When the insert itself
// fails, the URL is returned alongside the error so the caller can still
// redirect.
func (r *Tracking) RecordClick(ctx context.Context, tenantID, linkID, recipientID string) (string, error) {
	var url string
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var campaignID string
		err := tx.QueryRow(ctx,
			"SELECT campaign_id, url FROM links WHERE id = $1", linkID).Scan(&campaignID, &url)
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrLinkNotFound
		}
		if err != nil {
			return fmt.Errorf("loading link: %w", err)
		}
		if _, perr := uuid.Parse(recipientID); perr != nil {
			return nil
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO link_clicks (tenant_id, link_id, campaign_id, recipient_id)
			 SELECT @tenant_id::uuid, @link_id::uuid, @campaign_id::uuid, cr.id
			   FROM campaign_recipients cr
			  WHERE cr.id = @recipient_id::uuid AND cr.campaign_id = @campaign_id::uuid`,
			pgx.NamedArgs{
				"tenant_id":    tenantID,
				"link_id":      linkID,
				"campaign_id":  campaignID,
				"recipient_id": recipientID,
			})
		if err != nil {
			return fmt.Errorf("recording click: %w", err)
		}
		return nil
	})
	return url, err
}

// RecordView records an open when the recipient belongs to the campaign. A
// malformed or foreign recipient id records nothing.
func (r *Tracking) RecordView(ctx context.Context, tenantID, campaignID, recipientID string) error {
	if _, err := uuid.Parse(recipientID); err != nil {
		return nil
	}
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO campaign_views (tenant_id, campaign_id, recipient_id)
			 SELECT $1::uuid, $2::uuid, cr.id
			   FROM campaign_recipients cr
			  WHERE cr.id = $3::uuid AND cr.campaign_id = $2::uuid`,
			tenantID, campaignID, recipientID)
		if err != nil {
			return fmt.Errorf("recording view: %w", err)
		}
		return nil
	})
}
```

- [ ] **Step 3: Make the click handler redirect despite attribution errors**

In `internal/api/tracking_handlers.go`, replace the tail of `handleTrackClick`, from `url, err := s.tracking.RecordClick(...)` onward:

```go
	url, err := s.tracking.RecordClick(r.Context(), tenantID, linkID, recipientID)
	if err != nil {
		if url == "" {
			s.fail(w, r, "track click", err)
			return
		}
		// Attribution is best-effort; the recipient still reaches the link.
		s.logger.Warn("recording click", "error", err)
	}
	http.Redirect(w, r, url, http.StatusFound)
```

- [ ] **Step 4: Run the tests**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./internal/campaign/adapters/ -run Tracking -count=1 && go test ./internal/api/ -run 'Track|Send' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/campaign/adapters/tracking_pg.go internal/campaign/adapters/tracking_pg_test.go internal/api/tracking_handlers.go
git commit -m "fix(tracking): attribute clicks and opens only to the campaign's own recipients"
```

---

### Task 10: https-only tenant logo

**Files:**
- Modify: `internal/tenant/domain/branding.go` (`SetLogoURL`)
- Test: `internal/tenant/domain/branding_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tenant/domain/branding_test.go`, adding `"time"` to the imports if absent:

```go
func TestSetLogoURLRequiresHTTPS(t *testing.T) {
	t.Parallel()
	b := domain.HydrateTenantBranding("tenant-1", "", "", "", time.Time{})
	require.Error(t, b.SetLogoURL("http://acme.example/logo.png"), "an http logo is mixed content on https pages")
	require.NoError(t, b.SetLogoURL("https://acme.example/logo.png"))
	require.Equal(t, "https://acme.example/logo.png", b.LogoURL())
}
```

Run: `go test ./internal/tenant/domain/ -run TestSetLogoURLRequiresHTTPS -count=1`
Expected: FAIL, because the `http://` URL is accepted.

- [ ] **Step 2: Implement**

In `SetLogoURL`, replace the prefix check and its error with:

```go
	if !strings.HasPrefix(strings.ToLower(url), "https://") {
		return apperr.NewIncorrectInput("invalid_logo_url",
			"the logo URL must start with https://")
	}
```

Delete the now-unused `lower` variable.

- [ ] **Step 3: Run the tests**

Run: `grep -rn 'logo_url.*http://\|SetLogoURL("http://' internal --include='*_test.go'`. Fix any hit that expects an `http://` logo to be accepted.
Then: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./internal/tenant/... -count=1 && go test ./internal/api/ -run Branding -count=1`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tenant/domain/branding.go internal/tenant/domain/branding_test.go
git commit -m "fix(tenant): require an https logo URL"
```

---

### Task 11: Frontend: sanitization setting and variables note

**Files:**
- Modify: `frontend/src/lib/api-types.ts`, `frontend/src/lib/api.ts`
- Test: `frontend/src/lib/api.test.ts`
- Modify: `frontend/src/routes/t/$slug/settings/index.tsx`
- Test: `frontend/src/routes/t/$slug/settings/index.test.tsx`
- Modify: `frontend/src/routes/t/$slug/transactional/index.tsx`
- Modify: `frontend/src/locales/en/settings.json`, `frontend/src/locales/ru/settings.json`, `frontend/src/locales/en/transactional.json`, `frontend/src/locales/ru/transactional.json`
- Regenerate: `frontend/src/i18n/resources.d.ts`

**Interfaces:**
- Consumes: `GET /settings`, which returns `{"settings": {display_name, timezone, raw_html_sanitization}}`, and `PUT /settings/raw-html-sanitization` (Task 3).
- Produces:
  - `type RawHTMLSanitization = "standard" | "off"`.
  - `WorkspaceSettings.raw_html_sanitization`.
  - `api.getSettings`, which now returns the unwrapped settings.
  - `api.setRawHTMLSanitization(slug, {level, accept_risk})`.

- [ ] **Step 1: Write the failing api test**

Inside `describe("api client — verb & path correctness", ...)` in `frontend/src/lib/api.test.ts`, add:

```ts
  it("unwraps the settings envelope", async () => {
    const settings = { display_name: "Acme", timezone: "UTC", raw_html_sanitization: "standard" }
    mockFetch(200, { settings })
    const res = await api.getSettings("acme")
    expect(res.data).toEqual(settings)
  })

  it("puts the raw-HTML sanitization level", async () => {
    const calls = mockFetch(200, { raw_html_sanitization: "off" })
    await api.setRawHTMLSanitization("acme", { level: "off", accept_risk: true })
    expect(calls[0].url).toBe("/t/acme/api/settings/raw-html-sanitization")
    expect(calls[0].init.method).toBe("PUT")
    expect(JSON.parse(String(calls[0].init.body))).toEqual({ level: "off", accept_risk: true })
  })
```

Run: `export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH"; .claude/scripts/test-frontend.sh src/lib/api.test.ts`
Expected: FAIL. `res.data` is the envelope, and `setRawHTMLSanitization` is not a function.

- [ ] **Step 2: Implement the api layer**

In `frontend/src/lib/api-types.ts`, replace `WorkspaceSettings` with:

```ts
export type RawHTMLSanitization = "standard" | "off"

export type WorkspaceSettings = {
  display_name: string
  timezone: string
  raw_html_sanitization: RawHTMLSanitization
}
```

In `frontend/src/lib/api.ts`, add `RawHTMLSanitization` to the `./api-types` import list and replace `getSettings`:

```ts
  getSettings: async (slug: string): Promise<ApiResult<WorkspaceSettings>> => {
    // Go wraps the settings in a `settings` envelope.
    const res = await request<{ settings: WorkspaceSettings }>("GET", tp(slug, "/settings"))
    return { ...res, data: res.data.settings }
  },
  setRawHTMLSanitization: (
    slug: string,
    body: { level: RawHTMLSanitization; accept_risk: boolean },
  ) =>
    request<{ raw_html_sanitization: RawHTMLSanitization }>(
      "PUT",
      tp(slug, "/settings/raw-html-sanitization"),
      body,
    ),
```

Run the api test again. Expected: PASS.

- [ ] **Step 3: Add the copy**

In `frontend/src/locales/en/settings.json`, add a top-level `"rawHtml"` key, a sibling of `"form"`:

```json
  "rawHtml": {
    "title": "HTML sanitization",
    "description": "Applies to campaigns and templates written in the HTML editor. The visual editor always sanitizes, and archive pages stay sandboxed either way.",
    "statusStandard": "On: scripts, forms, frames, event handlers, and unsafe links are removed on save. Styles and table layouts are kept.",
    "statusOff": "Off: HTML is stored exactly as written.",
    "turnOff": "Disable sanitization…",
    "turnOn": "Re-enable sanitization",
    "savedToast": "Sanitization setting saved.",
    "dialog": {
      "title": "Disable HTML sanitization?",
      "description": "Campaign and template HTML will reach your subscribers without cleanup.",
      "riskActiveContent": "Scripts, forms, iframes, and event handlers will be delivered in your emails.",
      "riskClients": "Mail clients' own filtering becomes the only safeguard.",
      "riskDeliverability": "Mailbox providers may lower deliverability for such mail.",
      "accept": "I understand these risks and accept them for this workspace.",
      "confirm": "Disable anyway"
    }
  },
```

In `frontend/src/locales/ru/settings.json`, add the same key:

```json
  "rawHtml": {
    "title": "Санитизация HTML",
    "description": "Применяется к кампаниям и шаблонам из HTML-редактора. Визуальный редактор санитизирует всегда, а страницы архива в любом случае открываются в песочнице.",
    "statusStandard": "Включена: скрипты, формы, фреймы, обработчики событий и опасные ссылки удаляются при сохранении. Стили и табличная вёрстка сохраняются.",
    "statusOff": "Выключена: HTML сохраняется ровно так, как написан.",
    "turnOff": "Отключить санитизацию…",
    "turnOn": "Снова включить санитизацию",
    "savedToast": "Настройка санитизации сохранена.",
    "dialog": {
      "title": "Отключить санитизацию HTML?",
      "description": "HTML кампаний и шаблонов будет уходить подписчикам без очистки.",
      "riskActiveContent": "Скрипты, формы, iframe и обработчики событий попадут в ваши письма.",
      "riskClients": "Единственной защитой останется фильтрация на стороне почтовых клиентов.",
      "riskDeliverability": "Почтовые провайдеры могут понизить доставляемость таких писем.",
      "accept": "Я понимаю эти риски и принимаю их для этого рабочего пространства.",
      "confirm": "Всё равно отключить"
    }
  },
```

Add these keys inside `"endpoint"` in `frontend/src/locales/en/transactional.json`:

```json
    "variablesNotePrefix": "Template variables: ",
    "variablesNoteEscaped": " is HTML-escaped in the HTML body; ",
    "variablesNoteRaw": " inserts the value as HTML, which is then sanitized. Subjects and text bodies always get the plain value."
```

And inside `"endpoint"` in `frontend/src/locales/ru/transactional.json`:

```json
    "variablesNotePrefix": "Переменные шаблона: ",
    "variablesNoteEscaped": " в HTML-теле экранируется; ",
    "variablesNoteRaw": " вставляет значение как HTML, после чего оно санитизируется. В тему и текстовую часть значение всегда подставляется как есть."
```

Regenerate the types: `export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH"; cd frontend && pnpm i18n:types && cd ..`

- [ ] **Step 4: Write the failing settings-page test**

In `frontend/src/routes/t/$slug/settings/index.test.tsx`:

- Extend the api mock to `api: { getSettings: vi.fn(), updateSettings: vi.fn(), setRawHTMLSanitization: vi.fn() }`.
- Add `raw_html_sanitization: "standard"` to the existing `getSettings` mock value.
- Append this test inside `describe("SettingsView", ...)`:

```tsx
  it("disables raw HTML sanitization only after the risk is accepted", async () => {
    vi.mocked(api.getSettings).mockResolvedValue(
      ok({ display_name: "Acme", timezone: "UTC", raw_html_sanitization: "standard" }),
    )
    vi.mocked(api.setRawHTMLSanitization).mockResolvedValue(ok({ raw_html_sanitization: "off" }))
    renderWithClient(<SettingsView />)

    fireEvent.click(await screen.findByRole("button", { name: /disable sanitization/i }))
    const confirm = await screen.findByRole("button", { name: /disable anyway/i })
    expect(confirm).toBeDisabled()

    fireEvent.click(screen.getByRole("checkbox"))
    expect(confirm).toBeEnabled()
    fireEvent.click(confirm)

    await waitFor(() =>
      expect(api.setRawHTMLSanitization).toHaveBeenCalledWith("acme", {
        level: "off",
        accept_risk: true,
      }),
    )
  })
```

Run: `export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH"; .claude/scripts/test-frontend.sh 'src/routes/t/$slug/settings'`
Expected: FAIL, because no "Disable sanitization" button exists.

- [ ] **Step 5: Implement the settings card**

In `frontend/src/routes/t/$slug/settings/index.tsx`:

- Add these imports:
  - `useState` from `react`
  - `RawHTMLSanitization` alongside `WorkspaceSettings`
  - `Checkbox` from `@/components/ui/checkbox`
  - `AlertDialog`, `AlertDialogAction`, `AlertDialogCancel`, `AlertDialogContent`, `AlertDialogDescription`, `AlertDialogFooter`, `AlertDialogHeader`, `AlertDialogTitle` from `@/components/ui/alert-dialog`
- In `SettingsForm`, change `mutationFn: (v: WorkspaceSettings) =>` to `mutationFn: (v: Pick<WorkspaceSettings, "display_name" | "timezone">) =>`.
- Replace the `AsyncState` children with:

```tsx
        {(settings) => (
          <>
            <SettingsForm slug={slug} settings={settings} />
            <RawHTMLSanitizationCard slug={slug} level={settings.raw_html_sanitization} />
          </>
        )}
```

Append the component:

```tsx
function RawHTMLSanitizationCard({
  slug,
  level,
}: {
  slug: string
  level: RawHTMLSanitization
}) {
  const queryClient = useQueryClient()
  const { t } = useTranslation(["settings", "common"])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [accepted, setAccepted] = useState(false)

  const change = useMutation({
    mutationFn: (next: RawHTMLSanitization) =>
      api.setRawHTMLSanitization(slug, { level: next, accept_risk: next === "off" }),
    onSuccess: async () => {
      setDialogOpen(false)
      setAccepted(false)
      await queryClient.invalidateQueries({ queryKey: queryKeys.settings(slug) })
      toast.success(t("rawHtml.savedToast"))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const off = level === "off"
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("rawHtml.title")}</CardTitle>
        <CardDescription>{t("rawHtml.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col items-start gap-3">
        <p className="text-sm">{off ? t("rawHtml.statusOff") : t("rawHtml.statusStandard")}</p>
        {off ? (
          <Button
            variant="outline"
            disabled={change.isPending}
            onClick={() => change.mutate("standard")}
          >
            {t("rawHtml.turnOn")}
          </Button>
        ) : (
          <Button variant="destructive" onClick={() => setDialogOpen(true)}>
            {t("rawHtml.turnOff")}
          </Button>
        )}
      </CardContent>
      <AlertDialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) setAccepted(false)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("rawHtml.dialog.title")}</AlertDialogTitle>
            <AlertDialogDescription>{t("rawHtml.dialog.description")}</AlertDialogDescription>
          </AlertDialogHeader>
          <ul className="list-disc pl-5 text-sm text-muted-foreground">
            <li>{t("rawHtml.dialog.riskActiveContent")}</li>
            <li>{t("rawHtml.dialog.riskClients")}</li>
            <li>{t("rawHtml.dialog.riskDeliverability")}</li>
          </ul>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={accepted} onCheckedChange={(v) => setAccepted(v === true)} />
            {t("rawHtml.dialog.accept")}
          </label>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={change.isPending}>
              {t("common:actions.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={!accepted || change.isPending}
              onClick={(e) => {
                e.preventDefault()
                change.mutate("off")
              }}
            >
              {t("rawHtml.dialog.confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  )
}
```

- [ ] **Step 6: Add the transactional variables note**

In `frontend/src/routes/t/$slug/transactional/index.tsx`, inside the endpoint `CardContent`, right after the `<p>` that ends with `t("endpoint.templateNoteSuffix")`, add:

```tsx
          <p className="text-muted-foreground">
            {t("endpoint.variablesNotePrefix")}
            <code>{"{{key}}"}</code>
            {t("endpoint.variablesNoteEscaped")}
            <code>{"{{{key}}}"}</code>
            {t("endpoint.variablesNoteRaw")}
          </p>
```

The braces live in JSX, not in the i18n strings, because i18next would treat `{{key}}` in a catalog string as interpolation.

- [ ] **Step 7: Run the frontend checks**

Run:
```bash
export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH"
.claude/scripts/test-frontend.sh 'src/routes/t/$slug/settings' && .claude/scripts/test-frontend.sh src/lib/api.test.ts && .claude/scripts/test-frontend.sh src/i18n
cd frontend && pnpm typecheck && pnpm lint && pnpm i18n:lint && cd ..
```
Expected: all PASS. The i18n catalog-parity test confirms that the en and ru keys match.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/lib/api.ts frontend/src/lib/api-types.ts frontend/src/lib/api.test.ts 'frontend/src/routes/t/$slug/settings' 'frontend/src/routes/t/$slug/transactional/index.tsx' frontend/src/locales frontend/src/i18n/resources.d.ts
git commit -m "feat(frontend): let owners disable raw-HTML sanitization after accepting the risk"
```

---

### Task 12: Full verification

- [ ] **Step 1: Run the whole Go suite and the architecture lint**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; export PATH="$HOME/go/bin:$PATH"; make test && make lint && make lint-arch`
Expected: PASS. A `go-cleanarch` complaint about `campaign/app/command` importing `visualrender` would be new only if the import had not existed before. `save_visual_campaign.go` already has it, so none is expected.

- [ ] **Step 2: Run the whole frontend suite**

Run: `export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH"; .claude/scripts/test-frontend.sh`
Expected: PASS.

- [ ] **Step 3: Mark the spec implemented**

In `docs/superpowers/specs/2026-11-14-content-security-design.md`, change `Status: Draft (pending review)` to `Status: Implemented`. Then:

```bash
git add docs/superpowers/specs/2026-11-14-content-security-design.md
git commit -m "docs: mark the content security design implemented"
```

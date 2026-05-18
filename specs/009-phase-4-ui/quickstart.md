# Quickstart: Phase 4 — Deliverability & Analytics — Frontend UI

## Prerequisites

- Go toolchain + Docker (backend, integration tests via testcontainers).
- `pnpm` (frontend lives in `frontend/`, `pnpm@10.25.0`).
- The Phase 4 backend running so the UI has a live API. Some campaign
  analytics figures only appear after the analytics-refresh job has run and
  after the consumer has ingested feedback.

## Run the stack

```bash
# Backend API
make run            # or: go run ./cmd/...

# Frontend dev server (proxies /api and /t/{slug}/api to the Go API)
cd frontend && pnpm install && pnpm dev   # http://localhost:3000
```

## Verify the feature (maps to spec Success Criteria)

1. **Campaign analytics (SC-001, SC-002, SC-008)** — sign in, open a sent
   campaign, follow the analytics link, confirm sent/delivered/opened/
   clicked/bounced/complained counts and the four rates render, and that the
   "last refreshed" time is shown. Open a just-sent campaign and confirm the
   "awaiting data" state instead of an error.
2. **Suppression list (SC-003, SC-004, SC-005)** — open **Suppression list**,
   confirm bounce/complaint entries show reason + date, filter by reason,
   search by address, add an address manually (invalid input shows an inline
   error), and remove an entry — confirming the removal prompt appears first.
3. **Dashboard (SC-006)** — open **Dashboard**, confirm aggregate counts, the
   overall bounce/complaint rates, and the recent-campaign list render; click a
   campaign row and confirm its analytics view opens. With no sending activity,
   confirm the empty state.
4. **Bounce settings (SC-009)** — from the suppression list open the
   bounce-action settings, confirm both toggles default to on, turn one off,
   save, reload, and confirm the change persisted.
5. **Tenant scope & permissions (SC-007)** — sign in as a user lacking
   `sending:*` / `campaigns:*` and confirm the Dashboard and Suppression list
   nav entries and the analytics link are hidden/disabled.

## Tests

```bash
cd frontend && pnpm test         # vitest — route + component tests
cd frontend && pnpm typecheck && pnpm lint
make test                        # Go suite — unchanged, must stay green
```

New frontend tests: one `*.test.tsx` per new route (dashboard, suppression
list, bounce settings, campaign analytics), plus tests for the `MetricTile` and
`RateValue` shared components. No new backend tests — this feature touches no
Go code.

## Definition of done

- Both new nav areas (Dashboard, Suppression list) reachable, permission-gated,
  and styled with the shared design system (no unstyled screens).
- Campaign analytics reachable from the campaign detail page; awaiting-data and
  not-found states handled.
- Bounce-action settings reachable from the suppression list; toggles persist.
- `pnpm test`, `pnpm typecheck`, `pnpm lint` all green; `make test` still green.
- No backend code, schema, or migration change.

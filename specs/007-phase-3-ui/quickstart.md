# Quickstart: Phase 3 Sending Pipeline — Frontend UI

## Prerequisites

- Go toolchain + Docker (backend, integration tests via testcontainers).
- `pnpm` (frontend lives in `frontend/`, `pnpm@10.25.0`).
- The Phase 3 backend running so the UI has a live API.

## Run the stack

```bash
# Backend API
make run            # or: go run ./cmd/...

# Frontend dev server (proxies /api and /t/{slug}/api to the Go API)
cd frontend && pnpm install && pnpm dev   # http://localhost:3000
```

## Verify the feature (maps to spec Success Criteria)

1. **Sending domain (SC-001)** — sign in, open **Sending Domains**, add a
   domain, copy each DKIM/SPF/DMARC record, publish them on a test domain,
   click re-check, confirm it reaches `verified` without a reload.
2. **Campaign (SC-002, SC-003, SC-004)** — open **Campaigns**, create a
   campaign (optionally from a template), set content, pick the verified
   domain, target a list, confirm start is blocked without a verified domain,
   then start it and watch sent/failed/remaining advance without reloading.
3. **Templates** — open **Templates**, create a campaign template and a
   transactional template, edit one, delete one (confirm prompt appears).
4. **Transactional (SC-005)** — open **Transactional Sending**, issue an API
   key with the `transactional:send` scope, confirm the secret shows once,
   revoke it, confirm the endpoint/payload reference is displayed.
5. **Permissions (SC-008)** — sign in as a user lacking `sending:*` /
   `campaigns:*` and confirm the nav entries and actions are hidden/disabled.

## Tests

```bash
make test                       # Go suite incl. new cancel/delete handler tests
cd frontend && pnpm test         # vitest — route + component tests
cd frontend && pnpm typecheck && pnpm lint
```

New backend tests: `handleCancelCampaign` and `handleDeleteTemplate` in
`internal/api/campaign_handlers_test.go`, plus command tests for
`CancelCampaign` / `DeleteTemplate`. New frontend tests: one `*.test.tsx`
per new route, colocated.

## Definition of done

- All four nav areas reachable, permission-gated, and styled with the shared
  design system (no unstyled screens).
- The two new backend endpoints implemented, routed, and tested.
- `make test`, `pnpm test`, `pnpm typecheck`, `pnpm lint` all green.
- Clean migration apply (no schema changes expected in this feature).

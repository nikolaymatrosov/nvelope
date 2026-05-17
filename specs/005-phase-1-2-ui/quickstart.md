# Quickstart: Phases 1 & 2 — Frontend UI

How to run, develop, and verify the frontend feature.

## Prerequisites

- Node + `pnpm` 10.25 (the frontend's `packageManager`).
- Docker running (the Go backend's integration tests use testcontainers).
- The Go API service reachable at `http://localhost:8080`.

## Run the stack

```bash
# 1. Backend API (repo root) — serves /api/platform/* and /t/{slug}/api/*
make run            # or: go run ./cmd/api

# 2. Frontend dev server (frontend/)
cd frontend
pnpm install
pnpm dev            # http://localhost:3000
```

The Vite dev server proxies `/api` and `^/t/[^/]+/api` to
`http://localhost:8080`, so the browser talks to the API same-origin and the
session cookie works without CORS.

## One-time setup for this feature

```bash
cd frontend
pnpm add @tanstack/react-query          # research.md Decision 1
# Generate the shadcn primitives (research.md Decision 6) via the shadcn
# MCP/CLI against the existing components.json:
#   sidebar input label card dialog alert-dialog table badge dropdown-menu
#   select tabs sonner skeleton separator avatar tooltip textarea checkbox
```

## Verification bundle

Run before claiming any slice complete (Constitution Principle II):

```bash
cd frontend
pnpm typecheck      # tsc --noEmit
pnpm lint           # eslint
pnpm test           # vitest run
```

## Manual smoke test per user story

Each user story is independently demonstrable (Constitution Principle III).

**US1 — Onboard & open a workspace (P1)**
1. Visit `/signup`, register email + password + name → signed in.
2. Re-try signup with the same email → clear duplicate-email error, no 2nd account.
3. Create a workspace with a name + unique slug → land inside the sidebar shell.
4. Sign out, sign back in at `/login`, pick the workspace from the home list.
5. Open a slug you are not a member of → "not found / no access" screen.

**US2 — Lists & subscribers (P1)**
1. Create a list (name + description) → appears in Lists.
2. Create subscribers with email, name, custom attributes → appear in Subscribers.
3. Re-create with a duplicate email → refused with a clear message.
4. Edit a subscriber; add/remove from a list; change subscription state.
5. Run an email/name search and an attribute segment query → matches + count.
6. Delete a subscriber and a list after confirming.

**US3 — Invite teammates & manage roles (P2)**
1. Invite an email → pending invitation shows; revoke one.
2. Accept an invite via the invite link as the invitee.
3. Create a role with a limited permission set; assign it to the member.
4. Confirm the member's disallowed actions are hidden/disabled; a denied attempt
   shows an authorization message.
5. Grant a per-list role → access widens for that one list only.

**US4 — Import & export (P2)**
1. Upload a CSV (mix of new + existing emails), pick a target list → job starts.
2. Watch progress → result summary with created/updated/failed counts.
3. Start an export of a list → download the CSV when the job completes.
4. As a user without import/export permission → controls unavailable + explained.

**US5 — Account & workspace security (P3)**
1. Enrol in TOTP (scan QR, confirm a code); sign out/in → TOTP challenge appears.
2. Issue an API key → secret shown once with a non-retrievable warning; revoke it.
3. Open the audit trail → recent actions with actor/action/time.
4. Update workspace settings → save confirmed on next view.

## Cross-cutting checks (apply to every screen)

- Every async view shows distinct **loading / empty / error / populated** states
  (FR-034, SC-006) — never a blank screen.
- Every destructive action requires explicit **confirmation** (FR-031, SC-007).
- Actions the user lacks permission for are **hidden or disabled** with an
  explanation (FR-009, SC-008).
- A **session expiry** mid-task routes to sign-in with a clear message
  (FR-006, SC-010).
- Every Phase 1 + Phase 2 screen uses the **shared design system** — no screen
  keeps the old minimal styling (FR-033, SC-009).

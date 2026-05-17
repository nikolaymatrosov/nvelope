# Implementation Plan: Phases 1 & 2 — Frontend UI

**Branch**: `005-phase-1-2-ui` | **Date**: 2026-05-17 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-phase-1-2-ui/spec.md`

## Summary

Build one cohesive web interface covering Phase 1 (platform onboarding, workspaces,
invitations) and Phase 2 (lists & subscribers, RBAC, import/export, account
security). The Phase 1 backend (platform API) and Phase 2 backend (tenant-scoped
API) are already implemented and stable; this feature delivers only the frontend.
The existing minimally styled Phase 1 screens are redesigned into a shared design
system, and a persistent sidebar app shell hosts every authenticated workspace
view.

Technical approach: extend the existing TanStack Start application
(`frontend/`). Add TanStack Query for cache-backed loading/empty/error/populated
states, generate the shadcn component set the design system needs, model the
backend as a typed API client layer, and build file-routed screens under a
`/t/$slug` sidebar layout. Permission-aware UI gating is derived from the
workspace member's role joined to the role catalogue, with reactive handling of
backend `403`/`401` responses as the authoritative backstop.

## Technical Context

**Language/Version**: TypeScript 5.9, React 19.2

**Primary Dependencies**: TanStack Start 1.166, TanStack Router 1.167, Vite 7,
Tailwind CSS v4, shadcn (radix-nova style, `mist` base color), Radix UI,
lucide-react; **add** `@tanstack/react-query` (its router-SSR integration
`@tanstack/react-router-ssr-query` is already a dependency)

**Storage**: None in the frontend — all state lives behind the Go API
(platform API at `/api/platform/*`, tenant-scoped API at `/t/{slug}/api/*`).
Session state is held in HTTP-only cookies set by the backend.

**Testing**: Vitest 3 + Testing Library (jsdom) for component and behavior
tests; `pnpm typecheck` (tsc) and `pnpm lint` (eslint) as static gates.

**Target Platform**: Modern desktop browsers (Chromium, Firefox, Safari).
Responsive/mobile layout is a nice-to-have, not a requirement this iteration.

**Project Type**: Web frontend (single-page app served by TanStack Start;
backend already exists).

**Performance Goals**: Interactive within typical SPA budgets; list and
subscriber views stay usable at large result sizes via server-side
limit/offset paging (FR-016). No hard latency target — the SC metrics are
task-completion-time, not request-latency.

**Constraints**: No new UI framework (FR/Assumptions). Reuse the existing
TanStack Start + Tailwind + shadcn stack. The backend is the source of truth
for authorization; UI gating is advisory only and every action is re-checked
server-side. The session cookie mechanism is used as-is; the UI never handles
tokens directly.

**Scale/Scope**: 5 prioritized user stories, 34 functional requirements, ~13
key entities. Roughly 25–30 routed screens across onboarding, the workspace
shell, lists, subscribers, people & access, import/export, and security/audit/
settings.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `.specify/memory/constitution.md` v1.1.0.

| Principle | Status | Notes |
| --- | --- | --- |
| I. Tenant Isolation by Default | PASS | The frontend never queries data directly. Every tenant-scoped call goes through a `/t/{slug}/api/*` path; the backend enforces isolation. The API client makes the slug a required argument of every tenant call so a screen cannot accidentally omit it. |
| II. Test-Backed Delivery | PASS | Each user story ships with Vitest component/behavior tests. Critical UI paths — auth/session routing, permission gating, destructive-action confirmation, async-state rendering — get explicit coverage. `pnpm typecheck`, `pnpm lint`, `pnpm test` form the frontend verification bundle. |
| III. Incremental, Shippable Phases | PASS | The 5 user stories are independently shippable slices (P1: onboarding shell, P1: lists & subscribers; P2: RBAC, import/export; P3: security/audit/settings). No speculative scope. |
| IV. Security & Consent by Design | PASS | Auth, the TOTP challenge, permission gating, and one-time API-key display are designed into the relevant stories from the start, not retrofitted. UI gating mirrors the backend; the backend stays authoritative. |
| V. Operable & Observable Services | PASS (N/A scope) | The frontend is stateless; it holds no server/session state of its own. No async job processing lives in the UI — jobs run server-side and the UI polls status. |
| VI. Layered Architecture & Domain Integrity | PASS | Dependency rule applied to the frontend: a single typed API-client layer (`src/lib/api.ts`) owns all transport; route components and hooks depend inward on it and never call `fetch` directly. Error mapping happens once (the client normalizes the backend's typed `{error, message}` envelope); screens branch on a normalized error kind, never on status-code arithmetic or error strings. |

**Gate result: PASS — no violations, Complexity Tracking not required.**

Re-check after Phase 1 design: **PASS** — the data model and contracts introduce
no direct data access, no transport leakage into components, and no new
framework. Design holds the gate.

## Project Structure

### Documentation (this feature)

```text
specs/005-phase-1-2-ui/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── api-client.md    # Phase 1 output — typed client surface vs. backend
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
frontend/
├── src/
│   ├── lib/
│   │   ├── api.ts            # typed API client — the only transport layer
│   │   ├── api-types.ts      # request/response/view types (NEW)
│   │   ├── query.ts          # TanStack Query client + query/mutation keys (NEW)
│   │   ├── permissions.ts    # derive effective permissions; gating helpers (NEW)
│   │   ├── errors.ts         # normalized ApiError kind + 401/403 routing (NEW)
│   │   └── utils.ts          # cn() etc. (exists)
│   ├── components/
│   │   ├── ui/               # shadcn primitives (button exists; add the rest)
│   │   ├── shell/            # sidebar app shell, top bar, nav (NEW)
│   │   └── common/           # async-state, confirm-dialog, JSON editor,
│   │                         # segment builder, data-table (NEW)
│   ├── hooks/                # useWorkspace, usePermissions, useSession (NEW)
│   ├── routes/
│   │   ├── __root.tsx        # exists — wrap with QueryClientProvider
│   │   ├── index.tsx         # workspace picker (redesigned)
│   │   ├── login.tsx         # redesigned
│   │   ├── signup.tsx        # redesigned
│   │   ├── tenants.new.tsx   # redesigned
│   │   ├── invite.$token.tsx # redesigned
│   │   └── t.$slug/          # workspace shell layout + nested screens (NEW)
│   │       ├── route.tsx     # sidebar layout, session guard, TOTP challenge
│   │       ├── index.tsx     # workspace overview / redirect
│   │       ├── subscribers.*.tsx
│   │       ├── lists.*.tsx
│   │       ├── access.*.tsx  # members, invitations, roles
│   │       ├── import-export.*.tsx
│   │       ├── audit.tsx
│   │       └── settings.tsx
│   ├── routeTree.gen.ts      # generated
│   └── styles.css            # design tokens (exists)
└── (vite/vitest/eslint configs — exist)
```

**Structure Decision**: Extend the existing `frontend/` TanStack Start project
in place. No backend directories change. The dependency rule of Principle VI is
honored by keeping `src/lib/` (transport + types + error mapping) as the inward
core that `routes/`, `components/`, and `hooks/` depend on — components never
import `fetch` or branch on raw HTTP status. shadcn primitives stay isolated in
`components/ui/`; feature composition lives in `components/shell|common/` and
`routes/`.

## Complexity Tracking

No constitution violations — this section is intentionally empty.

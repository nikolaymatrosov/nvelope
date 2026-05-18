# Implementation Plan: Phase 3 Sending Pipeline — Frontend UI

**Branch**: `007-phase-3-ui` | **Date**: 2026-05-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/007-phase-3-ui/spec.md`

## Summary

Deliver the tenant-workspace web interface for the already-built Phase 3
sending pipeline: sending-domain verification, template authoring, campaign
authoring/sending/monitoring, and transactional API-key setup. The UI extends
the existing Phase 1 & 2 React/TanStack app shell with four new permission-gated
nav areas and reuses the shared design system, typed API client, and async-state
conventions. A small backend slice is included as a prerequisite: two missing
endpoints — cancel campaign and delete template — are added so FR-009, FR-019,
and FR-025 can be satisfied (the campaign domain already has a validated
`Cancel()` method; only the command + route are missing).

## Technical Context

**Language/Version**: TypeScript 5.9 / React 19 (frontend); Go 1.x (backend slice)

**Primary Dependencies**: TanStack Start/Router/Query/Form/Table, shadcn +
Radix UI, Tailwind v4, lucide-react, sonner (frontend); chi router, existing
campaign/sending CQRS application packages (backend)

**Storage**: PostgreSQL via the existing Phase 3 schema — no new tables or
migrations in this feature

**Testing**: vitest + @testing-library/react (frontend); `go test` with
testcontainers PostgreSQL (backend)

**Target Platform**: Modern desktop browsers; Go API service

**Project Type**: Web application — existing `frontend/` SPA + `internal/` Go
backend

**Performance Goals**: Interactive UI; live views poll (~5 s domains, ~3 s
running campaigns) — no specific throughput target

**Constraints**: No new frontend framework; extend not rebuild the app shell;
plain HTML/text content editing (no WYSIWYG); interval re-fetch for live
updates (no real-time transport)

**Scale/Scope**: 4 new nav areas, ~8 new file-routes, ~2 new hooks, ~1 shared
component (DNS-record copy row); 2 new backend endpoints + commands

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.1.0.*

- **I. Tenant Isolation by Default** — PASS. No data-layer changes. Every API
  call goes through the tenant-scoped client (`tp(slug, …)`); `slug` is a
  required first argument so a call site cannot omit tenant scope. The two new
  backend endpoints derive the tenant from request context exactly as the
  existing handlers do.
- **II. Test-Backed Delivery** — PASS. Each new frontend route ships a
  colocated `*.test.tsx`; the two new backend endpoints ship handler tests and
  command tests. Backend command tests exercise the real campaign domain.
- **III. Incremental, Shippable Phases** — PASS. The four user stories are
  independently shippable in priority order (P1 domains, P1 campaigns, P2
  templates, P3 transactional). No speculative scope; the backend slice is the
  minimum needed to honour the spec.
- **IV. Security & Consent by Design** — PASS. Nav/action gating uses the
  Phase 3 permission strings; the backend re-checks every request and remains
  authoritative. The API-key secret is shown once; `401` routes to sign-in.
- **V. Operable & Observable Services** — PASS. Frontend is stateless. The new
  backend handlers reuse the existing CQRS command path, which already carries
  logging/metrics decorators.
- **VI. Layered Architecture & Domain Integrity** — PASS. The new backend
  commands (`CancelCampaign`, `DeleteTemplate`) follow the established
  command-handler pattern; the domain method `Campaign.Cancel()` already
  validates the transition. Typed-error → transport mapping reuses `s.fail`;
  no new transport-level error branching. Frontend keeps transport isolated in
  `lib/api.ts`.

**Result**: PASS — no violations, Complexity Tracking not required.

*Post-design re-check*: the design adds no new layers, no DI framework, no
schema change, and no duplicated infrastructure. Still PASS.

## Project Structure

### Documentation (this feature)

```text
specs/007-phase-3-ui/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── tenant-api.md    # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
frontend/src/
├── lib/
│   ├── api.ts                       # + sending/template/campaign methods, cancel + delete
│   ├── api-types.ts                 # + DomainView, TemplateView, CampaignView; extend Permission
│   └── query.ts                     # + query keys for domains/templates/campaigns
├── hooks/
│   ├── use-sending-domains.ts       # NEW — poll while any domain pending
│   └── use-campaign.ts              # NEW — poll while campaign running
├── components/
│   └── common/
│       └── dns-record-row.tsx       # NEW — copyable DNS record (DKIM/SPF/DMARC)
├── components/shell/
│   └── sidebar.tsx                  # + 4 nav entries, permission-gated
└── routes/t/$slug/
    ├── sending-domains/
    │   ├── index.tsx                # list + add + recheck (US1)
    │   └── $id.tsx                  # detail + DNS records + status
    ├── templates/
    │   ├── index.tsx                # list + create + delete (US3)
    │   └── $id.tsx                  # edit (US3)
    ├── campaigns/
    │   ├── index.tsx                # list + create (US2)
    │   └── $id.tsx                  # editor + start/pause/resume/cancel + progress (US2)
    └── transactional/
        └── index.tsx                # API-key panel + endpoint reference (US4)

internal/
├── campaign/app/command/
│   ├── campaigns.go                 # + CancelCampaign command & handler
│   └── templates.go                 # + DeleteTemplate command & handler
├── campaign/app/app.go (or equiv.)  # + wire the two new commands into Application
└── api/
    ├── campaign_handlers.go         # + handleCancelCampaign, handleDeleteTemplate
    ├── campaign_handlers_test.go    # + tests for the two new handlers
    └── server.go                    # + 2 routes
```

**Structure Decision**: Existing web-application layout. The frontend SPA in
`frontend/` is extended with file-routes under the established
`routes/t/$slug/` tree; the Go backend in `internal/` gains a thin slice in the
existing `campaign` bounded context. No new top-level directories.

## Phase 0 — Research

Complete. See [research.md](./research.md). Seven decisions, all resolved from
in-repo inspection; no `NEEDS CLARIFICATION` remain. The one genuine ambiguity
(missing cancel/delete endpoints) was resolved with the user: add the backend
endpoints.

## Phase 1 — Design & Contracts

Complete:
- [data-model.md](./data-model.md) — view shapes for sending domain, template,
  campaign, and derived send-progress; no new persisted entities.
- [contracts/tenant-api.md](./contracts/tenant-api.md) — the full tenant-API
  surface the UI consumes, including the two new backend endpoints and their
  implementation notes.
- [quickstart.md](./quickstart.md) — run, verify, and test instructions.
- Agent context (`CLAUDE.md`) updated to point at this plan.

## Phase 2 — Next step

Run `/speckit-tasks` to generate `tasks.md`. Suggested task ordering:

1. **Backend slice** — `DeleteTemplate` + `CancelCampaign` commands, handlers,
   routes, and tests (unblocks FR-009/FR-019/FR-025).
2. **Shared frontend plumbing** — extend `Permission`/`ALL_PERMISSIONS`, add
   API client methods, query keys, the `DnsRecordRow` component, and the four
   sidebar nav entries.
3. **US1 Sending Domains** (P1) — list/add/recheck route + detail + polling
   hook.
4. **US2 Campaigns** (P1) — list/create + editor with start/pause/resume/cancel
   + live progress hook.
5. **US3 Templates** (P2) — list/create/delete + edit route.
6. **US4 Transactional** (P3) — API-key panel + endpoint reference.

Each user story is independently demonstrable and testable per its spec
Independent Test.

## Complexity Tracking

No constitution violations — section intentionally empty.

---
name: go-ddd-architecture
description: "Build maintainable Go business applications using DDD-Lite, Clean Architecture, Repository pattern, CQRS, and a layered testing strategy. Use this skill when writing or refactoring Go services with non-trivial business logic — anything beyond simple CRUD where code needs to stay maintainable as it grows. Triggers include: writing a Go service or microservice, designing Go project structure, implementing Go domain models, writing Go repository code, organizing Go test suites, refactoring Go code that mixes business logic with HTTP/DB code, or applying DDD / Clean Architecture / CQRS in Go. Distilled from 'Go with the Domain' by Three Dots Labs (Miłosz Smółka, Robert Laszczak)."
---

# Building Maintainable Go Business Applications

This skill packages the practical patterns from *Go with the Domain* into something you can apply while writing Go code. The patterns work together — they are not independent options to mix and match arbitrarily. The whole point is **constant development velocity as the codebase ages**.

## When to use these patterns (and when not to)

This is the most important section in the skill. Applying these patterns to the wrong project is over-engineering.

**Use them when:**
- The service has real business logic (rules, workflows, invariants) — not just CRUD over a database
- The service will be maintained for more than ~6 months
- More than one or two engineers will work on it
- The team values being able to refactor without fear

**Do NOT use them when:**
- The service is a thin CRUD layer that mostly maps HTTP/gRPC requests to DB rows
- The service is an authentication, proxy, or pure-orchestration service with no rules
- It is a prototype or throwaway tool

In *Go with the Domain*, the `users` service is intentionally NOT refactored to Clean Architecture / DDD / CQRS — because it does not need to be. Apply judgment. Putting everything in one `main` package is a perfectly fine design for many services.

When in doubt, start simple and refactor when you actually feel the pain.

## The stack at a glance

Four ideas, composed:

1. **DDD-Lite (tactical)** — Domain types own their state. Fields are private. Methods describe **behaviors**, not data mutations. Construction validates invariants. The domain knows nothing about HTTP, gRPC, or the database.
2. **Repository pattern** — The domain declares a `Repository` interface for persistence. Implementations live in `adapters/`. Updates go through a closure: `Update(ctx, id, func(entity) (entity, error))`. The closure is the transaction boundary.
3. **Clean Architecture (4 layers)** — `ports` (entry points: HTTP, gRPC) → `app` (orchestration: commands + queries) → `domain` (business rules) ← `adapters` (exits: DB, external clients). Outer layers depend on inner layers, never the reverse. Inner layers declare interfaces; outer layers implement them.
4. **CQRS** — Split the `app` layer into commands (state changes, return error only) and queries (reads, return data). Each command and each query is a separate handler type.

## Standard project layout

For a service called `trainings`:

```
internal/trainings/
├── main.go                          # wires dependencies, starts server
├── service/
│   └── service.go                   # NewApplication() — constructs app + adapters
├── domain/
│   └── training/                    # one package per aggregate
│       ├── training.go              # entity with private fields
│       ├── repository.go            # Repository interface
│       ├── user.go                  # value objects (User, UserType)
│       ├── errors.go
│       └── *_test.go                # pure unit tests, no infra
├── app/
│   ├── app.go                       # Application struct: Commands + Queries
│   ├── command/
│   │   ├── schedule_training.go     # one file per command
│   │   ├── cancel_training.go
│   │   └── *_test.go                # unit tests with simple mocks
│   └── query/
│       ├── available_hours.go
│       └── training_details.go
├── adapters/
│   ├── trainings_firestore_repo.go  # implements training.Repository
│   ├── trainings_mysql_repo.go      # alternative implementation
│   ├── trainer_grpc.go              # implements app's trainerService interface
│   └── *_test.go                    # integration tests against Docker DB
├── ports/
│   ├── http.go                      # implements OpenAPI-generated interface
│   ├── grpc.go
│   └── openapi_*.gen.go             # generated, do not edit
└── tests/                           # component + e2e tests
    └── component_test.go
```

**The import direction rule** — outer layers can import inner layers, never the reverse:

```
ports     →  app  →  domain
adapters  →  app  →  domain
                      ↑
                domain has no imports from any of the others
```

If you get a Go import cycle, an inner layer is reaching into an outer layer — that's the bug. Fix it by declaring an interface in the inner layer and implementing it in the outer one.

You can enforce this with the [go-cleanarch](https://github.com/roblaszczak/go-cleanarch) linter in CI.

## How a request flows through

Following a `POST /trainings/{uuid}/cancel` request:

1. **HTTP port** (`ports/http.go`) — decodes JSON, extracts auth user, builds a `command.CancelTraining` struct, calls `h.app.Commands.CancelTraining.Handle(ctx, cmd)`. Returns slug-based errors to map to HTTP codes.
2. **Command handler** (`app/command/cancel_training.go`) — calls `repo.UpdateTraining(ctx, uuid, user, func(tr *training.Training) (*training.Training, error) { ... })`. Inside the closure: invoke domain methods, call external services, return the updated entity.
3. **Domain method** (`domain/training/cancel.go`) — `tr.Cancel()` checks invariants (`if t.IsCanceled() { return ErrTrainingAlreadyCanceled }`) and mutates private state. No I/O.
4. **Repository adapter** (`adapters/trainings_firestore_repo.go`) — opens a transaction, fetches the doc, unmarshals into `*training.Training`, runs the closure, marshals back, commits.

No layer skips another. No layer knows what is "above" it.

## Decision tree for everyday Go tasks

**"I need a new endpoint that does X"** — Add a port handler (or method to an existing one). Build the command/query struct. Add a handler in `app/command/` or `app/query/`. The handler calls into `domain/` or directly into a read model. See `references/application.md` and `references/ports-adapters.md`.

**"I need a new business rule"** — Goes in `domain/`. Add a method to the relevant entity, or a free-standing function in the entity's package. NEVER add business `if` statements in `app/` handlers — that's a smell. See `references/domain.md`.

**"I need to query data in a new shape"** — Add a `Query` type and a `Handler` in `app/query/`. Define a read-model interface next to the handler. Implement it in `adapters/`. The query response type is NOT the domain type — it is optimized for the UI. See `references/application.md`.

**"I need to swap or add a database"** — Implement the existing `Repository` interface from `domain/`. The domain code does not change. See `references/repository.md`.

**"I'm adding tests"** — Decide which layer: pure logic → unit (no infra). Repository → integration (Docker DB). Whole service → component (mock external services). Multiple services → E2E (full docker-compose). See `references/testing.md`.

**"I'm refactoring a tangled service"** — Start with the domain. Extract entities with private fields and behavior methods. Get them under unit tests. Then extract the repository. Then introduce the application layer. Ports last. See the refactoring sequence in `references/domain.md` and `references/application.md`.

## Reference modules

Load the relevant file before writing code for that layer:

| Task | Read |
|------|------|
| Design or modify domain types, value objects, factories | `references/domain.md` |
| Write or extend a repository, handle transactions, swap DB | `references/repository.md` |
| Add commands, queries, organize the `app` layer, naming | `references/application.md` |
| Add HTTP/gRPC handlers, set up dependency injection | `references/ports-adapters.md` |
| Write any tests | `references/testing.md` |
| Avoid common pitfalls, justify NOT applying a pattern | `references/anti-patterns.md` |

For projects with multiple aggregates, you will end up reading several of these in one task — that's expected. Each one is small.

## A few cross-cutting principles

- **Private fields, behavior methods.** Public getters are fine. Public setters almost never are — they break the "valid state in memory" guarantee. Instead expose methods like `tr.Cancel()`, `tr.ApproveReschedule(userType)`.

- **Separate transport types.** A `mysqlTraining` (DB shape with struct tags) is a different type from `training.Training` (domain shape). Mapping between them is boilerplate, and that boilerplate is the whole point — it lets each side change independently. See `references/anti-patterns.md` on why sharing structs across layers hurts.

- **Domain-first when you can.** When starting a new aggregate, build the domain layer with an in-memory repository implementation and unit tests. Defer the database choice until you actually know the access patterns. See `references/repository.md`.

- **Business language in names.** Command names mirror what business people say: `ScheduleTraining`, `CancelTraining`, `ApproveReschedule`. Not `CreateTraining`, `UpdateTraining`, `DeleteTraining`. If you find yourself reaching for CRUD verbs in the `app/` layer, you are probably losing domain information.

- **Errors as values, with types.** Define exported error variables (`var ErrHourNotAvailable = errors.New(...)`) or exported error types (`type ForbiddenToSeeTrainingError struct{...}`) in the domain. Application and port layers can match on them with `errors.Is` / `errors.As` and translate to transport-specific responses.

- **Tests run in parallel by default.** Every test you write should be safe to `t.Parallel()`. If it isn't, that's a signal — usually shared global state or non-unique IDs. Fix the test, don't disable parallelism.

## What this skill does NOT cover

- Microservice infrastructure setup (Cloud Run, Firebase, Terraform) — the book covers it in chapters 2, 3, 4, 14, 15, but those are environment-specific.
- Event Storming, Bounded Contexts, Aggregates as strategic patterns — these are introduced in chapter 16 of the book and were marked "coming soon" in subsequent chapters.
- Event Sourcing or polyglot persistence — mentioned as future optimizations on top of CQRS but not detailed.

For these, refer to the original book and to [threedots.tech](https://threedots.tech/).

# Reusable Go Patterns

Patterns extracted from this project (Three Dots Labs "Wild Workouts" — a DDD +
CQRS + Clean Architecture reference app) that transfer well to other Go projects.

## 1. Clean Architecture layering (per bounded context)

Each module is split into fixed layers, with
dependencies pointing inward only:

```
ports/      → HTTP/gRPC entrypoints (translate transport ↔ app)
app/        → use cases: command/ and query/ handlers
domain/     → pure business logic, no framework imports
adapters/   → DB, external gRPC clients (implement domain interfaces)
service/    → composition root: wires everything, returns Application
main.go     → just calls service.NewApplication + starts server
```

The dependency rule is enforced with `go-cleanarch` in CI. The domain package
imports nothing infrastructural — only stdlib and a shared errors package.

## 2. CQRS with generic decorators

Commands mutate, queries read — separate handler types, never mixed
(`internal/common/decorator/`):

```go
type CommandHandler[C any] interface {
    Handle(ctx context.Context, cmd C) error
}
type QueryHandler[Q any, R any] interface {
    Handle(ctx context.Context, q Q) (R, error)
}
```

`ApplyCommandDecorators` wraps a handler with logging + metrics generically, so
every use case gets observability for free. Each handler has a `NewXxxHandler`
constructor that panics on nil deps (fail-fast wiring) and returns the decorated
interface type.

## 3. Rich domain model with enforced invariants

- Struct fields are unexported; access via getter methods → object can't be
  constructed in an invalid state.
- One real constructor `NewTraining(...)` validates and returns `(*T, error)`.
- A separate `UnmarshalTrainingFromDatabase(...)` for repository hydration,
  explicitly documented as "DB only, not a constructor."
- Behavior lives on the entity (`UpdateNotes`, `CanBeCanceledForFree`), not in
  services.

## 4. Repository interface owned by the domain

`domain/training/repository.go` declares the `Repository` interface; `adapters/`
implements it. The update-with-closure pattern keeps load→mutate→save
transactional:

```go
UpdateTraining(ctx, uuid, user,
    updateFn func(ctx, *Training) (*Training, error)) error
```

Read queries return flat read-model DTOs (`query.Training`), not domain
entities — a separate model for the read side.

## 5. Typed errors → transport-agnostic mapping

`common/errors` defines `SlugError` carrying a machine-readable slug + an
`ErrorType` (Unknown / Authorization / IncorrectInput). The HTTP layer
(`httperr.RespondWithSlugError`) maps type → status code in one place. Domain
code never knows about HTTP; ports never `switch` on error strings.

## 6. Explicit composition root

`service.NewApplication(ctx)` builds clients, repositories, and every handler,
returning an `Application{Commands, Queries}` struct plus a `cleanup func()`. A
parallel `NewComponentTestApplication` swaps in mocks — same wiring, test
doubles. No DI framework, just plain constructors.

## 7. Shared `common/` infrastructure, not duplicated

Cross-cutting concerns live once: server bootstrap + middleware
(`common/server`), auth, structured logging, gRPC clients. Each service's
`main.go` is ~10 lines.

## 8. Contract-first codegen

HTTP types/routers generated from OpenAPI (`*.gen.go`), internal RPC from
protobuf (`common/genproto`). The hand-written `HttpServer` only implements the
generated interface — contracts stay the source of truth.

---

**What transfers best to a smaller project:** the generic decorator pattern
(#2), domain-owned repository interfaces (#4), and the slug-error mapping (#5)
give the most value with the least ceremony. The full 5-layer split (#1) and
codegen (#8) pay off mainly once you have multiple services or teams.

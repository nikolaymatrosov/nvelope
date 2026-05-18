---
name: go-clean-architecture
description: Use this skill whenever writing a Go web service, API, gRPC service, or any non-trivial Go application that has business logic (anything beyond a CLI tool or a simple script). Covers project layout (ports/adapters/app/domain), dependency inversion in Go, the Repository pattern with closure-based transactions, and CQRS-style command/query handlers. Trigger this even when the user just asks for "a Go HTTP handler", "a Go service", "how should I structure this Go project", "where should this database call go", "add a feature to this Go service", or pastes Go code that mixes HTTP, business logic, and database calls in one place — that mixing is exactly the smell this skill exists to fix. Based on Three Dots Labs' "Go with the Domain".
---

# Clean Architecture for Go Services

## When this matters

Go's simplicity tempts people to write services where HTTP handlers call the database directly, business rules sit inline between `if err != nil` blocks, and `internal/` is a flat folder of `.go` files. This works for a week. Then someone needs to swap Firestore for Postgres, or add gRPC alongside HTTP, or test a rule without spinning up the whole infrastructure — and the codebase fights back.

The pattern below — four layers, dependencies pointing inward — is the most boring solution to that problem, and that's the point. It looks like over-engineering in week one. It's the difference between a 1-month and 1-year-old project still moving at the same speed.

If the project is genuinely tiny (a CLI tool, a one-shot migration script, a webhook receiver that does one thing) **don't use this**. Apply it when there's domain logic worth protecting from the next person who joins the team.

## The four layers

```
internal/<service-name>/
├── domain/        # Pure business types and rules. No imports from other layers.
│   └── <aggregate>/   (e.g. training/, hour/)
├── app/           # Use cases. Orchestrates domain + adapters. Knows nothing about HTTP/gRPC.
│   ├── command/   # Write operations (mutations)
│   └── query/     # Read operations
├── ports/         # Entry points: HTTP handlers, gRPC servers, Pub/Sub subscribers.
└── adapters/      # Exits: database repos, gRPC clients to other services, message publishers.
```

**The dependency rule**: outer layers may import inner layers, never the reverse.

- `domain` imports nothing from this service.
- `app` may import `domain`.
- `ports` and `adapters` may import `app` and `domain`.
- `ports` and `adapters` do **not** import each other.

In Go this is enforced by the import graph itself — if you accidentally invert a dependency, the compiler tells you with an import cycle error. That error is a feature, not an obstacle.

## Dependency inversion the Go way

Inner layers need to *use* outer concerns (databases, other services) without *depending* on them. The standard answer is interfaces. Go's implicit interface satisfaction makes this cheaper than in most languages: **define the interface in the package that consumes it, not the package that implements it.**

```go
// internal/trainings/app/command/cancel_training.go
package command

// Defined here, in the consumer. The interface lists only what THIS command needs.
type trainingRepository interface {
    UpdateTraining(
        ctx context.Context,
        trainingUUID string,
        user training.User,
        updateFn func(ctx context.Context, tr *training.Training) (*training.Training, error),
    ) error
}

type userService interface {
    UpdateTrainingBalance(ctx context.Context, userID string, amountChange int) error
}

type CancelTrainingHandler struct {
    repo        trainingRepository
    userService userService
}
```

The concrete `TrainingsFirestoreRepository` lives in `adapters/` and never imports `command`. `command` never imports `adapters`. They meet in `main.go` (or a `service.go` setup function) where the concrete types are injected:

```go
// main.go — the only place that knows everything
repo := adapters.NewTrainingsFirestoreRepository(firestoreClient)
trainerSvc := adapters.NewTrainerGrpc(trainerClient)
userSvc := adapters.NewUsersGrpc(usersClient)

app := app.Application{
    Commands: app.Commands{
        CancelTraining: command.NewCancelTrainingHandler(repo, userSvc, trainerSvc),
        // ...
    },
    Queries: app.Queries{ /* ... */ },
}

httpServer := ports.NewHttpServer(app)
```

Two consequences worth internalising:

- Interfaces are **small** (often one method) and live next to their use site. Don't define one giant `Repository` interface in `domain/` and force every consumer to depend on all of it.
- Mocks for tests are trivial: a struct with one method that records calls or returns canned values. Don't reach for mocking frameworks for these.

## The Repository pattern with transactions

A repository abstracts persistence behind an interface defined in the domain (or app) package. The non-obvious part is **how to make transactions clean without leaking transaction objects everywhere**.

The pattern that works in Go is the **update-function closure**:

```go
// internal/trainings/domain/training/repository.go
package training

type Repository interface {
    GetTraining(ctx context.Context, trainingUUID string, user User) (*Training, error)
    UpdateTraining(
        ctx context.Context,
        trainingUUID string,
        user User,
        updateFn func(ctx context.Context, tr *Training) (*Training, error),
    ) error
}
```

The caller hands in a closure that mutates the entity. The repository:

1. Starts a transaction (or whatever the storage equivalent is).
2. Loads the entity.
3. Calls the closure with the loaded entity.
4. If the closure returns an error → rollback, return error.
5. If the closure returns success → save the result, commit.

Caller code is dead simple and has zero awareness of transaction objects:

```go
// In a command handler:
return h.repo.UpdateTraining(ctx, cmd.TrainingUUID, cmd.User,
    func(ctx context.Context, tr *training.Training) (*training.Training, error) {
        originalTime := tr.Time()
        if err := tr.ApproveReschedule(cmd.User.Type()); err != nil {
            return nil, err
        }
        // External call inside the transaction — if this fails, the DB rolls back.
        if err := h.trainerService.MoveTraining(ctx, tr.Time(), originalTime); err != nil {
            return nil, err
        }
        return tr, nil
    },
)
```

### SQL transaction implementation

For `database/sql` or `sqlx`, the deferred-rollback-or-commit pattern is the cleanest:

```go
func (m MySQLHourRepository) UpdateHour(
    ctx context.Context,
    hourTime time.Time,
    updateFn func(h *hour.Hour) (*hour.Hour, error),
) (err error) {                                    // NAMED return — important
    tx, err := m.db.Beginx()
    if err != nil {
        return errors.Wrap(err, "unable to start transaction")
    }
    defer func() {
        err = m.finishTransaction(err, tx)          // overrides err if commit/rollback fails
    }()

    existing, err := m.getOrCreateHour(ctx, tx, hourTime, true /* FOR UPDATE */)
    if err != nil {
        return err
    }
    updated, err := updateFn(existing)
    if err != nil {
        return err
    }
    return m.upsertHour(tx, updated)
}

func (m MySQLHourRepository) finishTransaction(err error, tx *sqlx.Tx) error {
    if err != nil {
        if rbErr := tx.Rollback(); rbErr != nil {
            return multierr.Combine(err, rbErr)
        }
        return err
    }
    if commitErr := tx.Commit(); commitErr != nil {
        return errors.Wrap(commitErr, "failed to commit tx")
    }
    return nil
}
```

Note the **named return value `(err error)`**. Without it, the deferred function cannot inspect or override the error. Use `SELECT ... FOR UPDATE` (or your DB's equivalent) when loading inside `UpdateHour` to get pessimistic locking, otherwise concurrent updates will silently overwrite each other.

### Repository as authorization boundary

Make authorization a parameter to the repository, not a separate cross-cutting check. If `GetTraining(ctx, uuid, user)` requires a `User` and internally calls `training.CanUserSeeTraining(user, *tr)`, there is no path to read a training without going through the authorization. New team members literally cannot forget the check because the function signature requires it.

```go
func (r TrainingsFirestoreRepository) GetTraining(
    ctx context.Context, trainingUUID string, user training.User,
) (*training.Training, error) {
    // ... load training ...
    if err := training.CanUserSeeTraining(user, *tr); err != nil {
        return nil, err  // returns ForbiddenToSeeTrainingError
    }
    return tr, nil
}
```

The actual authorization rule (`CanUserSeeTraining`) lives in the domain, where business people would expect it. The repository just enforces it on every read path.

## CQRS-style command and query handlers

The application layer splits into `command/` (writes) and `query/` (reads). Each operation is a struct + handler pair:

```go
// app/command/cancel_training.go
package command

type CancelTraining struct {
    TrainingUUID string
    User         training.User
}

type CancelTrainingHandler struct {
    repo           trainingRepository
    userService    userService
    trainerService trainerService
}

func NewCancelTrainingHandler(repo trainingRepository, us userService, ts trainerService) CancelTrainingHandler {
    if repo == nil { panic("missing repo") }
    if us == nil { panic("missing userService") }
    if ts == nil { panic("missing trainerService") }
    return CancelTrainingHandler{repo, us, ts}
}

func (h CancelTrainingHandler) Handle(ctx context.Context, cmd CancelTraining) (err error) {
    defer func() { logs.LogCommandExecution("CancelTraining", cmd, err) }()
    return h.repo.UpdateTraining(ctx, cmd.TrainingUUID, cmd.User, /* closure */)
}
```

Why this shape:

- **Commands return only `error`.** They mutate state. If you need to return a generated ID, return it (`(string, error)`) — pragmatism beats purity.
- **Queries return data, never mutate.** They can use a separate `ReadModel` interface that's shaped for the UI's needs, not for the domain.
- **Cross-cutting concerns** (logging, metrics, tracing) go in the handler, not the port. Then it's measured the same way whether called from HTTP, gRPC, or a Pub/Sub subscriber.
- **Constructor with nil checks** + panic. These dependencies are wired once at startup; if any are missing it's a programmer error, not a runtime condition.

Name commands and queries from the **business vocabulary**: `ScheduleTraining`, `CancelTraining`, `ApproveReschedule` — not `CreateTraining`, `UpdateTraining`, `DeleteTraining`. CRUD verbs erase the domain.

The `Application` struct aggregates everything and is what `ports/` receives:

```go
// app/app.go
type Application struct {
    Commands Commands
    Queries  Queries
}
type Commands struct {
    CancelTraining            command.CancelTrainingHandler
    ApproveTrainingReschedule command.ApproveTrainingRescheduleHandler
    // ...
}
type Queries struct {
    AvailableHours    query.AvailableHoursHandler
    HourAvailability  query.HourAvailabilityHandler
    // ...
}
```

Ports then call `h.app.Commands.CancelTraining.Handle(ctx, cmd)` — uniform, discoverable, trivial to grep for.

## Don't share structs across layers

Resist the urge to use one struct for HTTP request body + DB row + domain entity. It seems DRY; it's actually high coupling. The day you need to add a field to the database that shouldn't be in the API response, you'll either expose it accidentally or paper over it with delete-after-load. Both are bugs waiting to happen.

Keep three (or more) types:

- The OpenAPI/gRPC-generated request/response types in `ports/`.
- The domain type in `domain/` (private fields, behaviors).
- A separate DB-row struct in `adapters/` (with `db:"..."` tags or whatever your driver wants).

Mapping between them is a few lines of boilerplate per type. That boilerplate is cheap; the coupling it prevents is not. DRY applies to **behavior**, not to data shapes that happen to look similar today.

For the rationale and a worked example of the bug this prevents, see Chapter 5 of "Go with the Domain" ("When to stay away from DRY").

## Connections to the other skills

- **Domain modeling** — what goes inside `domain/`. Always-valid types, value objects, encapsulation. See the `go-ddd-tactical` skill if it's available.
- **Testing strategy** — how to test each layer (unit for domain, integration for adapters, component for the whole service). See the `go-test-architecture` skill if it's available.

## Heuristics

- A handler with more than ~20 lines of business logic is a hint to push logic into the domain.
- If you can't read a command handler and explain what it does to a non-engineer in one sentence, the domain is wrong, not the handler.
- If `adapters/` imports `ports/` (or vice versa), you've made a mistake. Refactor.
- If `domain/` imports anything from your DB driver, ORM, or web framework, that's the bug.
- "Where should this code live?" — ask whether the rule would survive switching from Firestore to Postgres (→ domain), whether it would survive switching from HTTP to gRPC (→ app), or whether it's purely about talking to one specific external thing (→ adapters or ports).

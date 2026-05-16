# Application Layer (CQRS)

The application layer orchestrates. It does not contain business rules — those belong in `domain/`. It does not know about HTTP or gRPC — those belong in `ports/`. Its job is to glue them together for one specific use case.

CQRS splits the application layer further into **commands** (state changes — return only an error) and **queries** (reads — return data, no side effects). Each command and each query is a separate type. The result is that every use case of your service has exactly one place it lives.

## File and package layout

```text
app/
├── app.go              # Application struct: Commands + Queries
├── command/
│   ├── schedule_training.go
│   ├── cancel_training.go
│   ├── approve_training_reschedule.go
│   └── *_test.go       # unit tests with simple mocks
└── query/
    ├── available_hours.go
    ├── training_details.go
    ├── types.go        # response types specific to queries
    └── read_model.go   # read-model interfaces (when not using the repo)
```

## The Application struct

A single struct holds every command handler and every query handler. The ports layer receives this struct and dispatches into it:

```go
// in app/app.go

package app

import (
    "yourproject/internal/trainings/app/command"
    "yourproject/internal/trainings/app/query"
)

type Application struct {
    Commands Commands
    Queries  Queries
}

type Commands struct {
    ScheduleTraining          command.ScheduleTrainingHandler
    CancelTraining            command.CancelTrainingHandler
    RequestTrainingReschedule command.RequestTrainingRescheduleHandler
    ApproveTrainingReschedule command.ApproveTrainingRescheduleHandler
}

type Queries struct {
    AvailableHours    query.AvailableHoursHandler
    TrainingDetails   query.TrainingDetailsHandler
    AllTrainings      query.AllTrainingsHandler
}
```

A new team member looking at this file gets the entire API of the service at a glance. No hunting through HTTP routers.

## A command handler

A command has two parts: a parameter struct and a handler struct.

```go
// in app/command/cancel_training.go

package command

import (
    "context"
    "fmt"

    "yourproject/internal/trainings/domain/training"
    "yourproject/internal/common/logs"
)

type CancelTraining struct {
    TrainingUUID string
    User         training.User
}

type CancelTrainingHandler struct {
    repo           training.Repository
    userService    UserService
    trainerService TrainerService
}

func NewCancelTrainingHandler(
    repo training.Repository,
    userService UserService,
    trainerService TrainerService,
) CancelTrainingHandler {
    if repo == nil {
        panic("nil repo")
    }
    if userService == nil {
        panic("nil userService")
    }
    if trainerService == nil {
        panic("nil trainerService")
    }
    return CancelTrainingHandler{repo, userService, trainerService}
}

func (h CancelTrainingHandler) Handle(ctx context.Context, cmd CancelTraining) (err error) {
    defer func() {
        logs.LogCommandExecution("CancelTraining", cmd, err)
    }()

    return h.repo.UpdateTraining(
        ctx,
        cmd.TrainingUUID,
        cmd.User,
        func(ctx context.Context, tr *training.Training) (*training.Training, error) {
            if err := tr.Cancel(); err != nil {
                return nil, err
            }
            if delta := training.CancelBalanceDelta(*tr, cmd.User.Type()); delta != 0 {
                if err := h.userService.UpdateTrainingBalance(ctx, tr.UserUUID(), delta); err != nil {
                    return nil, fmt.Errorf("unable to change trainings balance: %w", err)
                }
            }
            if err := h.trainerService.CancelTraining(ctx, tr.Time()); err != nil {
                return nil, fmt.Errorf("unable to cancel training: %w", err)
            }
            return tr, nil
        },
    )
}
```

**Things to notice:**

- The command struct uses domain types (`training.User`), not primitive strings. The port layer constructs them; mistakes (wrong argument order, missing field) become compile errors.
- The handler struct holds dependencies and is constructed once in `main.go`.
- The constructor panics on `nil` dependencies. This is fail-fast at startup — better than a `nil` pointer panic at 3am.
- The `Handle` method uses named return `(err error)` so the `defer` for logging can see what happened.
- All business decisions (`tr.Cancel()`, `CancelBalanceDelta`) come from the domain. The handler is pure orchestration.
- Cross-cutting concerns — logging, metrics, tracing — go in the handler. The domain stays focused on rules.

## Defining dependencies as interfaces, in the same file

The handler depends on `UserService` and `TrainerService`. Define those interfaces in the `command` package — not in a separate `interfaces.go`, not in the adapter package. Define them where they're used:

```go
// in app/command/cancel_training.go (or a sibling file)

type UserService interface {
    UpdateTrainingBalance(ctx context.Context, userUUID string, amountChange int) error
}

type TrainerService interface {
    CancelTraining(ctx context.Context, trainingTime time.Time) error
    MoveTraining(ctx context.Context, newTime, oldTime time.Time) error
}
```

The Dependency Inversion Principle in Go: the consumer declares the interface. The implementation lives in `adapters/`. This makes the consumer's needs explicit and lets you mock with a 5-line struct.

If two handlers share the same dependency interface, deduplicate it into one file (e.g. `app/command/services.go`). Don't over-engineer this — three handlers each with their own small interface is fine.

## A query handler

Queries are read-only. Side effects beyond logging and metrics are not allowed.

```go
// in app/query/available_hours.go

package query

import (
    "context"
    "time"
)

type AvailableHours struct {
    From time.Time
    To   time.Time
}

type AvailableHoursReadModel interface {
    AvailableHours(ctx context.Context, from, to time.Time) ([]Date, error)
}

type AvailableHoursHandler struct {
    readModel AvailableHoursReadModel
}

func NewAvailableHoursHandler(rm AvailableHoursReadModel) AvailableHoursHandler {
    if rm == nil {
        panic("nil readModel")
    }
    return AvailableHoursHandler{readModel: rm}
}

func (h AvailableHoursHandler) Handle(ctx context.Context, q AvailableHours) ([]Date, error) {
    if q.From.After(q.To) {
        return nil, errors.NewIncorrectInputError("date-from-after-date-to", "Date from after date to")
    }
    return h.readModel.AvailableHours(ctx, q.From, q.To)
}
```

## Query response types ≠ domain types

The query layer defines its own response types, shaped for the UI:

```go
// in app/query/types.go

package query

import "time"

type Date struct {
    Date         time.Time
    HasFreeHours bool
    Hours        []Hour
}

type Hour struct {
    Available            bool
    HasTrainingScheduled bool
    Hour                 time.Time
}
```

Compare to the domain:

```go
// in domain/training/hour/hour.go (private fields, methods only)
type Hour struct {
    hour         time.Time
    availability Availability
}
```

Why two types? The UI needs a flattened, redundant shape with `HasTrainingScheduled` as a boolean — easy to render. The domain hides `availability` behind methods like `IsAvailable()`. Both are correct for their context. As the application grows, they will diverge further. That's a feature.

## Where read-model data comes from

A simple project can satisfy queries from the same database as the write models. The read-model interface gets implemented in `adapters/` and runs queries against the same tables.

For complex queries or scale, you can later split:

- Maintain a separate, denormalized read store (e.g. ElasticSearch indexed from domain events).
- Use event sourcing where read models are projections.

The point of the interface is that you can swap implementations without touching `app/` or `domain/`. Defer this complexity until you need it.

## Slug-based errors for port-agnostic responses

Application errors need to map cleanly to HTTP codes AND gRPC codes AND CLI exit codes. Use a small error type with a machine-readable slug:

```go
// in internal/common/errors/errors.go

package errors

type SlugError struct {
    slug      string
    message   string
    errorType ErrorType
}

type ErrorType int

const (
    ErrorTypeUnknown ErrorType = iota
    ErrorTypeAuthorization
    ErrorTypeIncorrectInput
    ErrorTypeNotFound
)

func NewIncorrectInputError(slug, message string) SlugError {
    return SlugError{slug: slug, message: message, errorType: ErrorTypeIncorrectInput}
}

func NewAuthorizationError(slug, message string) SlugError {
    return SlugError{slug: slug, message: message, errorType: ErrorTypeAuthorization}
}

func (s SlugError) Error() string    { return s.message }
func (s SlugError) Slug() string     { return s.slug }
func (s SlugError) ErrorType() ErrorType { return s.errorType }
```

The HTTP port translates to status codes; the gRPC port translates to gRPC codes. The slug travels through to the client, which can localize the message in the UI.

```go
// in app/query/available_hours.go
if q.From.After(q.To) {
    return nil, errors.NewIncorrectInputError(
        "date-from-after-date-to",
        "Date from after date to",
    )
}
```

```go
// in ports/http/errors.go
func RespondWithSlugError(err error, w http.ResponseWriter, r *http.Request) {
    var slugErr errors.SlugError
    if errors.As(err, &slugErr) {
        switch slugErr.ErrorType() {
        case errors.ErrorTypeIncorrectInput:
            httperr.BadRequest(slugErr.Slug(), slugErr, w, r)
        case errors.ErrorTypeAuthorization:
            httperr.Forbidden(slugErr.Slug(), slugErr, w, r)
        // ...
        }
        return
    }
    httperr.InternalError("internal-error", err, w, r)
}
```

## Naming: speak the business language

Command and query names mirror what business people say. NOT CRUD verbs:

| ✗ CRUD name           | ✓ Business name           |
|-----------------------|---------------------------|
| `CreateTraining`      | `ScheduleTraining`        |
| `UpdateTraining`      | `RequestTrainingReschedule`, `ApproveTrainingReschedule` |
| `DeleteTraining`      | `CancelTraining`          |
| `UpdateUserBalance`   | `RefundTrainingCredit`, `ChargeTrainingFee` |

If you find yourself reaching for `Create*` / `Update*` / `Delete*`, you are probably losing domain information. "Update training" with a `proposedNewTime` field is two separate business actions (`RequestReschedule`, `ApproveReschedule`) collapsed into one — and the business rules for each are different.

## Should every command/query be a separate type?

Yes, even tiny ones. The temptation to "save boilerplate" by putting all reschedule logic in one `RescheduleHandler` with branching is a trap — once you have the shared handler, every new related action piles more branches on it. Three or four months later it's a thousand lines.

Writing the command struct + handler struct + constructor + `Handle` method is three minutes of work. It pays back many times over the lifetime of the project.

## Testing the application layer

The application layer's main job is orchestration. The tests:

- Verify the handler calls the right domain methods.
- Verify it calls external services in the right order.
- Verify it returns the right error in failure cases.

Mocks here are tiny — usually a struct that records what was called:

```go
type trainerServiceMock struct {
    trainingsCancelled []time.Time
    moveErr            error
}

func (t *trainerServiceMock) CancelTraining(ctx context.Context, trainingTime time.Time) error {
    t.trainingsCancelled = append(t.trainingsCancelled, trainingTime)
    return nil
}

func (t *trainerServiceMock) MoveTraining(ctx context.Context, newTime, oldTime time.Time) error {
    return t.moveErr
}
```

No mocking library is needed. Test tables work well — see `references/testing.md` for full examples.

If the test would just check that "mock was called", skip it — you're testing the mock. Test the *outcomes* (state changes, returned errors, downstream calls).

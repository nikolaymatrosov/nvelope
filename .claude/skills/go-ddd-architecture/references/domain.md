# Domain Layer (DDD-Lite)

The domain layer holds business rules — and nothing else. No HTTP, no database, no gRPC, no logging frameworks, no external service clients. It is the most stable layer of the application: code here can survive framework changes, database migrations, and rewrites of everything else.

The whole point: when a business person asks "when can a training be cancelled?", you should be able to point at one method (`training.CanBeCanceledForFree()`) and they should be able to read it.

## The three rules

### Rule 1: Reflect business logic literally

Stop modeling entities as "structs with setters". Model them as **types with behaviors**. The methods on a domain type are the verbs a business person would use.

A business person says "schedule a training at 13:00" — not "set the `availability` attribute of hour 13:00 to `training_scheduled`". The code should match the first phrasing:

```go
// In package: domain/training/hour

func (h *Hour) ScheduleTraining() error {
    if !h.IsAvailable() {
        return ErrHourNotAvailable
    }
    h.availability = TrainingScheduled
    return nil
}
```

The test for this method reads like documentation:

```go
func TestHour_ScheduleTraining(t *testing.T) {
    h, err := hour.NewAvailableHour(validTrainingHour())
    require.NoError(t, err)

    require.NoError(t, h.ScheduleTraining())

    assert.True(t, h.HasTrainingScheduled())
    assert.False(t, h.IsAvailable())
}

func TestHour_ScheduleTraining_when_not_available(t *testing.T) {
    h := newNotAvailableHour(t)
    assert.Equal(t, hour.ErrHourNotAvailable, h.ScheduleTraining())
}
```

The question to ask while designing a method: *"Could a non-technical stakeholder read this and understand when it succeeds?"* If not, the method probably has a wrong name or is doing too much.

### Rule 2: Always keep a valid state in memory

A domain object must NEVER be in an invalid state. Achieve this with three things:

1. **Private fields** — only the domain package can mutate them.
2. **Constructor with validation** — the only way to create an instance is through a function that validates inputs.
3. **Methods that enforce invariants** — every state-changing method checks preconditions and returns an error if they are violated.

```go
package training

type Training struct {
    uuid     string
    userUUID string
    userName string
    time     time.Time
    notes    string

    proposedNewTime time.Time
    moveProposedBy  UserType

    canceled bool
}

func NewTraining(uuid, userUUID, userName string, trainingTime time.Time) (*Training, error) {
    if uuid == "" {
        return nil, errors.New("empty training uuid")
    }
    if userUUID == "" {
        return nil, errors.New("empty userUUID")
    }
    if userName == "" {
        return nil, errors.New("empty userName")
    }
    if trainingTime.IsZero() {
        return nil, errors.New("zero training time")
    }
    return &Training{
        uuid:     uuid,
        userUUID: userUUID,
        userName: userName,
        time:     trainingTime,
    }, nil
}
```

Public getters are fine — they preserve the invariant. Public setters almost never are.

**Anti-pattern:**

```go
// BAD: caller can put the object in an invalid state
h := hour.NewAvailableHour(...)
if h.HasTrainingScheduled() {
    h.SetState(hour.Available)  // ← who validates this transition?
} else {
    return errors.New("unable to cancel training")
}
```

**Pattern:**

```go
// GOOD: the transition rule lives inside the type
func (h *Hour) CancelTraining() error {
    if !h.HasTrainingScheduled() {
        return ErrNoTrainingScheduled
    }
    h.availability = Available
    return nil
}

// caller:
if err := h.CancelTraining(); err != nil {
    return err
}
```

### Rule 3: The domain is database-agnostic

The domain knows nothing about how persistence works. It does not import database drivers, struct tags, or ORM types. The domain declares a `Repository` interface; an outer layer implements it.

This means **no `` `db:"hour"` `` struct tags on domain types.** Define a separate "transport" type in the `adapters/` package (e.g. `mysqlHour` with `db` tags) and map between it and the domain type. See `references/repository.md`.

Why this rule matters in Go specifically: Go has no "magic" annotations like Java/Hibernate. Coupling a domain type to a specific driver tends to be more visible and more invasive in Go, so the cost of NOT separating them shows up faster.

## Value objects

Use a small custom type wherever a primitive feels suspicious — when a `string` could be confused with another `string`, or when there's an enumerated set of valid values.

```go
package training

type UserType struct {
    s string
}

var (
    Trainer  = UserType{"trainer"}
    Attendee = UserType{"attendee"}
)

func NewUserTypeFromString(s string) (UserType, error) {
    switch s {
    case "trainer":
        return Trainer, nil
    case "attendee":
        return Attendee, nil
    default:
        return UserType{}, fmt.Errorf("invalid user type: %q", s)
    }
}

func (u UserType) String() string { return u.s }
func (u UserType) IsZero() bool   { return u.s == "" }
```

Why a struct wrapping a string instead of `type UserType string`? Two reasons: (1) you cannot construct an invalid value with a literal — `UserType{"hax"}` won't compile from outside the package because the field is unexported; (2) you can add methods and an `IsZero` check.

The `IsZero` method matters because Go zero values pass into your code through struct literals. The domain factory should reject them.

## Factories for entities with shared config

When an entity has knobs that aren't tied to a single instance (e.g. "training hours must be between 9 and 17 UTC", "schedules can't be set more than 6 months ahead"), put them in a factory type rather than passing the config through every constructor call:

```go
type FactoryConfig struct {
    MaxWeeksInTheFutureToSet int
    MinUtcHour               int
    MaxUtcHour               int
}

type Factory struct {
    fc FactoryConfig
}

func NewFactory(fc FactoryConfig) (Factory, error) {
    if err := fc.Validate(); err != nil {
        return Factory{}, err
    }
    return Factory{fc: fc}, nil
}

func (f Factory) NewAvailableHour(hour time.Time) (*Hour, error) {
    if err := f.validateTime(hour); err != nil {
        return nil, err
    }
    return &Hour{hour: hour, availability: Available}, nil
}

func (f Factory) Config() FactoryConfig { return f.fc }
func (f Factory) IsZero() bool          { return f.fc.MaxWeeksInTheFutureToSet == 0 }
```

The factory is constructed once in `main.go` and injected into the application layer (and tests).

## Errors in the domain

Define errors in the domain package, exported, so outer layers can match on them:

```go
// Sentinel errors for simple cases
var (
    ErrHourNotAvailable     = errors.New("hour is not available")
    ErrNoTrainingScheduled  = errors.New("no training scheduled")
    ErrTrainingAlreadyCanceled = errors.New("training is already canceled")
)

// Error types when context matters
type ForbiddenToSeeTrainingError struct {
    RequestingUserUUID string
    TrainingOwnerUUID  string
}

func (e ForbiddenToSeeTrainingError) Error() string {
    return fmt.Sprintf("user %q can't see user %q training",
        e.RequestingUserUUID, e.TrainingOwnerUUID)
}
```

Outer layers (HTTP, gRPC) match with `errors.Is` and `errors.As` and translate to transport-specific responses (HTTP 403, gRPC `PermissionDenied`, etc.). See `references/application.md` on slug-based errors.

## Free-standing domain functions

Not every domain operation belongs on an entity. When the operation is a pure function over domain values, just write a function in the entity's package:

```go
package training

// CancelBalanceDelta returns how many trainings the attendee gains or loses
// when this training is cancelled by the given user type.
func CancelBalanceDelta(tr Training, cancelingUserType UserType) int {
    if tr.CanBeCanceledForFree() {
        return 1 // give the training back
    }
    switch cancelingUserType {
    case Trainer:
        return 2 // refund + penalty for trainer cancelling late
    case Attendee:
        return 0 // forfeit the training
    default:
        panic(fmt.Sprintf("not supported user type %s", cancelingUserType))
    }
}
```

This separates the *query* ("what is the balance delta?") from the *command* ("cancel this training"), keeping each method small. In Java you might wrap this in a `DomainService`. In Go, a function is fine.

## Testing helpers (in `_test.go` files in the same package)

Long-lived tests benefit from named factory helpers and `Must*` constructors. These live alongside the tests:

```go
// in user_test.go or testing.go

func newExampleTrainerUser(t *testing.T) training.User {
    u, err := training.NewUser(uuid.New().String(), training.Trainer)
    require.NoError(t, err)
    return u
}

// MustNewUser panics on invalid input. Tests get to use it without
// require.NoError on every line. Production code uses NewUser.
func MustNewUser(uuid string, userType training.UserType) training.User {
    u, err := training.NewUser(uuid, userType)
    if err != nil {
        panic(err)
    }
    return u
}
```

For comparing domain types with private fields, use [`github.com/google/go-cmp/cmp`](https://github.com/google/go-cmp):

```go
func assertTrainingsEqual(t *testing.T, want, got *training.Training) {
    opts := []cmp.Option{
        cmp.AllowUnexported(training.Training{}, training.UserType{}, time.Time{}),
        cmpRoundTimeOpt, // ignore time precision below the second
    }
    assert.True(t, cmp.Equal(want, got, opts...), cmp.Diff(want, got, opts...))
}
```

`cmp.AllowUnexported` is the lever for comparing private fields. Without it, `cmp` refuses to look inside.

## Black-box testing

Domain tests live in package `training_test` (not `training`). This forces them to use only the exported API. If you can't write a meaningful test without poking at private fields, your public API is probably wrong.

## When you finish writing a domain type

A checklist:

- [ ] All fields are unexported.
- [ ] The only way to construct it is through `New*` (returns `error`) or `Unmarshal*FromDatabase` (used by adapters).
- [ ] Every state-changing method returns `error` if the transition is invalid.
- [ ] No method imports anything outside the standard library, the entity's own package, or other domain packages.
- [ ] Public getters exist for fields the application layer needs (`UUID()`, `Time()`, `IsCanceled()`). No public setters.
- [ ] There is a test for every behavior method, including the error paths.

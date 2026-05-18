---
name: go-ddd-tactical
description: Use this skill whenever designing Go types that represent business concepts — anything with rules about what state is valid, what transitions are allowed, who can do what to it. Triggers include modeling things like Order, Booking, Payment, Subscription, Account, Reservation, Invoice, Schedule, or any domain entity in a Go service. Also use this when reviewing Go structs with exported fields and no methods, when a struct's validation is happening in HTTP handlers instead of on the type itself, when business rules are scattered across `if` statements in handlers, or when the user asks "how should I model X in Go". The skill teaches always-valid construction, encapsulated state, behavior-rich types (not getters/setters), and secure-by-design APIs. Based on Three Dots Labs' "Go with the Domain" (DDD Lite chapters).
---

# Tactical Domain Modeling in Go

## When this matters

In any non-trivial Go service, there's a moment when a struct grows from "three fields and a JSON tag" to "the thing the whole business is about." That transition is where most codebases rot. Validation gets sprinkled across HTTP handlers. Setters appear. Someone writes `order.Status = "cancelled"` from a goroutine that didn't check if the order was already shipped. Six months later nobody knows where the rules live.

This skill is the antidote: a small set of patterns that make Go types *enforce* their own rules so the rest of the code physically cannot misuse them. None of it is novel — it's tactical DDD adapted for idiomatic Go (no setters everywhere, no annotations, no inheritance gymnastics).

If you're modelling a config struct, a DTO, or a row from a CSV, this is overkill. Apply it when there are rules that matter and people who will join the team later.

## Rule 1: Reflect the business vocabulary literally in method names

When the business says "schedule a training," the method is `ScheduleTraining()`. Not `SetStatus("scheduled")`. Not `Update()`. The method name is the same noun phrase a non-technical person would use.

```go
// Bad — leaks the implementation, opens room for misuse
type Hour struct {
    Available            bool
    HasTrainingScheduled bool
}
hour.Available = false
hour.HasTrainingScheduled = true

// Good — speaks the domain
func (h *Hour) ScheduleTraining() error {
    if !h.IsAvailable() {
        return ErrHourNotAvailable
    }
    h.availability = TrainingScheduled
    return nil
}
hour.ScheduleTraining()
```

The test for whether you got it right: read the method out loud to a product manager. If they nod, it's right. If they say "what does that mean technically?", rename it.

This isn't just style — it dissolves a class of bugs. The bad version above lets you set both flags incoherently. The good version makes that state literally unrepresentable.

## Rule 2: Always-valid state. Validate at construction, never expose mutation.

A constructed value of a domain type must already be valid. There is no "I just made it, give me a moment to fill in the fields" stage. Every method that mutates state checks invariants before changing anything.

To make this enforceable, the type lives in its own package with **all fields private**, exposed only through:

- A constructor function (`NewX`) that validates and returns `(X, error)`.
- Behavior methods (`(x *X) DoThing() error`) that check invariants.
- Read accessors only where the outside world genuinely needs them.

```go
// internal/trainings/domain/hour/hour.go
package hour

type Hour struct {
    hour         time.Time      // private
    availability Availability   // private
}

func NewAvailableHour(t time.Time) (*Hour, error) {
    if err := validateTime(t); err != nil {
        return nil, err
    }
    return &Hour{hour: t, availability: Available}, nil
}

func (h *Hour) CancelTraining() error {
    if !h.HasTrainingScheduled() {
        return ErrNoTrainingScheduled
    }
    h.availability = Available
    return nil
}

// Read accessor — yes; setter — no.
func (h Hour) Time() time.Time           { return h.hour }
func (h Hour) IsAvailable() bool          { return h.availability == Available }
func (h Hour) HasTrainingScheduled() bool { return h.availability == TrainingScheduled }
```

What this prevents: there is no way to construct an `Hour` outside the `hour` package. Even within the package, `NewAvailableHour` is the only path to one with `availability == Available`. A caller who tries `h.availability = TrainingScheduled` gets a compile error because they can't see the field.

**No dumb getters and setters.** Read accessors are fine when needed. A setter (`func (h *Hour) SetAvailability(a Availability)`) re-opens every door you just closed. If you need to expose a state transition, expose the *behavior* (`CancelTraining`, `ScheduleTraining`, `MakeAvailable`), not the underlying field.

## Rule 3: Value Objects for things-not-IDs

When a domain concept has rules but isn't a long-lived "thing with an identity," make it a value object: a small immutable type with its own constructor.

```go
package training

type UserType struct {
    t string
}

var (
    Trainer  = UserType{"trainer"}
    Attendee = UserType{"attendee"}
)

func NewUserTypeFromString(s string) (UserType, error) {
    switch s {
    case "trainer":  return Trainer, nil
    case "attendee": return Attendee, nil
    default:         return UserType{}, fmt.Errorf("unknown user type %q", s)
    }
}

func (u UserType) String() string { return u.t }
func (u UserType) IsZero() bool   { return u.t == "" }
```

Now `UserType` cannot hold an invalid string. Functions take `UserType` instead of `string`, so passing `"admin"` where a user type is expected fails at compile time, not at runtime.

Other typical value objects: `Email`, `Money`, `Currency`, `Coordinates`, `DateRange`, `Percentage`. Anything where "any old string/int" would let bugs through.

## Rule 4: Entities are private-field structs in their own package

An entity has identity that persists over time (a `Training` exists across many changes). It is:

- In its own package (`training/`, `order/`, etc.).
- All fields private.
- Created through a `NewX` constructor that returns `(*X, error)`.
- Mutated only through behavior methods.
- Stored and reloaded via a `Repository` interface defined in the same package.

```go
package training

type Training struct {
    uuid             string
    userUUID         string
    userName         string
    time             time.Time
    notes            string
    proposedNewTime  time.Time
    moveProposedBy   UserType
    canceled         bool
}

func NewTraining(uuid, userUUID, userName string, t time.Time) (*Training, error) {
    if uuid == "" {
        return nil, errors.New("empty training uuid")
    }
    if userUUID == "" {
        return nil, errors.New("empty userUUID")
    }
    if t.IsZero() {
        return nil, errors.New("zero training time")
    }
    return &Training{uuid: uuid, userUUID: userUUID, userName: userName, time: t}, nil
}

func (t Training) UUID() string       { return t.uuid }
func (t Training) Time() time.Time    { return t.time }
func (t Training) IsCanceled() bool   { return t.canceled }

func (t Training) CanBeCanceledForFree() bool {
    return t.time.Sub(time.Now()) >= 24*time.Hour
}

var ErrTrainingAlreadyCanceled = errors.New("training is already canceled")

func (t *Training) Cancel() error {
    if t.IsCanceled() {
        return ErrTrainingAlreadyCanceled
    }
    t.canceled = true
    return nil
}
```

The entire business rule "you can't double-cancel" lives in `Cancel()`. There is no other path that sets `canceled = true`. The repository, the HTTP handler, the gRPC server, the message handler — none of them can shortcut this rule, because they all go through `Cancel()`.

## Rule 5: Domain stays database-agnostic

The domain package does **not** import your SQL driver, ORM, Firestore client, or anything else infrastructure-related. No `db:"..."` tags. No `firestore:"..."` tags. No JSON tags either (those belong on transport types).

When you need to persist or reload an entity, the persistence layer has its own row/document struct (a DTO) and maps it to/from the domain type. The mapping is a few lines; the decoupling is worth orders of magnitude more.

```go
// adapters/hour_mysql_repository.go — a SEPARATE struct for persistence
type mysqlHour struct {
    ID           string    `db:"id"`
    Hour         time.Time `db:"hour"`
    Availability string    `db:"availability"`
}
```

To rebuild a domain `*Hour` from the DB row, the domain package exposes a "factory" or "unmarshal" function that still enforces invariants. Don't export field setters — export a function:

```go
// in package hour
func (f Factory) UnmarshalHourFromDatabase(t time.Time, a Availability) (*Hour, error) {
    if t.IsZero() {
        return nil, errors.New("zero hour time")
    }
    return &Hour{hour: t, availability: a}, nil
}
```

This way, even values reconstructed from the DB go through validation. A corrupted row doesn't silently become a corrupted entity.

## Rule 6: Secure by design — encode authorization in signatures

If a method should only be callable by certain users, take a `User` parameter and check it inside. Don't rely on "the HTTP middleware will have already checked." Middleware can be forgotten. Function signatures cannot.

```go
package training

type ForbiddenToSeeTrainingError struct {
    RequestingUserUUID string
    TrainingOwnerUUID  string
}
func (f ForbiddenToSeeTrainingError) Error() string { /* ... */ }

func CanUserSeeTraining(user User, training Training) error {
    if user.Type() == Trainer {
        return nil
    }
    if user.UUID() == training.UserUUID() {
        return nil
    }
    return ForbiddenToSeeTrainingError{user.UUID(), training.UserUUID()}
}
```

The repository's `GetTraining` and `UpdateTraining` now take `user training.User` as a required parameter and call `CanUserSeeTraining` internally. A new team member literally cannot write code that reads a training without going through the authorization check — the function won't compile without a `User`.

## Constructors, Must-variants, and test helpers

For tests, normal constructors that return `(X, error)` get tiresome. Provide `Must` variants that panic — fine in tests, never used in production code:

```go
func NewUser(userUUID string, userType UserType) (User, error) { /* ... */ }

func MustNewUser(userUUID string, userType UserType) User {
    u, err := NewUser(userUUID, userType)
    if err != nil {
        panic(err)
    }
    return u
}
```

Also provide test helper builders (`newExampleTraining`, `newCanceledTraining`, `newValidAvailableHour`) — these live next to the tests and dramatically improve readability. For comparing domain values with private fields in tests, `github.com/google/go-cmp` with `cmp.AllowUnexported(YourType{})` is the cleanest tool.

## Where domain logic lives — and where it doesn't

If you find yourself writing `if` statements about business state in a command handler, that's a smell. Move it.

```go
// SMELL — calculation lives in the command handler
trainingBalanceDelta := 0
if training.CanBeCancelled() {
    trainingBalanceDelta = 1
} else {
    if user.Role == "trainer" {
        trainingBalanceDelta = 2
    } else {
        trainingBalanceDelta = 0
    }
}

// BETTER — the calculation IS the domain
package training

func CancelBalanceDelta(tr Training, cancelingUserType UserType) int {
    if tr.CanBeCanceledForFree() {
        return 1
    }
    switch cancelingUserType {
    case Trainer:  return 2  // 1 returned + 1 fine for late cancellation by trainer
    case Attendee: return 0  // fine for late cancellation
    default:
        panic(fmt.Sprintf("not supported user type %s", cancelingUserType))
    }
}
```

Plain functions are fine in Go — you don't need to invent a "domain service" object just because that's how Java does it. The point is that the rule lives in the `training` package, can be unit-tested without any infrastructure, and can be reused from any number of handlers.

## What goes in the domain package (recap)

- Entity structs with private fields and behavior methods.
- Value object types and their constructors.
- Pure functions that encode business rules (`CanUserSeeTraining`, `CancelBalanceDelta`).
- Custom error types so callers can distinguish failure modes (`ErrHourNotAvailable`, `ForbiddenToSeeTrainingError`).
- The `Repository` interface (the *interface*, not implementations).

What does **not** go in the domain package:

- HTTP handlers.
- Database drivers, queries, or row structs.
- gRPC servers or generated protobuf types.
- Logging or metrics calls.
- Anything that imports `net/http`, `database/sql`, `cloud.google.com/...`, etc.

## Heuristics

- "Where's the validation?" If the answer is "in the HTTP handler," you have a bug waiting to happen. Move it to the constructor or method.
- "How do you set this field?" If the answer is anything other than "you can't, you call this method," you have a setter you shouldn't have.
- "How would I rename this struct field?" If the answer involves touching ten files, your domain leaked. Make the field private and audit who's reading it.
- "How would this look to a non-engineer reading the code?" The closer this gets to "actually pretty understandable," the better the domain.
- It is fine — encouraged — to spend a couple of weeks building the domain with only an in-memory `Repository` implementation, before deciding what database to use. The shape of the domain is more durable than the choice of storage.

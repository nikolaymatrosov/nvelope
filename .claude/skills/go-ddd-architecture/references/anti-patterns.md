# Anti-Patterns and When NOT to Apply

These patterns are not free. They cost lines of code and require team agreement. The whole skill is wasted on the wrong project. This file is the antidote — read it whenever you're about to apply DDD-Lite / Clean Architecture / CQRS to something, and double-check you should.

## When NOT to apply these patterns

### Pure CRUD services

If your service is `POST /thing → INSERT`, `GET /thing/:id → SELECT`, `PUT /thing → UPDATE`, `DELETE /thing/:id → DELETE` — with no business rules beyond field validation — this skill is overkill. A flat package with HTTP handlers calling a thin DB layer is the right design.

The `users` service in *Go with the Domain* is intentionally NOT refactored to Clean Architecture. It's a username/email/balance CRUD. Forcing the layering on it would add boilerplate without solving any problem.

**Signal that you're over-applying:**

- Your `command/` handlers contain three lines: build a thing, save it. Every time.
- Your `domain/` types have only fields and validation, no behaviors.
- Your team can't articulate which business rules the architecture is protecting.

### Authentication, proxies, infrastructure services

Anything where the "domain" is really "translate this protocol to that protocol" or "verify this token". There's no business behavior to model. Skip the layering.

### Prototypes and throwaway code

If the service will be rewritten in three months, don't invest in long-term maintainability now. Move fast, ship, learn.

### Tiny single-purpose tools

A CLI tool, a one-off scraper, a webhook receiver. Single file, single main, done.

## Anti-patterns within the skill

These are patterns the book actively warns against — common ways developers misapply DDD/Clean Architecture in Go and end up worse off than before.

### Sharing struct types across layers

The setup: you have a `User` struct with `json:"..."` tags for the HTTP API and `firestore:"..."` tags for the database, and the same struct is the domain entity. "Saves boilerplate."

The bug: someone adds a `LastIP` field for security tracking, marshaled to the DB. It's now also exposed via the HTTP API. The fix is one line of "don't expose this field":

```go
// BAD
user.LastIP = nil  // before responding
```

This is fine *once*. As the application grows you get five, ten of these. One gets missed. A user's IP, billing address, or password hash leaks out an unrelated endpoint.

**The rule from the book:** DRY applies to *behaviors*, not *data*. Two structs holding similar fields are not duplication; they are two contracts that should evolve independently.

```go
// in domain/users:
type User struct {  // private fields, methods
    uuid string
    name string
    role Role
}

// in adapters/users_firestore.go:
type firestoreUser struct {
    UUID string `firestore:"uuid"`
    Name string `firestore:"name"`
    Role string `firestore:"role"`
    LastIP string `firestore:"last_ip"`  // ← stored
}

// in ports/openapi_types.gen.go (generated from OpenAPI):
type User struct {
    UUID        string `json:"uuid"`
    DisplayName string `json:"displayName"`
    Role        string `json:"role"`
    // LastIP is not in the OpenAPI spec; it cannot be exposed
}
```

Three structs. Mapping in between. Boring. Safe. Each can change without breaking the others.

### Logic in HTTP/gRPC handlers

The classic anti-pattern. The HTTP handler grows from "decode, call app, encode" to a few hundred lines of `if/else` business logic. Eventually, it's 8000 lines of conditionals.

The book calls these "Eight-thousanders". They are unmaintainable.

**Signal you're heading there:** you can't read the handler without scrolling.

**Fix:** any business `if` in the handler is a method that belongs on the domain entity. Move it. The handler should be no more than 30 lines per endpoint, mostly transport plumbing.

### Logic in the application layer

A more subtle version of the same problem. The handler is short, but it makes business decisions:

```go
// BAD
func (h CancelTrainingHandler) Handle(ctx context.Context, cmd CancelTraining) error {
    return h.repo.UpdateTraining(ctx, cmd.TrainingUUID, cmd.User,
        func(ctx context.Context, tr *training.Training) (*training.Training, error) {
            // ← business rules embedded in the handler
            if cmd.User.Role != "trainer" && tr.UserUUID != cmd.User.UUID {
                return nil, errors.Errorf("user can't cancel another user's training")
            }
            if time.Until(tr.Time) > 24*time.Hour {
                tr.Balance += 1
            } else if cmd.User.Role == "trainer" {
                tr.Balance += 2
            }
            tr.Canceled = true
            return tr, nil
        },
    )
}
```

Move all of this into the domain. The handler should be visibly *just orchestration*:

```go
// GOOD
func (h CancelTrainingHandler) Handle(ctx context.Context, cmd CancelTraining) error {
    return h.repo.UpdateTraining(ctx, cmd.TrainingUUID, cmd.User,
        func(ctx context.Context, tr *training.Training) (*training.Training, error) {
            if err := tr.Cancel(); err != nil {
                return nil, err
            }
            if delta := training.CancelBalanceDelta(*tr, cmd.User.Type()); delta != 0 {
                if err := h.userService.UpdateTrainingBalance(ctx, tr.UserUUID(), delta); err != nil {
                    return nil, err
                }
            }
            return tr, h.trainerService.CancelTraining(ctx, tr.Time())
        },
    )
}
```

The rule: `if` statements in the handler that decide *what should happen* (vs. what to do with an error or call result) belong in the domain.

### CRUD names instead of business names

`CreateTraining`, `UpdateTraining`, `DeleteTraining` — these names tell you nothing about what the operation *means*. They also tend to collapse multiple distinct business actions ("propose reschedule", "approve reschedule") into one fat `UpdateTraining` with switch logic.

Use names from the business: `ScheduleTraining`, `RequestReschedule`, `ApproveReschedule`, `CancelTraining`. If you cannot name an operation without resorting to CRUD verbs, you probably don't understand the business action yet — go ask.

### Exposing entity state via getters/setters

Domain types with `GetX()` / `SetX()` for every field are barely better than `struct{X int}` with public fields. The point of encapsulation is to make invalid states unrepresentable.

```go
// BAD
type Hour struct {
    hour         time.Time
    availability Availability
}

func (h *Hour) Hour() time.Time           { return h.hour }
func (h *Hour) SetHour(t time.Time)        { h.hour = t }
func (h *Hour) Availability() Availability { return h.availability }
func (h *Hour) SetAvailability(a Availability) { h.availability = a }
```

This is no protection at all. Anyone can put the object in any state.

```go
// GOOD
type Hour struct { /* private */ }

func NewAvailableHour(t time.Time) (*Hour, error) { /* validates */ }

func (h *Hour) Time() time.Time { return h.hour }            // OK: read access
func (h *Hour) IsAvailable() bool { return h.availability == Available }
func (h *Hour) ScheduleTraining() error { /* enforces invariant */ }
func (h *Hour) CancelTraining() error   { /* enforces invariant */ }
```

Public *getters* are fine — they don't mutate state. Public *setters* almost never are.

### Skipping the command/query struct for "simple" handlers

It's tempting to write:

```go
// Tempting but wrong
func (h CancelTrainingHandler) Handle(ctx context.Context, trainingUUID string, user training.User) error { ... }
```

instead of:

```go
func (h CancelTrainingHandler) Handle(ctx context.Context, cmd CancelTraining) error { ... }
```

Don't. The named struct:

- Names the operation in the type system (`command.CancelTraining` searchable across the codebase).
- Survives signature growth without breaking all callers.
- Makes test cases readable.
- Lets the port layer construct it explicitly, which catches mistakes at compile time.

The "saving" is three lines of struct definition. It's not worth it.

### `time.Sleep` in tests

Always wrong. Either you don't actually need to wait (then drop the sleep — make the dependency synchronous from the test's perspective via channels or `WaitGroup`), or you need to wait for an event (use `assert.Eventually` with a polling condition).

Sleep is the slowest *and* flakiest option simultaneously. Both costs.

### "Just use end-to-end tests, they catch everything"

E2E tests are the most expensive form of testing (slow, flaky, hard to debug) and the most cosmetic (they tell you "something is broken" without telling you which layer). They are necessary but they are not a substitute for unit/integration/component coverage.

The book quotes the Continuous Delivery Maturity Model: unit tests are "base level". Teams that rely primarily on E2E tests for confidence have lost the ability to refactor.

### Splitting microservices to avoid complexity

A common move: "this service is getting complex, let's split it into smaller ones." If the split is along the wrong axis — by REST endpoints, by database tables, by features rather than by bounded contexts — you create a **distributed monolith**: the same coupling, plus network calls.

The book is blunt on this: microservices don't reduce complexity by themselves. If you don't know where the natural seam is, you'll make things worse.

Apply CQRS within one service first. Use it to decouple the read and write models. If at that point you find clean separations of state and team ownership, *then* maybe split.

### Mocking the database

Don't write a mock for `*sql.DB` or `*firestore.Client` and call it an integration test. You're testing your mock, not your code's interaction with the database.

The integration test should run against the real database (in Docker). The unit test should mock at a higher layer — the `Repository` interface, not the database.

### Letting one mega-handler accumulate every variant

```go
// BAD
func (h Handler) UpdateTraining(ctx context.Context, cmd UpdateTraining) error {
    if cmd.NewTime != nil && cmd.IsApproved {
        // approve reschedule path
    } else if cmd.NewTime != nil {
        // propose reschedule path
    } else if cmd.Canceled {
        // cancel path
    } else if cmd.Notes != nil {
        // edit notes path
    } else {
        // generic update path
    }
}
```

The dispatch table at the top grows with every requirement, the test surface explodes. Each business action should be its own command + handler. Three minutes of "boilerplate" per action saves you weeks of detangling later.

### Premature event sourcing or polyglot persistence

These are listed in CQRS resources as "next steps". They are also where projects get stuck for months. Don't reach for them until basic CQRS has been in place for a while and you have *specific* read-side performance or audit requirements you can't meet otherwise.

The book mentions both but stops short of recommending them as defaults.

## When the patterns reveal themselves to be wrong-fit

Signs you should peel back complexity:

- Most of your domain methods are one-liners that just set a field. The "domain" isn't really doing anything.
- Your command handlers are all three lines: `cmd → repo.Save`. You don't need commands.
- Most of your queries are `SELECT * FROM table WHERE id = ?`. You don't need read models.
- Every test you write is a unit test for a method you're certain works. You're not gaining confidence; you're just adding bytes.

You can absolutely *remove* layers as you discover they don't fit. The patterns are a tool, not a religion.

## A pragmatic test

When you're about to add a new directory (`domain/`, `app/`, `adapters/`, `ports/`) for the first time in a service, ask:

1. Does this service have rules a non-technical person could discuss? (If no → skip `domain/`.)
2. Are there multiple ways to invoke the same operation? (If no → maybe skip `ports/` vs. `app/` split.)
3. Could you imagine swapping the database? (If no → still useful, but `adapters/` is mostly indirection.)
4. Is there orchestration logic separate from the database call? (If no → maybe skip `app/`.)

A "no" on all four means flat structure is fine. One "yes" means start picking up the relevant pieces of this skill. Three or four "yes"es means apply the whole stack.

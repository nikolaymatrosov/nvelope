---
name: go-test-architecture
description: Use this skill whenever writing Go tests beyond a trivial `func TestFoo(t *testing.T)`. Triggers include writing or restructuring tests for a Go service, choosing between unit/integration/component/e2e levels, writing repository tests against a real database, testing concurrent code, getting flaky CI failures, designing a test suite that's shared across multiple implementations of the same interface, using `t.Parallel()`, writing table-driven tests, or when the user asks "how should I test this Go code", "this test is flaky", or pastes a Go test that mixes too many concerns. Covers the four-layer test pyramid for layered services, idioms for parallel and table-driven tests, the loop-variable gotcha (Go 1.21 and earlier), integration tests against Docker databases, race-condition testing, and the test-sabotage technique. Based on Three Dots Labs' "Go with the Domain".
---

# Test Architecture for Go Services

## When this matters

Tests are not a chore. A well-designed test suite is how you keep deploying on Friday without panic. A badly designed one — slow, flaky, retried with sleeps, only running on someone's laptop — is worse than no tests, because it gives false confidence.

Most Go test guides stop at "use `t.Run` and table-driven tests." That's the surface. The harder question is **which kind of test should I write for this piece of code**, and the answer depends on what layer the code lives in. This skill maps test types onto the layers of a clean-architecture Go service and gives you the idioms for each.

The four principles to hold in mind throughout: tests must be **fast** (under a minute for the full suite, ideally under ten seconds locally), test **enough scenarios at the right level**, be **deterministic** (no retries, no sleeps), and be **runnable locally** so you don't have to push to a branch to see if you broke something.

## The four levels and where to use each

| Level         | What it tests                          | Uses Docker?      | Mocks                  | Speed          |
|---------------|----------------------------------------|-------------------|------------------------|----------------|
| Unit          | A single package in isolation          | No                | Most dependencies      | <1ms each      |
| Integration   | An adapter against real infrastructure | Yes (database)    | None                   | 10-200ms each  |
| Component     | One whole service, isolated            | Yes               | External services only | 100ms-1s each  |
| End-to-end    | All services together                  | Yes (everything)  | None                   | Slow, sparse   |

The default shape is roughly a pyramid (lots of unit, fewer integration, fewer component, even fewer e2e) — **but** for services that mostly aggregate data and don't have rich domain logic, the shape inverts and looks more like a "Christmas tree" with the integration layer being the widest. Match the shape to the application.

---

## Unit tests — domain and application logic

The domain layer has no external dependencies, so unit tests there should be the simplest you write. Aim for high coverage. Use **black-box testing** by suffixing the test package with `_test`:

```go
// hour_test.go
package hour_test     // note the _test suffix

import (
    "testing"
    "github.com/.../internal/trainer/domain/hour"
)

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

The `_test` suffix forces you to use only the public API of the package. This protects the tests from internal refactors — if a private field gets renamed, the tests don't care.

**`require` vs `assert`** (from `github.com/stretchr/testify`):

- `require.NoError(t, err)` *stops* the test on failure. Use it for errors and preconditions where continuing makes no sense.
- `assert.Equal(t, a, b)` reports the failure but continues. Use it when checking multiple fields, so you see all mismatches at once.

### Table-driven tests for domain rules

The domain is where corner cases live. Table-driven tests are the cleanest way to enumerate them:

```go
func TestFactoryConfig_Validate(t *testing.T) {
    testCases := []struct {
        Name        string
        Config      hour.FactoryConfig
        ExpectedErr string
    }{
        {
            Name:        "valid",
            Config:      hour.FactoryConfig{MaxWeeksInTheFutureToSet: 10, MinUtcHour: 10, MaxUtcHour: 12},
            ExpectedErr: "",
        },
        {
            Name:        "equal_min_and_max_hour",
            Config:      hour.FactoryConfig{MaxWeeksInTheFutureToSet: 10, MinUtcHour: 12, MaxUtcHour: 12},
            ExpectedErr: "",
        },
        {
            Name:        "min_greater_than_max",
            Config:      hour.FactoryConfig{MaxWeeksInTheFutureToSet: 10, MinUtcHour: 14, MaxUtcHour: 12},
            ExpectedErr: "MinUtcHour (14) is greater than MaxUtcHour (12)",
        },
    }

    for _, c := range testCases {
        t.Run(c.Name, func(t *testing.T) {
            err := c.Config.Validate()
            if c.ExpectedErr != "" {
                assert.EqualError(t, err, c.ExpectedErr)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Application-layer unit tests with hand-written mocks

Command/query handlers have external dependencies (a repository, other services). With small interfaces defined at the consumer (per Clean Architecture), mocking is trivial — usually a struct with one method:

```go
type repositoryMock struct {
    Trainings map[string]training.Training
}

func (r *repositoryMock) UpdateTraining(
    _ context.Context, uuid string, _ training.User,
    updateFn func(context.Context, *training.Training) (*training.Training, error),
) error {
    tr, ok := r.Trainings[uuid]
    if !ok { return training.NotFoundError{TrainingUUID: uuid} }
    updated, err := updateFn(context.Background(), &tr)
    if err != nil { return err }
    r.Trainings[uuid] = *updated
    return nil
}
```

You almost never need a mocking framework. Hand-written mocks read more clearly, are easier to debug, and only implement the methods this test actually calls.

**Don't write tests that just verify the mock was called.** If the test would change every time the handler changes, it's testing the implementation, not the behavior. Skip it.

### Comparing values with private fields

When asserting equality on domain types with private fields, the standard `reflect.DeepEqual` and `assert.Equal` won't see the private fields properly. Use `github.com/google/go-cmp` with `cmp.AllowUnexported`:

```go
func assertTrainingsEqual(t *testing.T, want, got *training.Training) {
    t.Helper()
    opts := []cmp.Option{
        cmp.AllowUnexported(training.Training{}, training.UserType{}, time.Time{}),
    }
    assert.True(t, cmp.Equal(want, got, opts...), cmp.Diff(want, got, opts...))
}
```

---

## Integration tests — adapters against real infrastructure

Integration tests verify that **you use the database correctly**, not that the database works. They run against a real instance (via `docker-compose`), pinned to the same major version as production.

The pattern that works well: define a **single test suite** that takes a `Repository` interface and exercises it. Then run that suite against every implementation (in-memory, Postgres, Firestore, etc.):

```go
package main_test

func TestRepository(t *testing.T) {
    rand.Seed(time.Now().UTC().UnixNano())
    repositories := createRepositories(t)   // returns []struct{Name string; Repository hour.Repository}

    for i := range repositories {
        r := repositories[i]                 // see "loop variable gotcha" below
        t.Run(r.Name, func(t *testing.T) {
            t.Parallel()
            t.Run("testUpdateHour",                  func(t *testing.T) { t.Parallel(); testUpdateHour(t, r.Repository) })
            t.Run("testUpdateHour_parallel",         func(t *testing.T) { t.Parallel(); testUpdateHour_parallel(t, r.Repository) })
            t.Run("testHourRepository_update_existing", func(t *testing.T) { t.Parallel(); testHourRepository_update_existing(t, r.Repository) })
            t.Run("testUpdateHour_rollback",         func(t *testing.T) { t.Parallel(); testUpdateHour_rollback(t, r.Repository) })
        })
    }
}
```

The in-memory implementation runs in microseconds and catches most contract violations during development. The real-database implementations run in CI and catch SQL/query mistakes.

### Keeping integration tests stable in parallel

The single biggest mistake in integration tests: asserting things like "the collection has exactly N rows." With `t.Parallel()` and a shared database, another test can have inserted between your "check empty" and "check has one" steps. The fix is to **never assert on totals; always assert on the specific thing you inserted**:

```go
// Bad — races with any other test using this table
assert.Len(t, allTrainings, 1)

// Good — only checks what THIS test created
got, err := repo.GetTrainingByUUID(ctx, createdUUID)
require.NoError(t, err)
assert.Equal(t, expectedTraining, got)
```

Alternatively, scope each test to its own user / UUID prefix / namespace so it physically can't interfere with others. If you find yourself spinning up a fresh DB per test, that's a sign you should restructure tests to be isolation-friendly instead.

### The loop variable gotcha (Go 1.21 and earlier)

In Go 1.21 and earlier, this looks right but is silently broken:

```go
for _, c := range testCases {
    t.Run(c.Name, func(t *testing.T) {
        t.Parallel()                  // <-- the bug
        if got := doThing(c.Input); got != c.Expected {
            t.Errorf("want %v got %v", c.Expected, got)
        }
    })
}
```

`t.Parallel()` makes the subtest defer execution until the parent loop exits. By then `c` has been overwritten by the last iteration, so **every subtest ends up testing the last case** — and they all pass, with the right subtest names in `-v` output. You only catch it by deliberately breaking the implementation and watching tests not fail.

The fix is to bind a fresh variable per iteration:

```go
// Go 1.21 and earlier — explicit fix
for i := range testCases {
    c := testCases[i]
    t.Run(c.Name, func(t *testing.T) {
        t.Parallel()
        // safe to use c here
    })
}
```

(In Go 1.22+ this was fixed at the language level — each iteration gets a fresh loop variable. If you know you're on 1.22+, the original form works. If unsure, write it the explicit way; it costs one line and never breaks.)

### Testing transactions actually roll back

Closure-based repository methods need their rollback path explicitly tested. The closure returns an error and we verify nothing got persisted:

```go
func testUpdateHour_rollback(t *testing.T, repository hour.Repository) {
    t.Helper()
    ctx := context.Background()
    hourTime := newValidHourTime()

    // First update succeeds — hour becomes available
    err := repository.UpdateHour(ctx, hourTime, func(h *hour.Hour) (*hour.Hour, error) {
        require.NoError(t, h.MakeAvailable())
        return h, nil
    })
    require.NoError(t, err)

    // Second update fails inside the closure — should not persist
    err = repository.UpdateHour(ctx, hourTime, func(h *hour.Hour) (*hour.Hour, error) {
        assert.True(t, h.IsAvailable())
        require.NoError(t, h.MakeNotAvailable())
        return h, errors.New("something went wrong")
    })
    require.Error(t, err)

    persisted, err := repository.GetOrCreateHour(ctx, hourTime)
    require.NoError(t, err)
    assert.True(t, persisted.IsAvailable(), "availability change should have been rolled back")
}
```

### Test sabotage — making sure the test actually tests

After writing a test for transaction logic, **deliberately break the implementation** (e.g., make `finishTransaction` always commit, never roll back). If your test still passes, the test wasn't testing what you thought. Fix the test. Restore the code.

This sounds silly until the first time you discover a six-month-old "passing" test that was never actually verifying anything.

### Testing concurrent database access

Optimistic-locking and `SELECT FOR UPDATE` behaviour can't be unit-tested in isolation — they need real concurrency against a real database. Spawn N goroutines that all try the same conflicting operation, release them at once, and assert exactly one succeeded:

```go
func testUpdateHour_parallel(t *testing.T, repository hour.Repository) {
    workersCount := 20
    var workersDone sync.WaitGroup
    workersDone.Add(workersCount)
    startWorkers := make(chan struct{})           // released at once
    trainingsScheduled := make(chan int, workersCount)

    for worker := 0; worker < workersCount; worker++ {
        workerNum := worker
        go func() {
            defer workersDone.Done()
            <-startWorkers                          // block until released

            schedulingTraining := false
            err := repository.UpdateHour(ctx, hourTime, func(h *hour.Hour) (*hour.Hour, error) {
                if h.HasTrainingScheduled() { return h, nil }
                if err := h.ScheduleTraining(); err != nil { return nil, err }
                schedulingTraining = true
                return h, nil
            })
            if schedulingTraining && err == nil {
                trainingsScheduled <- workerNum
            }
        }()
    }

    close(startWorkers)                            // GO!
    workersDone.Wait()
    close(trainingsScheduled)

    var winners []int
    for n := range trainingsScheduled { winners = append(winners, n) }
    assert.Len(t, winners, 1, "only one worker should successfully schedule")
}
```

This catches the entire class of bugs where two requests "both win" — far easier to write here than at the E2E level.

### What about gRPC client adapters?

Often, no. A function that wraps a generated gRPC client and does nothing but field mapping has nothing to test that wouldn't just be re-asserting the implementation:

```go
func (s UsersGrpc) UpdateTrainingBalance(ctx context.Context, userID string, amountChange int) error {
    _, err := s.client.UpdateTrainingBalance(ctx, &users.UpdateTrainingBalanceRequest{
        UserId: userID, AmountChange: int64(amountChange),
    })
    return err
}
```

Skip writing a test for this. Add coverage for it via component tests, which run real wire traffic.

---

## Component tests — one whole service, in isolation

Component tests run a single service end-to-end (real HTTP handlers, real database, real internal wiring) but **mock external services**. They prove that the service's pieces are connected correctly without depending on other teams' services being up.

The setup: provide two constructors for the service's `Application`. One for production, one for tests:

```go
// service.go
func NewApplication(ctx context.Context) (app.Application, func()) {
    // ... real gRPC clients to other services ...
    return newApplication(ctx, adapters.NewTrainerGrpc(...), adapters.NewUsersGrpc(...))
}

func NewComponentTestApplication(ctx context.Context) app.Application {
    return newApplication(ctx, TrainerServiceMock{}, UserServiceMock{})
}
```

Then start the service in a goroutine from `TestMain` and wait for the port to open:

```go
func TestMain(m *testing.M) {
    if !startService() { os.Exit(1) }
    os.Exit(m.Run())
}

func startService() bool {
    app := NewComponentTestApplication(context.Background())
    addr := os.Getenv("TRAININGS_HTTP_ADDR")
    go server.RunHTTPServerOnAddr(addr, func(r chi.Router) http.Handler {
        return ports.HandlerFromMux(ports.NewHttpServer(app), r)
    })
    return tests.WaitForPort(addr)    // dial in a loop with a timeout
}
```

**Do not use `time.Sleep` to wait for startup.** Dial the port in a loop. Sleeps make tests slow when generous and flaky when tight.

For making tests readable, wrap the generated HTTP client in helpers that hide the boilerplate:

```go
func (c TrainingsHTTPClient) CreateTraining(t *testing.T, note string, hour time.Time) string {
    response, err := c.client.CreateTrainingWithResponse(context.Background(),
        trainings.CreateTrainingJSONRequestBody{Notes: note, Time: hour})
    require.NoError(t, err)
    require.Equal(t, http.StatusNoContent, response.StatusCode())
    return lastPathElement(response.HTTPResponse.Header.Get("content-location"))
}

// Now the test reads as a story:
trainingUUID := client.CreateTraining(t, "some note", hour)
```

**What to cover at this level**: happy paths and a few key error paths per endpoint. Not corner cases — those belong in unit and integration tests. Component tests are the smoke detector that says "everything wires together."

---

## End-to-end tests — the whole platform, sparingly

E2E tests spin up every service via `docker-compose` and exercise them through their public HTTP APIs. They are slow and brittle by nature; that's accepted. The cost of getting them wrong is that the team starts ignoring them.

Keep them few and shallow. They check that services connect correctly — a contract has not been broken between team A and team B. Logic and corner cases must already be covered downward.

If running everything together is genuinely impossible (too many services, too much data, too much external state), accept that and lean harder on **contract tests** between services and on robust component tests inside each. Don't fake an E2E suite that doesn't actually exercise the real seams.

---

## A few more rules of thumb

- A test suite that takes longer than a minute locally is broken architecture, not just slow. Find the offenders with `go test -v` timing or `-cpuprofile` and fix them.
- If you're adding a sleep to fix flakiness, you have a synchronization bug, not a timing bug. Find the actual missing signal.
- "Coverage" is a poor goal on its own. 100% coverage with brittle tests is worse than 70% with sharp ones. The question to ask of every test is **how easily could the code it covers break in a way this test would catch?** If the answer is "it couldn't," delete the test.
- Tests on neighbouring layers should overlap by design. Unit tests cover the domain rule; integration tests cover the SQL query; component tests cover the wiring. When a feature breaks, multiple tests should fail, telling you where.
- For non-trivial domain assertions, **write a helper** (`assertHourInRepository`, `assertTrainingsEqual`). Helpers improve readability more than any single optimization.
- Mark helpers with `t.Helper()` so failures point at the call site, not the helper.

## Heuristics

- "Where should this test go?" — Ask which layer's code you're testing. Domain rule → unit test in the domain package. SQL query → integration test in adapters. Whole HTTP→DB roundtrip → component test. Cross-service flow → E2E.
- "Is this test worth writing?" — If breaking the implementation would not cause this test to fail (or if fixing the implementation requires fixing the test verbatim), it's not worth writing.
- "Why is this test flaky?" — Almost always: (a) shared state between parallel tests, (b) a sleep instead of a wait-for-condition, or (c) ordering assumptions over a map iteration.

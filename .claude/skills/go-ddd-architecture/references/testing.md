# Testing

This skill's testing approach has four levels. The unifying principle: **every test runs in parallel, is deterministic, and never sleeps.**

The four levels and where they live:

| Level | Where in the tree | What it covers | Infra needed |
|-------|---|---|---|
| **Unit** | `domain/*/`, `app/command/`, `app/query/` | Pure logic, single function, single handler | None |
| **Integration** | `adapters/` | One adapter against its real backend | Docker DB (or other backing service) |
| **Component** | `tests/` (one per service) | Full service through HTTP/gRPC, external services mocked | Docker DB + the service running |
| **End-to-End** | Top-level `tests/` (one per project) | Multiple services together, only HTTP exposed | Full docker-compose |

You apply this as a "Christmas tree", not a strict pyramid: integration tests can outnumber unit tests in adapter-heavy services. Unit tests still dominate when you have a rich domain.

## The four properties of a good test

1. **Fast.** Whole suite under one minute locally. If something is slow, fix it — don't accept it.
2. **Tests enough scenarios at the right level.** Test corner cases at the cheapest level that can express them. Don't test "what happens when DB rolls back" with an E2E test.
3. **Robust and deterministic.** No flakes. A flake is a bug; fix it before it normalizes the whole team to retries.
4. **Locally runnable.** Don't depend on staging environments to know if your change works.

## Unit tests — domain

Domain code has no I/O, so the tests have no setup. Aim for high coverage of behaviors and error paths. Black-box style — package name ends in `_test`:

```go
// in domain/training/hour/hour_test.go
package hour_test

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "yourproject/internal/trainer/domain/hour"
)

func TestFactoryConfig_Validate(t *testing.T) {
    t.Parallel()

    testCases := []struct {
        Name        string
        Config      hour.FactoryConfig
        ExpectedErr string
    }{
        {
            Name: "valid",
            Config: hour.FactoryConfig{
                MaxWeeksInTheFutureToSet: 10,
                MinUtcHour:               10,
                MaxUtcHour:               12,
            },
        },
        {
            Name: "min_hour_after_max_hour",
            Config: hour.FactoryConfig{
                MaxWeeksInTheFutureToSet: 10,
                MinUtcHour:               14,
                MaxUtcHour:               12,
            },
            ExpectedErr: "min_utc_hour must be <= max_utc_hour",
        },
    }

    for _, c := range testCases {
        c := c // copy for parallel subtests
        t.Run(c.Name, func(t *testing.T) {
            t.Parallel()
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

Notes:

- `package hour_test` (the `_test` suffix) forces black-box testing — only exported API is reachable. If you cannot write a useful test from outside the package, your public API is wrong.
- `t.Parallel()` on both the parent and each subtest.
- `c := c` is the Go 1.21 fix for loop-variable capture in parallel subtests. Go 1.22+ doesn't need this (the spec changed). When unsure, write it — it's a one-line cost.

## Unit tests — application layer (commands)

Commands are mostly orchestration. The tests verify the right calls happen in the right order with the right arguments. Mocks are hand-written, tiny structs:

```go
// in app/command/cancel_training_test.go
package command_test

import (
    "context"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "yourproject/internal/trainings/app/command"
    "yourproject/internal/trainings/domain/training"
)

type repositoryMock struct {
    trainings map[string]training.Training
}

func (r *repositoryMock) UpdateTraining(
    ctx context.Context,
    uuid string,
    user training.User,
    updateFn func(ctx context.Context, tr *training.Training) (*training.Training, error),
) error {
    tr, ok := r.trainings[uuid]
    if !ok {
        return training.NotFoundError{TrainingUUID: uuid}
    }
    updated, err := updateFn(ctx, &tr)
    if err != nil {
        return err
    }
    r.trainings[uuid] = *updated
    return nil
}

// AddTraining, GetTraining: implement as needed (or panic if unused)

type trainerServiceMock struct {
    cancelled []time.Time
}

func (t *trainerServiceMock) CancelTraining(_ context.Context, when time.Time) error {
    t.cancelled = append(t.cancelled, when)
    return nil
}

func (t *trainerServiceMock) MoveTraining(_ context.Context, newTime, oldTime time.Time) error {
    return nil
}

type userServiceMock struct {
    balanceUpdates []balanceUpdate
}

type balanceUpdate struct {
    userUUID string
    delta    int
}

func (u *userServiceMock) UpdateTrainingBalance(_ context.Context, userUUID string, delta int) error {
    u.balanceUpdates = append(u.balanceUpdates, balanceUpdate{userUUID, delta})
    return nil
}

func TestCancelTraining(t *testing.T) {
    t.Parallel()

    requestingUserID := uuid.New().String()
    trainingUUID := uuid.New().String()

    testCases := []struct {
        Name                   string
        UserType               training.UserType
        TrainingConstructor    func() *training.Training
        ExpectedBalanceChange  int
    }{
        {
            Name:     "trainer_cancels_more_than_24h_ahead_returns_balance",
            UserType: training.Trainer,
            TrainingConstructor: func() *training.Training {
                return training.MustNewTraining(trainingUUID, "attendee-id", "Attendee", time.Now().Add(48*time.Hour))
            },
            ExpectedBalanceChange: 1,
        },
        {
            Name:     "trainer_cancels_under_24h_returns_balance_plus_penalty",
            UserType: training.Trainer,
            TrainingConstructor: func() *training.Training {
                return training.MustNewTraining(trainingUUID, "attendee-id", "Attendee", time.Now().Add(12*time.Hour))
            },
            ExpectedBalanceChange: 2,
        },
        {
            Name:     "attendee_cancels_under_24h_forfeits_training",
            UserType: training.Attendee,
            TrainingConstructor: func() *training.Training {
                return training.MustNewTraining(trainingUUID, requestingUserID, "Attendee", time.Now().Add(12*time.Hour))
            },
            ExpectedBalanceChange: 0,
        },
    }

    for _, tc := range testCases {
        tc := tc
        t.Run(tc.Name, func(t *testing.T) {
            t.Parallel()

            repo := &repositoryMock{
                trainings: map[string]training.Training{trainingUUID: *tc.TrainingConstructor()},
            }
            trainerSvc := &trainerServiceMock{}
            userSvc := &userServiceMock{}

            handler := command.NewCancelTrainingHandler(repo, userSvc, trainerSvc)

            err := handler.Handle(context.Background(), command.CancelTraining{
                TrainingUUID: trainingUUID,
                User:         training.MustNewUser(requestingUserID, tc.UserType),
            })
            require.NoError(t, err)

            assert.Len(t, trainerSvc.cancelled, 1, "trainer service should have been called once")

            if tc.ExpectedBalanceChange == 0 {
                assert.Empty(t, userSvc.balanceUpdates, "no balance change expected")
            } else {
                require.Len(t, userSvc.balanceUpdates, 1)
                assert.Equal(t, tc.ExpectedBalanceChange, userSvc.balanceUpdates[0].delta)
            }
        })
    }
}
```

If a test would just be "mock was called, with these args, mock returned this" without checking any actual outcome — skip it. You'd be testing the mock, not the code.

## `require` vs `assert`

- `require.*` halts the test on failure. Use it for prerequisites (the entity loaded, the err was nil) where continuing would just produce a cascade of nil-pointer panics or meaningless failures.
- `assert.*` continues. Use it for the actual checks where you want to see ALL failures at once for better debugging.

A common pattern:

```go
err := handler.Handle(ctx, cmd)
require.NoError(t, err)             // if it errored, stop

tr := repo.trainings[trainingUUID]
assert.True(t, tr.IsCanceled())     // check state
assert.Len(t, trainerSvc.calls, 1)  // check side effects
```

## Integration tests — testing one adapter

These verify your *code* uses the database correctly. They are NOT testing the database itself.

The standard pattern: same test suite runs against every implementation of the repository interface.

```go
// in adapters/hour_repository_test.go
package adapters_test

import (
    "math/rand"
    "sync"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "yourproject/internal/trainer/adapters"
    "yourproject/internal/trainer/domain/hour"
)

type Repository struct {
    Name string
    Repo hour.Repository
}

func createRepositories(t *testing.T) []Repository {
    return []Repository{
        {Name: "memory", Repo: adapters.NewMemoryHourRepository(testHourFactory)},
        {Name: "mysql",  Repo: adapters.NewMySQLHourRepository(newMySQLConnection(t), testHourFactory)},
        {Name: "firestore", Repo: adapters.NewFirestoreHourRepository(newFirestoreClient(t), testHourFactory)},
    }
}

func TestRepository(t *testing.T) {
    t.Parallel()
    rand.Seed(time.Now().UnixNano())

    repos := createRepositories(t)

    for i := range repos {
        r := repos[i] // scope for parallel subtests
        t.Run(r.Name, func(t *testing.T) {
            t.Parallel()

            t.Run("testUpdateHour", func(t *testing.T) {
                t.Parallel()
                testUpdateHour(t, r.Repo)
            })
            t.Run("testUpdateHour_parallel", func(t *testing.T) {
                t.Parallel()
                testUpdateHour_parallel(t, r.Repo)
            })
            t.Run("testUpdateHour_rollback", func(t *testing.T) {
                t.Parallel()
                testUpdateHour_rollback(t, r.Repo)
            })
        })
    }
}
```

### Generating unique IDs (so tests don't collide)

Every test creates objects with a fresh, unique key. No cleanup needed; nothing collides with anything else:

```go
var usedHours = sync.Map{}

func newValidHourTime() time.Time {
    for {
        minTime := time.Now().AddDate(0, 0, 1)
        minTS := minTime.Unix()
        maxTS := minTime.AddDate(0, 0, 30*7).Unix()
        t := time.Unix(rand.Int63n(maxTS-minTS)+minTS, 0).Truncate(time.Hour).Local()

        _, alreadyUsed := usedHours.LoadOrStore(t.Unix(), true)
        if !alreadyUsed {
            return t
        }
    }
}
```

Most aggregates can just use `uuid.New().String()`. The point is: tests are independent because they use different keys.

### Testing the `updateFn` rollback

This is the test that catches the silent-corruption bugs:

```go
func testUpdateHour_rollback(t *testing.T, repo hour.Repository) {
    t.Helper()
    ctx := context.Background()
    hourTime := newValidHourTime()

    // first call: make it available
    err := repo.UpdateHour(ctx, hourTime, func(h *hour.Hour) (*hour.Hour, error) {
        require.NoError(t, h.MakeAvailable())
        return h, nil
    })
    require.NoError(t, err)

    // second call: make it unavailable, then return an error
    err = repo.UpdateHour(ctx, hourTime, func(h *hour.Hour) (*hour.Hour, error) {
        assert.True(t, h.IsAvailable())
        require.NoError(t, h.MakeNotAvailable())
        return h, errors.New("something went wrong")
    })
    require.Error(t, err)

    // third call: hour should still be Available (the second call was rolled back)
    persisted, err := repo.GetOrCreateHour(ctx, hourTime)
    require.NoError(t, err)
    assert.True(t, persisted.IsAvailable(), "availability change was persisted, not rolled back")
}
```

If this test passes on your in-memory repo but fails on MySQL, you have a transaction bug — probably you're missing `defer tx.Rollback()` or you're returning before the commit/rollback path.

### Testing race conditions (concurrent writes)

The bug you most want to catch: two parallel calls both succeed when only one should have.

```go
func testUpdateHour_parallel(t *testing.T, repo hour.Repository) {
    t.Helper()
    ctx := context.Background()
    hourTime := newValidHourTime()

    // seed: make the hour available
    err := repo.UpdateHour(ctx, hourTime, func(h *hour.Hour) (*hour.Hour, error) {
        require.NoError(t, h.MakeAvailable())
        return h, nil
    })
    require.NoError(t, err)

    workersCount := 20
    workersDone := sync.WaitGroup{}
    workersDone.Add(workersCount)

    // a closed channel released to all workers simultaneously maximizes the
    // chance of an actual race.
    startWorkers := make(chan struct{})
    trainingsScheduled := make(chan int, workersCount)

    for worker := 0; worker < workersCount; worker++ {
        workerNum := worker
        go func() {
            defer workersDone.Done()
            <-startWorkers

            schedulingTraining := false
            err := repo.UpdateHour(ctx, hourTime, func(h *hour.Hour) (*hour.Hour, error) {
                if h.HasTrainingScheduled() {
                    return h, nil
                }
                if err := h.ScheduleTraining(); err != nil {
                    return nil, err
                }
                schedulingTraining = true
                return h, nil
            })
            if schedulingTraining && err == nil {
                trainingsScheduled <- workerNum
            }
        }()
    }

    close(startWorkers)
    workersDone.Wait()
    close(trainingsScheduled)

    var scheduled []int
    for w := range trainingsScheduled {
        scheduled = append(scheduled, w)
    }
    assert.Len(t, scheduled, 1, "exactly one worker should have scheduled the training, got %d: %v", len(scheduled), scheduled)
}
```

If you skip the `FOR UPDATE` lock (or the equivalent in your DB), this test will fail — multiple workers will succeed. The MySQL implementation in this skill uses `SELECT ... FOR UPDATE`; verify yours has the equivalent.

## The sabotage technique

When you're uncertain whether a test actually checks what you think it checks, deliberately break the implementation and confirm the test fails. If you break the code and the test still passes, the test is not testing what you thought.

For example, take a working transaction implementation:

```go
func (m MySQLHourRepository) finishTransaction(err error, tx *sqlx.Tx) error {
    if err != nil {
        if rollbackErr := tx.Rollback(); rollbackErr != nil {
            return multierr.Combine(err, rollbackErr)
        }
        return err
    }
    if commitErr := tx.Commit(); commitErr != nil {
        return fmt.Errorf("failed to commit tx: %w", commitErr)
    }
    return nil
}
```

Sabotage it — always commit, never roll back:

```go
func (m MySQLHourRepository) finishTransaction(err error, tx *sqlx.Tx) error {
    if commitErr := tx.Commit(); commitErr != nil {
        return fmt.Errorf("failed to commit tx: %w", commitErr)
    }
    return nil
}
```

Run the tests. If `testUpdateHour_rollback` still passes, your rollback test is broken — fix it. If it fails as expected, you can revert and trust the test.

This isn't a thing you automate. It's a thing you do once when you write a transaction test, and once again any time you doubt one.

## Never use `time.Sleep` in tests

Two reasons: it slows things down, and it eventually fails on a slow CI machine. Use one of:

- `sync.WaitGroup` for parallel coordination.
- Buffered channels as signals.
- `assert.Eventually(t, condition, waitFor, tick)` when you genuinely need to wait for an async outcome.

```go
assert.Eventually(t,
    func() bool {
        msg, err := subscriber.Receive(ctx)
        if err != nil { return false }
        return msg.UUID == expectedUUID
    },
    time.Second,         // max wait
    10*time.Millisecond, // check interval
)
```

`assert.Eventually` returns as soon as the condition is true. `time.Sleep(time.Second)` always takes a second, even when the result was ready in 5ms.

## Avoid length-based assertions on shared resources

If you `assert.Len(t, trainings, 1)` against a shared collection, another parallel test that adds a training will randomly fail your test. Two options:

1. Use unique IDs and assert that *your* ID is present (iterate and find):

   ```go
   var found bool
   for _, tr := range trainings {
       if tr.UUID == myUUID {
           found = true
           break
       }
   }
   assert.True(t, found)
   ```

2. Scope the test's data to a unique tenant/user/namespace (component tests do this with unique user UUIDs).

## Component tests

A component test runs your service end-to-end *internally*: the real ports, the real application, the real repository against Docker, but external services (other microservices) are mocked.

```go
// in tests/component_test.go
package tests_test

import (
    "context"
    "log"
    "os"
    "testing"

    "github.com/go-chi/chi/v5"

    "yourproject/internal/trainings/service"
    "yourproject/internal/trainings/ports"
    "yourproject/internal/common/server"
)

func TestMain(m *testing.M) {
    if !startService() {
        os.Exit(1)
    }
    os.Exit(m.Run())
}

func startService() bool {
    application := service.NewComponentTestApplication(context.Background())

    httpAddr := os.Getenv("TRAININGS_HTTP_ADDR")
    go server.RunHTTPServerOnAddr(httpAddr, func(router chi.Router) http.Handler {
        return ports.HandlerFromMux(ports.NewHttpServer(application), router)
    })

    ok := waitForPort(httpAddr)
    if !ok {
        log.Println("timed out waiting for HTTP server")
    }
    return ok
}
```

```go
// waiting helper — never use Sleep
func waitForPort(addr string) bool {
    deadline := time.Now().Add(10 * time.Second)
    for time.Now().Before(deadline) {
        conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
        if err == nil {
            conn.Close()
            return true
        }
        time.Sleep(50 * time.Millisecond) // OK here: this is bootstrap, not a test
    }
    return false
}
```

Wrap your client calls in test-friendly helpers:

```go
func (c TrainingsHTTPClient) CreateTraining(t *testing.T, note string, when time.Time) string {
    resp, err := c.client.CreateTrainingWithResponse(context.Background(), trainings.CreateTrainingJSONRequestBody{
        Notes: note, Time: when,
    })
    require.NoError(t, err)
    require.Equal(t, http.StatusNoContent, resp.StatusCode())
    contentLocation := resp.HTTPResponse.Header.Get("content-location")
    return lastPathElement(contentLocation)
}
```

The test then reads like a use case, not like HTTP plumbing:

```go
func TestTraining_Cancel(t *testing.T) {
    t.Parallel()
    client := newAttendeeClient(t)
    trainingUUID := client.CreateTraining(t, "yoga", tomorrow())

    client.CancelTraining(t, trainingUUID)

    trainings := client.GetTrainings(t)
    require.True(t, contains(trainings, trainingUUID))
    require.True(t, findByUUID(trainings, trainingUUID).Canceled)
}
```

**What to test at component level:** happy paths and the obvious failure paths a user might trigger (auth, not found, validation). Corner cases live in unit and integration tests.

## End-to-end tests

E2E uses the actual production binaries inside docker-compose. Multiple services together. Only public ports (HTTP).

```yaml
# docker-compose.test.yml
services:
  trainer:
    build: .
    environment: { ... }
  trainings:
    build: .
    environment: { ... }
  users:
    build: .
    environment: { ... }
  mysql: { image: mysql:8 }
  firestore: { image: gcr.io/... }
```

```go
// in tests/e2e_test.go (top-level project)

func TestE2E_Workflow(t *testing.T) {
    user := usersClient.GetCurrentUser(t)
    originalBalance := user.Balance

    _, err := usersGrpc.UpdateTrainingBalance(ctx, &users.UpdateTrainingBalanceRequest{
        UserId: userID, AmountChange: 1,
    })
    require.NoError(t, err)

    user = usersClient.GetCurrentUser(t)
    require.Equal(t, originalBalance+1, user.Balance)

    trainingUUID := trainingsClient.CreateTraining(t, "note", tomorrow())
    trainings := trainingsClient.GetTrainings(t)
    require.Equal(t, trainingUUID, findByUUID(trainings, trainingUUID).Uuid)

    user = usersClient.GetCurrentUser(t)
    require.Equal(t, originalBalance, user.Balance, "creating a training should consume the balance")
}
```

**Keep E2E tests short.** They're slow and brittle. They should test that services *connect correctly*, not that the logic inside them works (which is covered by component tests). Three or four tests covering the most important paths is enough.

## Build tags to separate Docker and non-Docker tests

```go
//go:build docker
```

at the top of integration / component tests. Then:

- `go test ./...` runs only unit tests (fast, no infra).
- `go test -tags=docker ./...` runs everything.

You can skip this if your suite stays under a minute. *Go with the Domain* eventually decided not to use build tags because their tests were fast enough together.

## A test you should NOT write

Tests that just duplicate the function they're testing add only noise. Example: a gRPC adapter method that calls the gRPC client with mapped arguments:

```go
func (s UsersGrpc) UpdateTrainingBalance(ctx context.Context, userID string, delta int) error {
    _, err := s.client.UpdateTrainingBalance(ctx, &users.UpdateTrainingBalanceRequest{
        UserId: userID, AmountChange: int64(delta),
    })
    return err
}
```

A test that mocks `s.client` and asserts the right `UpdateTrainingBalanceRequest` is reconstructed — that's the same code twice. If the mapping changes, you change two places. Skip it. The next layer up (component test) will catch genuine bugs.

## Cheatsheet

- `t.Parallel()` everywhere by default.
- `package <thing>_test` for black-box.
- Unique IDs (UUIDs, random+dedup'd values) instead of cleanup.
- `require` for prerequisites, `assert` for the checks you want all of.
- Table-driven tests for unit and integration.
- Hand-written mocks for the application layer; no mocking library needed.
- Sabotage to verify a test actually tests something.
- Never `time.Sleep` in a test; use channels, `WaitGroup`, or `assert.Eventually`.
- Don't assert on shared-resource list length; check for your specific item.
- Skip tests that duplicate the code they're testing.

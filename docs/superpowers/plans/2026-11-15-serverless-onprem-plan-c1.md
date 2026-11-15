# Serverless On-Prem — Plan C1 (job runtime) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- Run background jobs on Yandex Message Queue (YMQ) instead of River in serverless mode. This needs an enqueuer and the `POST /jobs` dispatcher a Message Queue trigger invokes.
- Replace `cmd/scheduler` and `cmd/consumer` with worker HTTP routes: `/cron/*` for the timer triggers and `/feedback` for the Data Streams trigger. Add `/admin/migrate` for the deploy step.

**Architecture:**
- **`internal/platform/ymq`.**
  - The `Enqueuer` sends one SQS message per job, `{"kind","args","not_before"}`, to the short or the long queue by kind. It implements every enqueuer port the serverless runtime wires.
  - The `Dispatcher` is an `http.Handler`. It decodes the trigger request and runs each message through the `jobs.Worker` registered with `ymq.AddWorker`. A snooze becomes a delayed copy of the message.
- **`internal/service.WorkerRoutes`.** The worker container's mux: `/jobs` (the dispatcher), `/cron/domain-verify-sweep`, `/cron/analytics-refresh`, `/feedback` and `/admin/migrate`. It lives in the composition-root package because `make lint-arch` forbids `internal/api` from importing YDB or adapter code.
- **Not started yet.** Nothing in C1 starts these handlers. Plan C4 wires them into `cmd/worker` once every context is on YDB. C1 adds files only and changes no existing Go file, so SaaS is untouched.

**Tech Stack:**
- Go 1.26
- `github.com/aws/aws-sdk-go-v2/service/sqs` **v1.42.27** (new; the release built on the `aws-sdk-go-v2` core v1.41.7 already in `go.mod`)
- River v0.37.1 (`river.JobArgs`, `rivertype.JobSnoozeError`)
- `ydb-go-sdk/v3` v3.136.4, local-ydb `26.3.1.23`
- testcontainers-go v0.42.0, fakecloud `ghcr.io/faiscadev/fakecloud:0.50.0`
- testify, `github.com/google/uuid`

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md). The binding sections are "Job runtime" (especially "YMQ enqueuer", "`POST /jobs`", "`POST /cron/:name`" and "`POST /feedback`"), "Testing" and "Plan C1 planning spike".

**Scope:** The first of four Plan C plans. C2 (single-tenant bootstrap), C3 (Terraform, images) and C4 (the serverless composition root, README, smoke test) follow. C1, C2 and C3 run in parallel with Plan B. Out of scope here:
- config keys (`NVELOPE_RUNTIME`, queue URLs, the tenant slug);
- `cmd/worker`'s serverless mode and the worker registrations;
- trigger, queue and retry settings, which belong to Terraform.

## Global Constraints

- **Branch.** Work in the worktree `/Users/nikthespirit/experiments/nvelope-wt/serverless-c1` on branch `feat/serverless-c1`, created from `main`. The main checkout and the `nvelope-wt/serverless-onprem` worktree belong to other sessions; do not touch them.
- **Docker.** Docker on this machine is OrbStack. Prefix every `go test` / `make test` / `make ci` with `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && `.
- **Shared test containers.** Test containers are reused by name across sessions: `nvelope-test-pg`, `nvelope-test-ydb`, and the new `nvelope-test-sqs`. Another session's run can reap or migrate one mid-run. Pg failures like `no migration found for version N` or `unexpected postmaster exit` followed by a burst of 0.00 s failures are environmental: re-run, and never `docker rm` a container.
- **YDB tests.** A cold local-ydb start takes about 45 s, so use `-timeout 8m` on runs that touch `internal/service`.
- **One new module, pinned.** Add exactly `github.com/aws/aws-sdk-go-v2/service/sqs@v1.42.27`. Never `@latest`: it upgrades the `aws-sdk-go-v2` core and `smithy-go` that the S3 and Postbox clients share. After `go mod tidy`, the only `go.mod` change is one new `require` line.
- **SaaS must not change.** C1 creates files only. It edits no existing `.go` file, so River, the pg adapters and every existing test keep their behaviour. `go test ./...` passes after every task.
- **fakecloud** (spec "Testing", "Plan C1 planning spike"):
  - run it with `FAKECLOUD_REGION=ru-central1` and `FAKECLOUD_ADDR=[::]:4566`;
  - readiness is `GET /_fakecloud/health`, and the credentials are `test`/`test`;
  - it rejects `DelaySeconds` above 900, as YMQ does;
  - it emulates no YC triggers, so tests drive the handler through a pump.
- **Never log job args.** `optin.send` and `auth.verification_send` carry raw tokens. Log the kind (inside the error) and the YMQ message id.
- **Commits.** Conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.** `goimports` is not installed, so use `gofmt -w`. `golangci-lint` is on PATH. `make lint-arch` needs `export PATH="$HOME/go/bin:$PATH" && `.

## File structure

| File | Responsibility |
|------|----------------|
| `internal/dbtest/sqs.go` | `SQSEndpoint(t)`: the shared fakecloud container, or `NVELOPE_TEST_SQS_ENDPOINT` |
| `internal/platform/ymq/enqueuer.go` | `NewClient`, the message `envelope`, kind→queue routing, `delaySeconds`, `Enqueuer` and its nine `Enqueue*` methods |
| `internal/platform/ymq/dispatcher.go` | `Dispatcher` (`POST /jobs`), `AddWorker`, the trigger request shape, the snooze and `not_before` re-send |
| `internal/platform/ymq/helpers_test.go` | per-test queue sets with DLQs, `receive` |
| `internal/platform/ymq/enqueuer_test.go` | port assertions, routing and envelope per kind, `delaySeconds` |
| `internal/platform/ymq/dispatcher_test.go` | the trigger pump and the dispatcher outcome table |
| `internal/service/worker_routes.go` | `WorkerRoutes`: the worker container mux, the cron sweeps, `/feedback`, `/admin/migrate` |
| `internal/service/worker_routes_test.go` | the routes against local YDB with recording fakes |

---

### Task 1: The YMQ enqueuer and the fakecloud test helper

**Files:**
- Modify: `go.mod`, `go.sum` (one new module)
- Create: `internal/dbtest/sqs.go`
- Create: `internal/platform/ymq/enqueuer.go`
- Test: `internal/platform/ymq/helpers_test.go`, `internal/platform/ymq/enqueuer_test.go`

**Interfaces:**
- Consumes (all unchanged):
  - the job arg types in `internal/platform/jobs/jobs.go`: `ImportArgs`, `ExportArgs`, `OptinSendArgs`, `VerificationSendArgs`, `CampaignStartArgs`, `CampaignBatchArgs`, `DomainVerifyArgs`, `FeedbackProcessArgs` and `AnalyticsRefreshArgs`, each with `Kind() string`;
  - the enqueuer ports it must satisfy:
    - `audiencecommand.JobEnqueuer` (`internal/audience/app/command/jobs.go`)
    - `audiencedomain.OptinEnqueuer` (`internal/audience/domain/pending_subscription.go`)
    - `authdomain.VerificationEnqueuer` (`internal/auth/domain/email_verification.go`)
    - `campaigncommand.CampaignEnqueuer` (`internal/campaign/app/command/campaigns.go`)
    - the unexported `batchEnqueuer` (`internal/campaign/adapters/start_worker.go`)
    - `sendingcommand.DomainVerifyEnqueuer` (`internal/sending/app/command/domains.go`)
    - `deliverabilitycommand.FeedbackEnqueuer` (`internal/deliverability/app/command/feedback.go`)
- Produces:
  ```go
  // package dbtest (internal/dbtest)
  const SQSRegion = "ru-central1"
  func SQSEndpoint(t *testing.T) string

  // package ymq (internal/platform/ymq)
  func NewClient(endpoint, region, accessKeyID, secretAccessKey string) *sqs.Client
  type Enqueuer struct{ /* unexported */ }
  func New(client *sqs.Client, queueURL, longQueueURL string) *Enqueuer
  func (e *Enqueuer) EnqueueImport(ctx context.Context, tenantID, jobID string) error
  func (e *Enqueuer) EnqueueExport(ctx context.Context, tenantID, jobID string) error
  func (e *Enqueuer) EnqueueOptinSend(ctx context.Context, tenantID, tenantSlug, pendingSubscriptionID, confirmationToken string) error
  func (e *Enqueuer) EnqueueVerificationSend(ctx context.Context, userID, rawToken string) error
  func (e *Enqueuer) EnqueueStart(ctx context.Context, tenantID, campaignID string) error
  func (e *Enqueuer) EnqueueBatch(ctx context.Context, tenantID, campaignID string, offset, limit int) error
  func (e *Enqueuer) EnqueueVerify(ctx context.Context, tenantID, domainID string) error
  func (e *Enqueuer) EnqueueFeedbackProcess(ctx context.Context, inboundEventID string) error
  func (e *Enqueuer) EnqueueAnalyticsRefresh(ctx context.Context, tenantID string) error
  // unexported, used by Task 2
  type envelope struct {
      Kind      string          `json:"kind"`
      Args      json.RawMessage `json:"args"`
      NotBefore time.Time       `json:"not_before,omitzero"`
  }
  const maxDelay = 900 * time.Second
  func (e *Enqueuer) send(ctx context.Context, env envelope, delay time.Duration) error
  func delaySeconds(d time.Duration) int32

  // package ymq test helpers (helpers_test.go), used by Task 2
  type testQueues struct {
      client            *sqs.Client
      short, long       string // queue URLs
      shortDLQ, longDLQ string
      enqueuer          *Enqueuer
  }
  func newTestQueues(t *testing.T) testQueues
  func createQueue(t *testing.T, client *sqs.Client, name, dlqURL string) string
  func receive(t *testing.T, client *sqs.Client, queueURL string, wait time.Duration) *types.Message
  ```
- Notes:
  - `EnqueueVerifyUnique`, `EnqueueBillingSweep`, `EnqueueBillingCharge`, `EnqueueUsageRollup` and `EnqueueVerificationCleanup` are deliberately absent: those kinds are not wired in serverless mode (spec "YMQ enqueuer", V3 table), and the domain sweep uses plain `EnqueueVerify` (Task 3).
  - The long queue takes `audience.import`, `audience.export`, `campaign.start` and `campaign.batch`. Every other kind goes to the short queue.

- [ ] **Step 1: Add the SQS client module**

Run:

```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c1
go get github.com/aws/aws-sdk-go-v2/service/sqs@v1.42.27
```

Expected: `go: added github.com/aws/aws-sdk-go-v2/service/sqs v1.42.27` and nothing upgraded. If any `upgraded` line appears, stop: the version is wrong. Step 6 runs `go mod tidy` once code imports the module.

- [ ] **Step 2: Add the fakecloud test helper**

Create `internal/dbtest/sqs.go`:

```go
package dbtest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// sqsContainerName is the fixed name of the shared fakecloud container that
// stands in for Yandex Message Queue (which speaks the SQS API). Like
// nvelope-test-ydb, testcontainers' reuse feature keys on this name.
const sqsContainerName = "nvelope-test-sqs"

// fakecloudImage pins the fakecloud release whose SQS semantics were verified
// against YMQ: per-message DelaySeconds capped at 900, redelivery after the
// visibility timeout, ApproximateReceiveCount and RedrivePolicy dead-lettering.
const fakecloudImage = "ghcr.io/faiscadev/fakecloud:0.50.0"

// SQSRegion is the region the test queues live in. Queues are regional and
// the SDK signs for the region, so clients must use the same value.
const SQSRegion = "ru-central1"

var (
	sqsOnce     sync.Once
	sqsEndpoint string
	sqsErr      error
)

// SQSEndpoint resolves an SQS endpoint for integration tests. If
// NVELOPE_TEST_SQS_ENDPOINT is set it is used verbatim; otherwise a fakecloud
// container is started — or an existing one reused — via testcontainers-go.
// The credentials are fakecloud's root identity, "test"/"test". Tests create
// their own uniquely named queues, so they never share state.
func SQSEndpoint(t *testing.T) string {
	t.Helper()
	sqsOnce.Do(func() {
		if env := os.Getenv("NVELOPE_TEST_SQS_ENDPOINT"); env != "" {
			sqsEndpoint = env
			return
		}
		sqsEndpoint, sqsErr = startSQSContainer()
	})
	require.NoError(t, sqsErr)
	return sqsEndpoint
}

func startSQSContainer() (string, error) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        fakecloudImage,
			ExposedPorts: []string{"4566/tcp"},
			Env: map[string]string{
				// The default IPv4-only bind breaks testcontainers' "localhost"
				// through the IPv6 port mapping.
				"FAKECLOUD_ADDR":   "[::]:4566",
				"FAKECLOUD_REGION": SQSRegion,
			},
			WaitingFor: wait.ForHTTP("/_fakecloud/health").
				WithPort("4566/tcp").
				WithStartupTimeout(90 * time.Second),
			Name: sqsContainerName,
		},
		Started: true,
		Reuse:   true,
	})
	if err != nil {
		return "", fmt.Errorf("SQS integration tests require Docker: %w", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving fakecloud host: %w", err)
	}
	port, err := container.MappedPort(ctx, "4566/tcp")
	if err != nil {
		return "", fmt.Errorf("resolving fakecloud port: %w", err)
	}
	return fmt.Sprintf("http://%s:%s", host, port.Port()), nil
}
```

- [ ] **Step 3: Write the failing tests**

Create `internal/platform/ymq/helpers_test.go`:

```go
package ymq

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// testQueues is one isolated set of queues: the short and long job queues,
// each with its own dead-letter queue. A message is dead-lettered on its third
// receive (maxReceiveCount 2), and an unacknowledged message reappears after
// one second (visibility timeout 1).
type testQueues struct {
	client            *sqs.Client
	short, long       string
	shortDLQ, longDLQ string
	enqueuer          *Enqueuer
}

func newTestQueues(t *testing.T) testQueues {
	t.Helper()
	client := NewClient(dbtest.SQSEndpoint(t), dbtest.SQSRegion, "test", "test")
	prefix := "t" + uuid.NewString()[:8]
	q := testQueues{client: client}
	q.shortDLQ = createQueue(t, client, prefix+"-jobs-dlq", "")
	q.longDLQ = createQueue(t, client, prefix+"-jobs-long-dlq", "")
	q.short = createQueue(t, client, prefix+"-jobs", q.shortDLQ)
	q.long = createQueue(t, client, prefix+"-jobs-long", q.longDLQ)
	q.enqueuer = New(client, q.short, q.long)
	return q
}

// createQueue creates a queue and returns its URL. A non-empty dlqURL gets a
// redrive policy onto that queue.
func createQueue(t *testing.T, client *sqs.Client, name, dlqURL string) string {
	t.Helper()
	ctx := context.Background()
	attrs := map[string]string{"VisibilityTimeout": "1"}
	if dlqURL != "" {
		out, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
			QueueUrl:       aws.String(dlqURL),
			AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameQueueArn},
		})
		require.NoError(t, err)
		attrs["RedrivePolicy"] = fmt.Sprintf(`{"deadLetterTargetArn":%q,"maxReceiveCount":"2"}`,
			out.Attributes[string(types.QueueAttributeNameQueueArn)])
	}
	out, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName:  aws.String(name),
		Attributes: attrs,
	})
	require.NoError(t, err)
	return *out.QueueUrl
}

// receive waits up to wait for one message on queueURL and returns it, or nil
// when none arrived. The message is left on the queue: it reappears after the
// visibility timeout unless the caller deletes it.
func receive(t *testing.T, client *sqs.Client, queueURL string, wait time.Duration) *types.Message {
	t.Helper()
	deadline := time.Now().Add(wait)
	for {
		out, err := client.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     1,
		})
		require.NoError(t, err)
		if len(out.Messages) > 0 {
			return &out.Messages[0]
		}
		if time.Now().After(deadline) {
			return nil
		}
	}
}
```

Create `internal/platform/ymq/enqueuer_test.go`:

```go
package ymq

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	authdomain "github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	campaigncommand "github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	deliverabilitycommand "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	sendingcommand "github.com/nikolaymatrosov/nvelope/internal/sending/app/command"
)

// The Enqueuer must satisfy every enqueuer port the serverless runtime wires.
// The campaign start worker's batch enqueuer is unexported, so its one method
// is checked through a local interface.
var (
	_ audiencecommand.JobEnqueuer            = (*Enqueuer)(nil)
	_ audiencedomain.OptinEnqueuer           = (*Enqueuer)(nil)
	_ authdomain.VerificationEnqueuer        = (*Enqueuer)(nil)
	_ campaigncommand.CampaignEnqueuer       = (*Enqueuer)(nil)
	_ sendingcommand.DomainVerifyEnqueuer    = (*Enqueuer)(nil)
	_ deliverabilitycommand.FeedbackEnqueuer = (*Enqueuer)(nil)
	_ interface {
		EnqueueBatch(ctx context.Context, tenantID, campaignID string, offset, limit int) error
	} = (*Enqueuer)(nil)
)

func TestEnqueuerRoutesEachKindWithItsArgs(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	ctx := context.Background()
	e := q.enqueuer

	cases := []struct {
		enqueue func() error
		long    bool
		want    river.JobArgs
	}{
		{func() error { return e.EnqueueImport(ctx, "t1", "j1") }, true,
			jobs.ImportArgs{TenantID: "t1", JobID: "j1"}},
		{func() error { return e.EnqueueExport(ctx, "t1", "j2") }, true,
			jobs.ExportArgs{TenantID: "t1", JobID: "j2"}},
		{func() error { return e.EnqueueStart(ctx, "t1", "c1") }, true,
			jobs.CampaignStartArgs{TenantID: "t1", CampaignID: "c1"}},
		{func() error { return e.EnqueueBatch(ctx, "t1", "c1", 500, 250) }, true,
			jobs.CampaignBatchArgs{TenantID: "t1", CampaignID: "c1", Offset: 500, Limit: 250}},
		{func() error { return e.EnqueueOptinSend(ctx, "t1", "acme", "p1", "raw-tok") }, false,
			jobs.OptinSendArgs{TenantID: "t1", TenantSlug: "acme", PendingSubscriptionID: "p1", ConfirmationToken: "raw-tok"}},
		{func() error { return e.EnqueueVerificationSend(ctx, "u1", "raw-tok") }, false,
			jobs.VerificationSendArgs{UserID: "u1", VerificationToken: "raw-tok"}},
		{func() error { return e.EnqueueVerify(ctx, "t1", "d1") }, false,
			jobs.DomainVerifyArgs{TenantID: "t1", DomainID: "d1"}},
		{func() error { return e.EnqueueFeedbackProcess(ctx, "ev1") }, false,
			jobs.FeedbackProcessArgs{InboundEventID: "ev1"}},
		{func() error { return e.EnqueueAnalyticsRefresh(ctx, "t1") }, false,
			jobs.AnalyticsRefreshArgs{TenantID: "t1"}},
	}
	for _, tc := range cases {
		t.Run(tc.want.Kind(), func(t *testing.T) {
			require.NoError(t, tc.enqueue())
			queueURL := q.short
			if tc.long {
				queueURL = q.long
			}
			msg := receive(t, q.client, queueURL, 5*time.Second)
			require.NotNil(t, msg, "no message on the expected queue (long=%v)", tc.long)
			// Delete it, or it reappears after the visibility timeout and a
			// later case on the same queue receives it instead of its own.
			_, err := q.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl: aws.String(queueURL), ReceiptHandle: msg.ReceiptHandle,
			})
			require.NoError(t, err)

			var env envelope
			require.NoError(t, json.Unmarshal([]byte(*msg.Body), &env))
			require.Equal(t, tc.want.Kind(), env.Kind)
			require.True(t, env.NotBefore.IsZero())
			wantArgs, err := json.Marshal(tc.want)
			require.NoError(t, err)
			require.JSONEq(t, string(wantArgs), string(env.Args))
		})
	}
}

func TestDelaySecondsRoundsUpAndCaps(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   time.Duration
		want int32
	}{
		{-time.Second, 0},
		{0, 0},
		{400 * time.Millisecond, 1},
		{2 * time.Second, 2},
		{2*time.Second + time.Millisecond, 3},
		{900 * time.Second, 900},
		{20 * time.Minute, 900},
	} {
		require.Equal(t, tc.want, delaySeconds(tc.in), "delaySeconds(%s)", tc.in)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/platform/ymq/ -count=1`

Expected: FAIL to build, with `undefined: Enqueuer`, `undefined: NewClient`, `undefined: New`, `undefined: envelope` and `undefined: delaySeconds`.

- [ ] **Step 5: Write the enqueuer**

Create `internal/platform/ymq/enqueuer.go`:

```go
// Package ymq runs background jobs on Yandex Message Queue in serverless mode.
// The Enqueuer sends one message per job; the Dispatcher is the HTTP handler a
// Message Queue trigger invokes with each message. SaaS mode uses River
// (internal/platform/jobs) instead. Both run the same jobs.Worker values.
package ymq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/riverqueue/river"

	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// NewClient builds an SQS API client for the YMQ endpoint
// (https://message-queue.api.cloud.yandex.net, region ru-central1) with a
// static access key.
func NewClient(endpoint, region, accessKeyID, secretAccessKey string) *sqs.Client {
	return sqs.New(sqs.Options{
		Region:       region,
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	})
}

// envelope is the body of every job message.
type envelope struct {
	Kind string          `json:"kind"`
	Args json.RawMessage `json:"args"`
	// NotBefore, when set, is the earliest time the job may run. A snooze sets
	// it. When it is more than maxDelay away, the message reappears early and
	// the dispatcher re-sends it until it is due.
	NotBefore time.Time `json:"not_before,omitzero"`
}

// maxDelay is the longest per-message delay YMQ accepts (DelaySeconds 900).
const maxDelay = 900 * time.Second

// longKinds are the job kinds that may run for minutes. They go to the long
// queue, whose visibility timeout covers the worker container's timeout.
var longKinds = map[string]bool{
	jobs.ImportArgs{}.Kind():        true,
	jobs.ExportArgs{}.Kind():        true,
	jobs.CampaignStartArgs{}.Kind(): true,
	jobs.CampaignBatchArgs{}.Kind(): true,
}

// Enqueuer sends job messages to YMQ. It implements every enqueuer port the
// serverless runtime wires: audience, auth, campaign, sending, deliverability,
// and the cron handlers' analytics refresh.
type Enqueuer struct {
	client       *sqs.Client
	queueURL     string
	longQueueURL string
}

// New builds an Enqueuer that sends to queueURL, or to longQueueURL for the
// long-running kinds.
func New(client *sqs.Client, queueURL, longQueueURL string) *Enqueuer {
	if client == nil || queueURL == "" || longQueueURL == "" {
		panic("ymq: nil client or empty queue URL")
	}
	return &Enqueuer{client: client, queueURL: queueURL, longQueueURL: longQueueURL}
}

func (e *Enqueuer) enqueue(ctx context.Context, args river.JobArgs) error {
	raw, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("encoding %s job args: %w", args.Kind(), err)
	}
	return e.send(ctx, envelope{Kind: args.Kind(), Args: raw}, 0)
}

// send puts env on its kind's queue, hidden for delay (capped at maxDelay).
func (e *Enqueuer) send(ctx context.Context, env envelope, delay time.Duration) error {
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("encoding %s job message: %w", env.Kind, err)
	}
	queueURL := e.queueURL
	if longKinds[env.Kind] {
		queueURL = e.longQueueURL
	}
	_, err = e.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:     aws.String(queueURL),
		MessageBody:  aws.String(string(body)),
		DelaySeconds: delaySeconds(delay),
	})
	if err != nil {
		return fmt.Errorf("enqueuing %s job: %w", env.Kind, err)
	}
	return nil
}

// delaySeconds rounds d up to whole seconds and caps it at maxDelay. Rounding
// up keeps a message from reappearing before its NotBefore.
func delaySeconds(d time.Duration) int32 {
	if d <= 0 {
		return 0
	}
	d = min(d, maxDelay)
	return int32((d + time.Second - 1) / time.Second)
}

// EnqueueImport enqueues a bulk-import job.
func (e *Enqueuer) EnqueueImport(ctx context.Context, tenantID, jobID string) error {
	return e.enqueue(ctx, jobs.ImportArgs{TenantID: tenantID, JobID: jobID})
}

// EnqueueExport enqueues a bulk-export job.
func (e *Enqueuer) EnqueueExport(ctx context.Context, tenantID, jobID string) error {
	return e.enqueue(ctx, jobs.ExportArgs{TenantID: tenantID, JobID: jobID})
}

// EnqueueOptinSend enqueues a double-opt-in confirmation send.
func (e *Enqueuer) EnqueueOptinSend(ctx context.Context, tenantID, tenantSlug,
	pendingSubscriptionID, confirmationToken string) error {
	return e.enqueue(ctx, jobs.OptinSendArgs{
		TenantID:              tenantID,
		TenantSlug:            tenantSlug,
		PendingSubscriptionID: pendingSubscriptionID,
		ConfirmationToken:     confirmationToken,
	})
}

// EnqueueVerificationSend enqueues a registration email-verification send.
func (e *Enqueuer) EnqueueVerificationSend(ctx context.Context, userID, rawToken string) error {
	return e.enqueue(ctx, jobs.VerificationSendArgs{UserID: userID, VerificationToken: rawToken})
}

// EnqueueStart enqueues the start of a campaign send.
func (e *Enqueuer) EnqueueStart(ctx context.Context, tenantID, campaignID string) error {
	return e.enqueue(ctx, jobs.CampaignStartArgs{TenantID: tenantID, CampaignID: campaignID})
}

// EnqueueBatch enqueues one campaign batch over the recipient slice
// [offset, offset+limit).
func (e *Enqueuer) EnqueueBatch(ctx context.Context, tenantID, campaignID string, offset, limit int) error {
	return e.enqueue(ctx, jobs.CampaignBatchArgs{
		TenantID: tenantID, CampaignID: campaignID, Offset: offset, Limit: limit,
	})
}

// EnqueueVerify enqueues a sending-domain verification poll.
func (e *Enqueuer) EnqueueVerify(ctx context.Context, tenantID, domainID string) error {
	return e.enqueue(ctx, jobs.DomainVerifyArgs{TenantID: tenantID, DomainID: domainID})
}

// EnqueueFeedbackProcess enqueues processing of one staged inbound
// notification.
func (e *Enqueuer) EnqueueFeedbackProcess(ctx context.Context, inboundEventID string) error {
	return e.enqueue(ctx, jobs.FeedbackProcessArgs{InboundEventID: inboundEventID})
}

// EnqueueAnalyticsRefresh enqueues an analytics refresh for one tenant. Unlike
// River's unique insert, a duplicate is not suppressed: the refresh is an
// idempotent recompute.
func (e *Enqueuer) EnqueueAnalyticsRefresh(ctx context.Context, tenantID string) error {
	return e.enqueue(ctx, jobs.AnalyticsRefreshArgs{TenantID: tenantID})
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run:

```bash
go mod tidy && gofmt -l internal/ && go vet ./internal/platform/ymq/ ./internal/dbtest/ \
  && export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" \
  && go test ./internal/platform/ymq/ -count=1 -race -v
```

Expected:
- `gofmt -l` prints nothing;
- `PASS` for `TestEnqueuerRoutesEachKindWithItsArgs` (nine subtests, one per kind) and `TestDelaySecondsRoundsUpAndCaps`;
- `git diff go.mod` shows exactly one added line, `github.com/aws/aws-sdk-go-v2/service/sqs v1.42.27`, in the direct `require` block.

The first run pulls the fakecloud image, which takes a few seconds.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/dbtest/sqs.go internal/platform/ymq/
git commit -m "feat(ymq): add the YMQ job enqueuer and the fakecloud SQS test helper"
```

---

### Task 2: The `POST /jobs` dispatcher

**Files:**
- Create: `internal/platform/ymq/dispatcher.go`
- Test: `internal/platform/ymq/dispatcher_test.go`

**Interfaces:**
- Consumes:
  - from Task 1: `Enqueuer`, `envelope`, `(*Enqueuer).send`, `testQueues`, `newTestQueues`, `receive`;
  - `jobs.Worker[T]` (`internal/platform/jobs/worker.go`): `Work(ctx context.Context, args T) error`;
  - `river.JobSnooze(d)`, which returns `*rivertype.JobSnoozeError{Duration: d}`.
- Produces (Plan C4 uses these to build the worker):
  ```go
  // package ymq
  type Dispatcher struct{ /* unexported */ }
  func NewDispatcher(enq *Enqueuer, timeout time.Duration, logger *slog.Logger) *Dispatcher
  func AddWorker[T river.JobArgs](d *Dispatcher, w jobs.Worker[T])
  func (d *Dispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request)
  ```
- Behaviour (spec "`POST /jobs`" outcome table):

  | Outcome | Action | HTTP |
  |---------|--------|------|
  | the job returns `nil` | — | 200 (the trigger deletes the message) |
  | the job returns `river.JobSnooze(d)` | send a copy with `not_before = now + d` and `DelaySeconds = delaySeconds(d)` (capped at 900) | 200 |
  | `not_before` is in the future | re-send unchanged with the remaining delay, capped at 900 | 200, the job does not run |
  | any other error; unknown kind; undecodable body, args or trigger request; the job exceeds `timeout` | log `message_id` and the error | 500 (redelivered after the visibility timeout, then the DLQ) |

- [ ] **Step 1: Write the failing tests**

Create `internal/platform/ymq/dispatcher_test.go`:

```go
package ymq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// workerFunc adapts a function to jobs.Worker.
type workerFunc[T river.JobArgs] func(context.Context, T) error

func (f workerFunc[T]) Work(ctx context.Context, args T) error { return f(ctx, args) }

// serve POSTs one message body to h wrapped the way a YC Message Queue trigger
// wraps it, and returns the status.
func serve(t *testing.T, h http.Handler, messageID, body string) int {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"messages": []any{map[string]any{
		"event_metadata": map[string]any{
			"event_id":   messageID,
			"event_type": "yandex.cloud.events.messagequeue.QueueMessage",
			"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		},
		"details": map[string]any{
			"queue_id": "yrn:yc:ymq:ru-central1:test:jobs",
			"message": map[string]any{
				"message_id": messageID,
				"body":       body,
				"attributes": map[string]any{"SentTimestamp": "1569285804456"},
			},
		},
	}}})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader(payload)))
	return rec.Code
}

// pump stands in for the trigger, which fakecloud does not emulate: it
// receives one message (waiting up to wait), serves it to h, and deletes it on
// 200. Any other status leaves the message to reappear after the visibility
// timeout. It returns 0 when no message arrived.
func pump(t *testing.T, q testQueues, queueURL string, h http.Handler, wait time.Duration) int {
	t.Helper()
	msg := receive(t, q.client, queueURL, wait)
	if msg == nil {
		return 0
	}
	code := serve(t, h, *msg.MessageId, *msg.Body)
	if code == http.StatusOK {
		_, err := q.client.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
			QueueUrl: aws.String(queueURL), ReceiptHandle: msg.ReceiptHandle,
		})
		require.NoError(t, err)
	}
	return code
}

func newTestDispatcher(q testQueues) *Dispatcher {
	return NewDispatcher(q.enqueuer, time.Minute, slog.New(slog.DiscardHandler))
}

func TestDispatcherRunsTheJobAndAcknowledgesIt(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := newTestDispatcher(q)
	var got []jobs.DomainVerifyArgs
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(_ context.Context, a jobs.DomainVerifyArgs) error {
		got = append(got, a)
		return nil
	}))

	require.NoError(t, q.enqueuer.EnqueueVerify(context.Background(), "t1", "d1"))
	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Equal(t, []jobs.DomainVerifyArgs{{TenantID: "t1", DomainID: "d1"}}, got)
	// Acknowledged: nothing reappears after the visibility timeout.
	require.Zero(t, pump(t, q, q.short, d, 2*time.Second))
}

func TestDispatcherSnoozeSendsADelayedCopy(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := newTestDispatcher(q)
	var runs []time.Time
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(context.Context, jobs.DomainVerifyArgs) error {
		runs = append(runs, time.Now())
		if len(runs) == 1 {
			return river.JobSnooze(2 * time.Second)
		}
		return nil
	}))

	require.NoError(t, q.enqueuer.EnqueueVerify(context.Background(), "t1", "d1"))
	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Zero(t, pump(t, q, q.short, d, time.Second), "the copy must stay hidden for the snooze")
	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Len(t, runs, 2)
	require.GreaterOrEqual(t, runs[1].Sub(runs[0]), 2*time.Second)
}

func TestDispatcherCapsALongSnoozeAtTheMaximumDelay(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := newTestDispatcher(q)
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(context.Context, jobs.DomainVerifyArgs) error {
		return river.JobSnooze(20 * time.Minute)
	}))

	require.NoError(t, q.enqueuer.EnqueueVerify(context.Background(), "t1", "d1"))
	// YMQ rejects DelaySeconds above 900, so an uncapped copy would fail to
	// send and the dispatcher would answer 500.
	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Zero(t, pump(t, q, q.short, d, time.Second))
}

func TestDispatcherHoldsAMessageUntilItsNotBefore(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := newTestDispatcher(q)
	var ranAt []time.Time
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(context.Context, jobs.DomainVerifyArgs) error {
		ranAt = append(ranAt, time.Now())
		return nil
	}))

	// A message that arrives early: the shape of a copy whose snooze exceeded
	// the maximum delay.
	notBefore := time.Now().Add(2 * time.Second)
	require.NoError(t, q.enqueuer.send(context.Background(), envelope{
		Kind:      jobs.DomainVerifyArgs{}.Kind(),
		Args:      json.RawMessage(`{"tenant_id":"t1","domain_id":"d1"}`),
		NotBefore: notBefore,
	}, 0))

	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Empty(t, ranAt, "an early message is re-sent, not run")
	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Len(t, ranAt, 1)
	require.False(t, ranAt[0].Before(notBefore))
}

func TestDispatcherFailureIsRedeliveredThenDeadLettered(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := newTestDispatcher(q)
	calls := 0
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(context.Context, jobs.DomainVerifyArgs) error {
		calls++
		return errors.New("postbox unavailable")
	}))

	require.NoError(t, q.enqueuer.EnqueueVerify(context.Background(), "t1", "d1"))
	require.Equal(t, http.StatusInternalServerError, pump(t, q, q.short, d, 5*time.Second))
	require.Equal(t, http.StatusInternalServerError, pump(t, q, q.short, d, 5*time.Second),
		"redelivered after the visibility timeout")
	require.Zero(t, pump(t, q, q.short, d, 3*time.Second), "the third receive dead-letters it")
	require.NotNil(t, receive(t, q.client, q.shortDLQ, 5*time.Second))
	require.Equal(t, 2, calls)
}

func TestDispatcherRejectsWhatItCannotRun(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := NewDispatcher(q.enqueuer, 50*time.Millisecond, slog.New(slog.DiscardHandler))
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(ctx context.Context, _ jobs.DomainVerifyArgs) error {
		<-ctx.Done() // runs until the job timeout
		return ctx.Err()
	}))

	require.Equal(t, http.StatusInternalServerError, serve(t, d, "m1", "not json"))
	require.Equal(t, http.StatusInternalServerError, serve(t, d, "m2", `{"kind":"no.such.kind","args":{}}`))
	require.Equal(t, http.StatusInternalServerError,
		serve(t, d, "m3", `{"kind":"domain.verify","args":{"tenant_id":42}}`), "undecodable args")
	require.Equal(t, http.StatusInternalServerError,
		serve(t, d, "m4", `{"kind":"domain.verify","args":{"tenant_id":"t1","domain_id":"d1"}}`),
		"a job past its timeout")

	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader([]byte("{"))))
	require.Equal(t, http.StatusInternalServerError, rec.Code, "an undecodable trigger request")

	require.Panics(t, func() {
		AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(context.Context, jobs.DomainVerifyArgs) error { return nil }))
	})
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/platform/ymq/ -count=1`

Expected: FAIL to build, with `undefined: Dispatcher`, `undefined: NewDispatcher` and `undefined: AddWorker`.

- [ ] **Step 3: Write the dispatcher**

Create `internal/platform/ymq/dispatcher.go`:

```go
package ymq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// Dispatcher is the POST /jobs handler a YC Message Queue trigger invokes. It
// runs the job each message carries through the worker registered for its
// kind. A 200 makes the trigger delete the message; any other status makes it
// reappear after the queue's visibility timeout, and after maxReceiveCount
// deliveries the redrive policy moves it to the dead-letter queue.
type Dispatcher struct {
	enq      *Enqueuer
	timeout  time.Duration
	logger   *slog.Logger
	handlers map[string]func(context.Context, json.RawMessage) error
}

// NewDispatcher builds a Dispatcher. enq re-sends snoozed and not-yet-due
// messages. timeout bounds one job run: set it to the container's execution
// timeout minus a safety margin, so a long batch stops cleanly (and is
// redelivered) before the platform kills the invocation.
func NewDispatcher(enq *Enqueuer, timeout time.Duration, logger *slog.Logger) *Dispatcher {
	if enq == nil || logger == nil || timeout <= 0 {
		panic("ymq: nil dependency or non-positive timeout")
	}
	return &Dispatcher{
		enq:      enq,
		timeout:  timeout,
		logger:   logger,
		handlers: map[string]func(context.Context, json.RawMessage) error{},
	}
}

// AddWorker registers w under T's kind. It mirrors jobs.AddWorker, so the
// serverless worker registers the same worker values SaaS gives River. Like
// river.AddWorker, it panics when the kind is already registered.
func AddWorker[T river.JobArgs](d *Dispatcher, w jobs.Worker[T]) {
	var zero T
	kind := zero.Kind()
	if _, dup := d.handlers[kind]; dup {
		panic("ymq: a worker is already registered for kind " + kind)
	}
	d.handlers[kind] = func(ctx context.Context, raw json.RawMessage) error {
		var args T
		if err := json.Unmarshal(raw, &args); err != nil {
			return fmt.Errorf("decoding %s job args: %w", kind, err)
		}
		return w.Work(ctx, args)
	}
}

// triggerRequest is the part of a Message Queue trigger request the
// dispatcher reads. The full shape is documented at
// https://yandex.cloud/en/docs/serverless-containers/concepts/trigger/ymq-trigger#format.
type triggerRequest struct {
	Messages []struct {
		Details struct {
			Message struct {
				MessageID string `json:"message_id"`
				Body      string `json:"body"`
			} `json:"message"`
		} `json:"details"`
	} `json:"messages"`
}

// ServeHTTP runs every message in the trigger batch.
func (d *Dispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req triggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		d.logger.Error("decoding the message queue trigger request", "error", err)
		http.Error(w, "undecodable trigger request", http.StatusInternalServerError)
		return
	}
	// ponytail: the trigger is configured with batch size 1. With a larger
	// batch, one failure redelivers the whole batch, including messages that
	// already succeeded.
	for _, m := range req.Messages {
		if err := d.run(r.Context(), m.Details.Message.Body); err != nil {
			// The args are not logged: some carry raw verification and
			// confirmation tokens.
			d.logger.Error("job failed", "message_id", m.Details.Message.MessageID, "error", err)
			http.Error(w, "job failed", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// run handles one message body. A nil return lets the trigger delete the
// message: the job finished, or a copy holding its remaining wait was sent.
func (d *Dispatcher) run(ctx context.Context, body string) error {
	var env envelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		return fmt.Errorf("decoding job message: %w", err)
	}
	if wait := time.Until(env.NotBefore); wait > 0 {
		return d.enq.send(ctx, env, wait)
	}
	handle, ok := d.handlers[env.Kind]
	if !ok {
		return fmt.Errorf("no worker registered for job kind %q", env.Kind)
	}
	jobCtx, cancel := context.WithTimeout(ctx, d.timeout)
	err := handle(jobCtx, env.Args)
	cancel()

	var snooze *rivertype.JobSnoozeError
	if errors.As(err, &snooze) {
		env.NotBefore = time.Now().Add(snooze.Duration)
		return d.enq.send(ctx, env, snooze.Duration)
	}
	if err != nil {
		return fmt.Errorf("%s job: %w", env.Kind, err)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run:

```bash
gofmt -l internal/platform/ymq && go vet ./internal/platform/ymq/ \
  && export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" \
  && go test ./internal/platform/ymq/ -count=1 -race -v
```

Expected: PASS for all eight tests in the package; the package takes about 7 s, dominated by the visibility-timeout and delay waits. If `TestDispatcherFailureIsRedeliveredThenDeadLettered` fails on the third pump, check that `createQueue` set `maxReceiveCount` `"2"` and `VisibilityTimeout` `"1"`.

- [ ] **Step 5: Sabotage check**

The cap is the one line a refactor could silently drop. Temporarily delete `d = min(d, maxDelay)` from `delaySeconds` in `enqueuer.go` and run:

`export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/platform/ymq/ -count=1 -run 'Caps'`

Expected: FAIL in both `TestDelaySecondsRoundsUpAndCaps` and `TestDispatcherCapsALongSnoozeAtTheMaximumDelay` (fakecloud rejects `DelaySeconds` 1200, so the dispatcher answers 500). Restore the line and re-run to PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/ymq/dispatcher.go internal/platform/ymq/dispatcher_test.go
git commit -m "feat(ymq): add the Message Queue trigger dispatcher for serverless jobs"
```

---

### Task 3: The serverless worker routes

**Files:**
- Create: `internal/service/worker_routes.go`
- Test: `internal/service/worker_routes_test.go`

**Interfaces:**
- Consumes (all unchanged):
  - `db.MigrateYDB(ctx context.Context, d *ydb.Driver) error` (`internal/db/ydbmigrate.go`).
  - `tenantdomain.TenantRepository.GetBySlug(ctx, slug string) (*tenantdomain.Tenant, error)`, which returns `tenantdomain.ErrTenantNotFound` when there is no such tenant, and `CreateWorkspace(ctx, t *Tenant, ownerID string) (*Tenant, error)` for fixtures. The YDB implementation is `tenantadapters.NewYDBTenants(d)`.
  - `sendingdomain.SendingDomainRepository`: `All`, `Add` and `Update`. On `*SendingDomain`: `IsPending()`, `LastCheckedAt() *time.Time`, `CreatedAt()`, `RecordCheck(at)` and `MarkVerified(at) error`. The YDB implementation is `sendingadapters.NewYDBSendingDomains(d)`.
  - `deliverabilitycommand.IngestNotification{RawPayload []byte}` and `deliverabilitydomain.ErrValidationFailed`. The parser returns the latter for a notification that is not valid JSON.
  - `dbtest.YDBMigrated(t) *ydb.Driver` and `dbtest.RandString() string`.
- Produces (Plan C4 builds this in the serverless composition root):
  ```go
  // package service
  type WorkerRoutes struct {
      Jobs           http.Handler                          // *ymq.Dispatcher
      Feedback       feedbackIngester                      // deliverabilitycommand.IngestNotificationHandler
      YDB            *ydb.Driver
      Tenants        tenantdomain.TenantRepository
      Domains        sendingdomain.SendingDomainRepository
      Enqueuer       cronEnqueuer                          // *ymq.Enqueuer
      TenantSlug     string
      VerifyInterval time.Duration
      Logger         *slog.Logger
  }
  func (wr WorkerRoutes) Handler() http.Handler
  // unexported consumer-side ports
  type cronEnqueuer interface {
      EnqueueVerify(ctx context.Context, tenantID, domainID string) error
      EnqueueAnalyticsRefresh(ctx context.Context, tenantID string) error
  }
  type feedbackIngester interface {
      Handle(ctx context.Context, cmd deliverabilitycommand.IngestNotification) error
  }
  ```
- Routes:
  - `POST /jobs` goes to `Jobs`.
  - `POST /feedback` takes a Data Streams trigger request, `{"messages":[<notification JSON>, ...]}`. Every element goes to `Feedback.Handle`. `ErrValidationFailed` is logged and skipped; any other error answers 500.
  - `POST /admin/migrate` runs `db.MigrateYDB` and answers 200, or 500 on an error.
  - `POST /cron/domain-verify-sweep` re-arms (`EnqueueVerify`) each pending domain of the `TenantSlug` tenant whose last poll, or its creation if it was never polled, is older than `2 × VerifyInterval`.
  - `POST /cron/analytics-refresh` calls `EnqueueAnalyticsRefresh` for the `TenantSlug` tenant.
  - Both cron routes answer 200 even when the sweep fails, after logging the failure. Before the tenant exists they do nothing.
  - Any other path is 404, and a wrong method is 405 (`http.ServeMux` patterns).

- [ ] **Step 1: Write the failing tests**

Create `internal/service/worker_routes_test.go`:

```go
package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	deliverabilitycommand "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	deliverabilitydomain "github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	sendingadapters "github.com/nikolaymatrosov/nvelope/internal/sending/adapters"
	sendingdomain "github.com/nikolaymatrosov/nvelope/internal/sending/domain"
	tenantadapters "github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

const testVerifyInterval = 15 * time.Minute

// recordingEnqueuer records the cron sweeps' enqueues.
type recordingEnqueuer struct {
	mu        sync.Mutex
	verify    []string // "tenantID/domainID"
	analytics []string // tenantID
}

func (e *recordingEnqueuer) EnqueueVerify(_ context.Context, tenantID, domainID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.verify = append(e.verify, tenantID+"/"+domainID)
	return nil
}

func (e *recordingEnqueuer) EnqueueAnalyticsRefresh(_ context.Context, tenantID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.analytics = append(e.analytics, tenantID)
	return nil
}

// scriptedIngester records each staged payload and returns the next scripted
// error (nil once the script runs out).
type scriptedIngester struct {
	errs     []error
	payloads []string
}

func (s *scriptedIngester) Handle(_ context.Context, cmd deliverabilitycommand.IngestNotification) error {
	s.payloads = append(s.payloads, string(cmd.RawPayload))
	if len(s.errs) == 0 {
		return nil
	}
	err := s.errs[0]
	s.errs = s.errs[1:]
	return err
}

// newWorkerRoutes wires WorkerRoutes over the test YDB for the tenant slug.
func newWorkerRoutes(t *testing.T, slug string) (WorkerRoutes, *recordingEnqueuer, *scriptedIngester) {
	t.Helper()
	d := dbtest.YDBMigrated(t)
	enq := &recordingEnqueuer{}
	ing := &scriptedIngester{}
	return WorkerRoutes{
		Jobs: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
		Feedback:       ing,
		YDB:            d,
		Tenants:        tenantadapters.NewYDBTenants(d),
		Domains:        sendingadapters.NewYDBSendingDomains(d),
		Enqueuer:       enq,
		TenantSlug:     slug,
		VerifyInterval: testVerifyInterval,
		Logger:         slog.New(slog.DiscardHandler),
	}, enq, ing
}

// createTenant creates a YDB tenant with a fresh slug and returns the slug and
// the tenant id.
func createTenant(t *testing.T, wr WorkerRoutes) (slug, id string) {
	t.Helper()
	tenant, err := tenantdomain.NewTenant("Acme", "acme-"+dbtest.RandString())
	require.NoError(t, err)
	created, err := wr.Tenants.CreateWorkspace(context.Background(), tenant, uuid.NewString())
	require.NoError(t, err)
	return created.Slug().String(), created.ID()
}

// addDomain adds a pending sending domain and applies change to it, if given,
// through the repository's Update.
func addDomain(t *testing.T, wr WorkerRoutes, tenantID, name string,
	change func(*sendingdomain.SendingDomain) error) string {
	t.Helper()
	ctx := context.Background()
	d, err := sendingdomain.NewSendingDomain(tenantID, name)
	require.NoError(t, err)
	id, err := wr.Domains.Add(ctx, tenantID, d)
	require.NoError(t, err)
	if change != nil {
		require.NoError(t, wr.Domains.Update(ctx, tenantID, id,
			func(d *sendingdomain.SendingDomain) (*sendingdomain.SendingDomain, error) {
				return d, change(d)
			}))
	}
	return id
}

func checkedAgo(ago time.Duration) func(*sendingdomain.SendingDomain) error {
	return func(d *sendingdomain.SendingDomain) error {
		d.RecordCheck(time.Now().Add(-ago))
		return nil
	}
}

func post(t *testing.T, h http.Handler, path, body string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec.Code
}

func TestWorkerRoutesDomainSweepRearmsOnlyLostChains(t *testing.T) {
	t.Parallel()
	wr, enq, _ := newWorkerRoutes(t, "")
	slug, tenantID := createTenant(t, wr)
	wr.TenantSlug = slug
	_, otherTenantID := createTenant(t, wr)

	addDomain(t, wr, tenantID, "fresh.example.com", nil)
	addDomain(t, wr, tenantID, "recent.example.com", checkedAgo(testVerifyInterval/2))
	lost := addDomain(t, wr, tenantID, "lost.example.com", checkedAgo(3*testVerifyInterval))
	addDomain(t, wr, tenantID, "verified.example.com", func(d *sendingdomain.SendingDomain) error {
		d.RecordCheck(time.Now().Add(-3 * testVerifyInterval))
		return d.MarkVerified(time.Now().Add(-3 * testVerifyInterval))
	})
	addDomain(t, wr, otherTenantID, "lost.example.com", checkedAgo(3*testVerifyInterval))

	require.Equal(t, http.StatusOK, post(t, wr.Handler(), "/cron/domain-verify-sweep", "{}"))
	require.Equal(t, []string{tenantID + "/" + lost}, enq.verify)
}

func TestWorkerRoutesAnalyticsRefreshTargetsTheInstallTenant(t *testing.T) {
	t.Parallel()
	wr, enq, _ := newWorkerRoutes(t, "")
	slug, tenantID := createTenant(t, wr)
	wr.TenantSlug = slug
	createTenant(t, wr)

	require.Equal(t, http.StatusOK, post(t, wr.Handler(), "/cron/analytics-refresh", "{}"))
	require.Equal(t, []string{tenantID}, enq.analytics)
}

func TestWorkerRoutesCronBeforeSetupIsANoOp(t *testing.T) {
	t.Parallel()
	wr, enq, _ := newWorkerRoutes(t, "not-created-"+dbtest.RandString())
	h := wr.Handler()

	require.Equal(t, http.StatusOK, post(t, h, "/cron/domain-verify-sweep", "{}"))
	require.Equal(t, http.StatusOK, post(t, h, "/cron/analytics-refresh", "{}"))
	require.Empty(t, enq.verify)
	require.Empty(t, enq.analytics)
}

func TestWorkerRoutesFeedbackStagesEachNotification(t *testing.T) {
	t.Parallel()
	const batch = `{"messages":[{"eventType":"Bounce","n":1},{"eventType":"Complaint","n":2}]}`

	t.Run("all staged", func(t *testing.T) {
		wr, _, ing := newWorkerRoutes(t, "")
		require.Equal(t, http.StatusOK, post(t, wr.Handler(), "/feedback", batch))
		require.Len(t, ing.payloads, 2)
		require.JSONEq(t, `{"eventType":"Bounce","n":1}`, ing.payloads[0])
		require.JSONEq(t, `{"eventType":"Complaint","n":2}`, ing.payloads[1])
	})
	t.Run("a malformed notification is skipped", func(t *testing.T) {
		wr, _, ing := newWorkerRoutes(t, "")
		ing.errs = []error{deliverabilitydomain.ErrValidationFailed.WithMessage("not valid JSON")}
		require.Equal(t, http.StatusOK, post(t, wr.Handler(), "/feedback", batch))
		require.Len(t, ing.payloads, 2, "the batch goes on past it")
	})
	t.Run("a transient failure fails the batch", func(t *testing.T) {
		wr, _, ing := newWorkerRoutes(t, "")
		ing.errs = []error{errors.New("ydb unavailable")}
		require.Equal(t, http.StatusInternalServerError, post(t, wr.Handler(), "/feedback", batch))
		require.Len(t, ing.payloads, 1)
	})
	t.Run("an undecodable request fails", func(t *testing.T) {
		wr, _, _ := newWorkerRoutes(t, "")
		require.Equal(t, http.StatusInternalServerError, post(t, wr.Handler(), "/feedback", "{"))
	})
}

func TestWorkerRoutesMigrateIsIdempotent(t *testing.T) {
	t.Parallel()
	wr, _, _ := newWorkerRoutes(t, "")
	h := wr.Handler()

	require.Equal(t, http.StatusOK, post(t, h, "/admin/migrate", ""))
	require.Equal(t, http.StatusOK, post(t, h, "/admin/migrate", ""))
}

func TestWorkerRoutesServeOnlyTheTriggerPaths(t *testing.T) {
	t.Parallel()
	wr, _, _ := newWorkerRoutes(t, "")
	h := wr.Handler()

	require.Equal(t, http.StatusTeapot, post(t, h, "/jobs", "{}"), "/jobs reaches the dispatcher")
	require.Equal(t, http.StatusNotFound, post(t, h, "/cron/no-such-sweep", "{}"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/migrate", nil))
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && go test ./internal/service/ -run TestWorkerRoutes -count=1 -timeout 8m`

Expected: FAIL to build, with `undefined: WorkerRoutes`.

- [ ] **Step 3: Write the routes**

Create `internal/service/worker_routes.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	deliverabilitycommand "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	deliverabilitydomain "github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	sendingdomain "github.com/nikolaymatrosov/nvelope/internal/sending/domain"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// cronEnqueuer enqueues the jobs the cron sweeps start. *ymq.Enqueuer
// implements it.
type cronEnqueuer interface {
	EnqueueVerify(ctx context.Context, tenantID, domainID string) error
	EnqueueAnalyticsRefresh(ctx context.Context, tenantID string) error
}

// feedbackIngester stages one Postbox notification.
// deliverabilitycommand.IngestNotificationHandler implements it.
type feedbackIngester interface {
	Handle(ctx context.Context, cmd deliverabilitycommand.IngestNotification) error
}

// WorkerRoutes is the HTTP surface of the worker container in serverless
// mode, standing in for cmd/worker's River client, cmd/scheduler and
// cmd/consumer. Only triggers and the deploy step call it. The container is
// private, so YC IAM (serverless.containers.invoker) authorizes every request.
type WorkerRoutes struct {
	// Jobs runs queued jobs (a *ymq.Dispatcher): the Message Queue triggers'
	// target.
	Jobs http.Handler
	// Feedback stages Postbox delivery feedback: the Data Streams trigger's
	// target.
	Feedback feedbackIngester
	// YDB is the database POST /admin/migrate migrates.
	YDB *ydb.Driver
	// Tenants, Domains and Enqueuer back the timer triggers' sweeps.
	Tenants  tenantdomain.TenantRepository
	Domains  sendingdomain.SendingDomainRepository
	Enqueuer cronEnqueuer
	// TenantSlug names the install's single tenant (NVELOPE_TENANT_SLUG).
	TenantSlug string
	// VerifyInterval is the sending-domain verification poll interval
	// (NVELOPE_SENDING_DOMAIN_VERIFY_INTERVAL).
	VerifyInterval time.Duration
	Logger         *slog.Logger
}

// Handler routes the trigger and deploy paths.
func (wr WorkerRoutes) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /jobs", wr.Jobs)
	mux.HandleFunc("POST /feedback", wr.feedback)
	mux.HandleFunc("POST /admin/migrate", wr.migrate)
	mux.HandleFunc("POST /cron/domain-verify-sweep", wr.cron(wr.sweepDomains))
	mux.HandleFunc("POST /cron/analytics-refresh", wr.cron(wr.refreshAnalytics))
	return mux
}

// dataStreamsRequest is a Data Streams trigger request. Each element of
// messages is one stream message as written; on the Postbox feedback topic,
// that is one notification JSON object.
type dataStreamsRequest struct {
	Messages []json.RawMessage `json:"messages"`
}

// feedback stages every notification in the batch, as cmd/consumer stages
// each one it reads. A non-200 makes the trigger retry the batch, then send
// it to the trigger's dead-letter queue. Staging dedupes by notification, so a
// retried batch does not count the messages before the failure twice.
func (wr WorkerRoutes) feedback(w http.ResponseWriter, r *http.Request) {
	var req dataStreamsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wr.Logger.Error("decoding the data streams trigger request", "error", err)
		http.Error(w, "undecodable trigger request", http.StatusInternalServerError)
		return
	}
	for _, raw := range req.Messages {
		err := wr.Feedback.Handle(r.Context(), deliverabilitycommand.IngestNotification{RawPayload: raw})
		switch {
		case err == nil:
		case errors.Is(err, deliverabilitydomain.ErrValidationFailed):
			// A malformed notification can never succeed. Skip it rather than
			// retry the batch into the dead-letter queue.
			wr.Logger.Warn("skipping malformed feedback notification", "error", err)
		default:
			wr.Logger.Error("staging feedback notification", "error", err)
			http.Error(w, "staging failed", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// migrate applies the embedded YDB migrations. It is idempotent: the deploy
// step calls it on every image change.
func (wr WorkerRoutes) migrate(w http.ResponseWriter, r *http.Request) {
	if err := db.MigrateYDB(r.Context(), wr.YDB); err != nil {
		wr.Logger.Error("migrating YDB", "error", err)
		http.Error(w, "migration failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// cron runs one sweep. A failure is logged and still answered 200, as
// cmd/scheduler logs and carries on: the next tick is the retry. A trigger
// retry could re-arm a domain a second time before its first poll records a
// check.
func (wr WorkerRoutes) cron(sweep func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := sweep(r.Context()); err != nil {
			wr.Logger.Error("cron sweep failed", "path", r.URL.Path, "error", err)
		}
		w.WriteHeader(http.StatusOK)
	}
}

// tenantID returns the install's tenant id, or "" while the first-run setup
// has not created the tenant yet.
func (wr WorkerRoutes) tenantID(ctx context.Context) (string, error) {
	t, err := wr.Tenants.GetBySlug(ctx, wr.TenantSlug)
	if errors.Is(err, tenantdomain.ErrTenantNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return t.ID(), nil
}

// sweepDomains re-arms verification for each pending domain whose last poll
// (or, if it was never polled, its creation) is older than two poll
// intervals. Such a domain's self-snoozing job chain was lost: it was
// dead-lettered, or the enqueue after creation failed. A live chain polls
// every interval, so the sweep never starts a second chain beside it. In SaaS,
// River's unique insert gives the same guarantee. YMQ has no equivalent.
func (wr WorkerRoutes) sweepDomains(ctx context.Context) error {
	tenantID, err := wr.tenantID(ctx)
	if err != nil || tenantID == "" {
		return err
	}
	domains, err := wr.Domains.All(ctx, tenantID)
	if err != nil {
		return err
	}
	staleBefore := time.Now().Add(-2 * wr.VerifyInterval)
	var errs []error
	for _, d := range domains {
		last := d.CreatedAt()
		if checked := d.LastCheckedAt(); checked != nil {
			last = *checked
		}
		if !d.IsPending() || last.After(staleBefore) {
			continue
		}
		if err := wr.Enqueuer.EnqueueVerify(ctx, tenantID, d.ID()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// refreshAnalytics enqueues the install tenant's analytics refresh.
func (wr WorkerRoutes) refreshAnalytics(ctx context.Context) error {
	tenantID, err := wr.tenantID(ctx)
	if err != nil || tenantID == "" {
		return err
	}
	return wr.Enqueuer.EnqueueAnalyticsRefresh(ctx, tenantID)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run:

```bash
gofmt -l internal/service && go vet ./internal/service/ \
  && export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" \
  && go test ./internal/service/ -run TestWorkerRoutes -count=1 -race -v -timeout 8m
```

Expected: PASS for the six `TestWorkerRoutes*` tests, including the four `TestWorkerRoutesFeedbackStagesEachNotification` subtests.

In `TestWorkerRoutesDomainSweepRearmsOnlyLostChains`, exactly the `lost.example.com` domain of the configured tenant is re-armed. These are not:
- the fresh domain, the recently polled one and the verified one;
- the other tenant's lost domain.

- [ ] **Step 5: Commit**

```bash
git add internal/service/worker_routes.go internal/service/worker_routes_test.go
git commit -m "feat(service): add the serverless worker routes for cron, feedback and migrate"
```

---

## Done criteria for Plan C1

Run from the worktree root:

```bash
export PATH="$HOME/go/bin:$PATH" && export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" \
  && go build ./... && go vet ./... && golangci-lint run ./... && make lint-arch \
  && go test ./... -timeout 15m
```

Expected:
- `golangci-lint` reports `0 issues`.
- `make lint-arch` ends with `[arch] clean`.
- Every package is `ok`, with no `FAIL`. If one fails, see "Shared test containers" under Global Constraints before debugging.

`git diff main --stat -- '*.go'` lists only the eight new files, plus `go.mod` and `go.sum`.

## Handover to Plan C4

- **Config.** C4 adds the config keys: the YMQ endpoint and keys, the two queue URLs, `NVELOPE_TENANT_SLUG` and the job timeout. The job timeout is the container's `execution_timeout` minus a margin.
- **Building the worker.** C4 builds `ymq.New(ymq.NewClient(...), jobsURL, jobsLongURL)` and `ymq.NewDispatcher(enq, timeout, logger)`. Then it registers every worker `cmd/worker` registers today, except billing and `auth.verification_cleanup`: `ymq.AddWorker(d, audienceadapters.NewImportWorker(...))` and so on, with the YDB adapters and `nil` billing ports. Finally it serves `service.WorkerRoutes{Jobs: d, ...}.Handler()` on `$PORT`.
- **The enqueuer.** The same `*ymq.Enqueuer` replaces both `jobs.Enqueuer` and `jobs.SendEnqueuer` in the composition root. It is also `WorkerRoutes.Enqueuer`. `WorkerRoutes.Feedback` is `deliverabilitycommand.NewIngestNotificationHandler(deliverabilityadapters.NewNotificationParser(), <YDB events>, enq)`.
- **For C3 (Terraform):**
  - each Message Queue trigger uses `batch_size = 1`;
  - the long queue's `visibility_timeout_seconds` is at least the worker's `execution_timeout`;
  - each queue's `redrive_policy` has `maxReceiveCount = 5`;
  - the timer triggers' paths are `/cron/domain-verify-sweep` and `/cron/analytics-refresh`, and the Data Streams trigger's is `/feedback`;
  - the analytics timer interval decides idle RU cost. SaaS refreshes every 60 s, which is too often for zero idle cost.

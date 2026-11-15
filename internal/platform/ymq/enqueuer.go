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

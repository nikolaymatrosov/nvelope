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

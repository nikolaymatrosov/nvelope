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

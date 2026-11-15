package ymq

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestRealYMQRoundTrip sends, receives and deletes one message on real Yandex
// Message Queue with the production client and Enqueuer. It confirms that YMQ
// accepts the AWS JSON protocol service/sqs speaks (fakecloud cannot). It is
// skipped unless NVELOPE_TEST_YMQ_ACCESS_KEY_ID and
// NVELOPE_TEST_YMQ_SECRET_ACCESS_KEY hold a static key of a service account
// with ymq.admin. It creates its own queue and deletes it.
func TestRealYMQRoundTrip(t *testing.T) {
	keyID, secret := os.Getenv("NVELOPE_TEST_YMQ_ACCESS_KEY_ID"), os.Getenv("NVELOPE_TEST_YMQ_SECRET_ACCESS_KEY")
	if keyID == "" || secret == "" {
		t.Skip("set NVELOPE_TEST_YMQ_ACCESS_KEY_ID and NVELOPE_TEST_YMQ_SECRET_ACCESS_KEY to probe real YMQ")
	}
	client := NewClient("https://message-queue.api.cloud.yandex.net", "ru-central1", keyID, secret)
	url := createQueue(t, client, "nvelope-probe-"+uuid.NewString()[:8], "")
	t.Cleanup(func() {
		_, err := client.DeleteQueue(context.Background(), &sqs.DeleteQueueInput{QueueUrl: aws.String(url)})
		require.NoError(t, err)
	})

	require.NoError(t, New(client, url, url).EnqueueAnalyticsRefresh(t.Context(), "probe-tenant"))
	msg := receive(t, client, url, 20*time.Second)
	require.NotNil(t, msg, "the enqueued message never arrived")
	require.JSONEq(t, `{"kind":"analytics.refresh","args":{"tenant_id":"probe-tenant"}}`, *msg.Body)
	_, err := client.DeleteMessage(t.Context(), &sqs.DeleteMessageInput{QueueUrl: aws.String(url), ReceiptHandle: msg.ReceiptHandle})
	require.NoError(t, err)
}

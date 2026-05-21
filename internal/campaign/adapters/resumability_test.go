package adapters_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/platform/ratelimit"
)

// crashingMessenger delivers messages but cancels the worker context after a
// fixed number of sends, simulating a worker killed mid-batch.
type crashingMessenger struct {
	mu      sync.Mutex
	sent    map[string]int
	crashAt int
	cancel  context.CancelFunc
	crashed bool
}

func (m *crashingMessenger) Send(_ context.Context, msg domain.OutboundMessage) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent[msg.To]++
	if !m.crashed && len(m.sent) >= m.crashAt {
		m.crashed = true
		m.cancel() // the next loop iteration sees the cancelled context
	}
	return "msg-ref", nil
}

func (m *crashingMessenger) total() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range m.sent {
		n += c
	}
	return n
}

func (m *crashingMessenger) duplicates() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var dups []string
	for email, c := range m.sent {
		if c > 1 {
			dups = append(dups, email)
		}
	}
	return dups
}

func TestSendPipelineResumesAfterWorkerCrash(t *testing.T) {
	t.Parallel()
	limit := ratelimit.Limit{Max: 1000, Window: time.Second}
	f := newPipelineFixture(t, 10, 100, limit, limit)

	// Materialise recipients and the single batch via the start worker.
	start := adapters.NewStartWorker(f.campaigns, f.recipients, f.tracking,
		f.source, f.enqueuer, nil, 100)
	require.NoError(t, start.Work(context.Background(), &river.Job[jobs.CampaignStartArgs]{
		Args: jobs.CampaignStartArgs{TenantID: f.tenantID, CampaignID: f.campaignID},
	}))
	require.Len(t, f.enqueuer.batches, 1)

	messenger := &crashingMessenger{sent: map[string]int{}, crashAt: 4}
	newBatch := func() *adapters.BatchWorker {
		return adapters.NewBatchWorker(f.campaigns, f.recipients, f.tracking,
			messenger, adapters.NewRateLimiter(f.limiter), fakeDomainLookup{name: "mail.acme.com"},
			noopSuppression{}, nil, nil, nil, nil,
			domain.Limit{Max: limit.Max, Window: limit.Window}, "https://track.test")
	}
	job := &river.Job[jobs.CampaignBatchArgs]{
		Args: jobs.CampaignBatchArgs{TenantID: f.tenantID, CampaignID: f.campaignID, Offset: 0, Limit: 100},
	}

	// First run: the worker is "killed" after crashAt sends.
	crashCtx, cancel := context.WithCancel(context.Background())
	messenger.cancel = cancel
	err := newBatch().Work(crashCtx, job)
	require.Error(t, err, "the cancelled context aborts the batch")
	require.Less(t, messenger.total(), 10, "the crash left recipients unsent")

	// Restart: River redelivers the same batch with a fresh context.
	require.NoError(t, newBatch().Work(context.Background(), job))

	require.Equal(t, 10, messenger.total(), "every recipient is eventually sent")
	require.Empty(t, messenger.duplicates(), "no recipient is sent twice across the restart")

	c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
	require.NoError(t, err)
	require.Equal(t, domain.CampaignFinished, c.Status())
	require.Equal(t, 10, c.SentCount())
}

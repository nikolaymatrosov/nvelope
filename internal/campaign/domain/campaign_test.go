package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

func newDraft(t *testing.T) *domain.Campaign {
	t.Helper()
	c, err := domain.NewCampaign("tenant-1", "Spring Sale", "Big news", "<p>hi</p>", "",
		"Acme", "news", "domain-1", "", 100)
	require.NoError(t, err)
	return c
}

func TestNewCampaignValidates(t *testing.T) {
	t.Parallel()
	_, err := domain.NewCampaign("tenant-1", "", "s", "<p>b</p>", "", "Acme", "news", "d", "", 100)
	require.ErrorIs(t, err, domain.ErrCampaignInvalid, "name is required")

	_, err = domain.NewCampaign("tenant-1", "C", "s", "<p>b</p>", "", "Acme", "bad local part", "d", "", 100)
	require.ErrorIs(t, err, domain.ErrCampaignInvalid, "rejects an invalid local part")
}

func TestNewCampaignAllowsBareDraft(t *testing.T) {
	t.Parallel()
	c, err := domain.NewCampaign("tenant-1", "Just a name", "", "", "", "", "", "", "", 0)
	require.NoError(t, err, "a draft only needs a name")
	require.True(t, c.IsDraft())
}

func TestCampaignStartRequiresContent(t *testing.T) {
	t.Parallel()
	c, err := domain.NewCampaign("tenant-1", "C", "", "", "", "", "", "domain-1", "", 100)
	require.NoError(t, err)
	require.ErrorIs(t, c.Start(time.Now()), domain.ErrCampaignInvalid, "subject is required")

	c, err = domain.NewCampaign("tenant-1", "C", "s", "", "", "", "", "domain-1", "", 100)
	require.NoError(t, err)
	require.ErrorIs(t, c.Start(time.Now()), domain.ErrCampaignInvalid, "needs a body")

	c, err = domain.NewCampaign("tenant-1", "C", "s", "<p>b</p>", "", "", "", "domain-1", "", 100)
	require.NoError(t, err)
	require.ErrorIs(t, c.Start(time.Now()), domain.ErrCampaignInvalid, "From local part is required")
}

func TestCampaignStartRequiresDomain(t *testing.T) {
	t.Parallel()
	c, err := domain.NewCampaign("tenant-1", "C", "s", "<p>b</p>", "", "Acme", "news", "", "", 100)
	require.NoError(t, err)
	require.ErrorIs(t, c.Start(time.Now()), domain.ErrSendingDomainRequired)
}

func TestCampaignLifecycle(t *testing.T) {
	t.Parallel()
	c := newDraft(t)
	require.True(t, c.IsDraft())

	require.NoError(t, c.Start(time.Now()))
	require.True(t, c.IsRunning())
	require.NotNil(t, c.StartedAt())
	require.ErrorIs(t, c.Start(time.Now()), domain.ErrCampaignNotDraft)

	require.NoError(t, c.Pause())
	require.Equal(t, domain.CampaignPaused, c.Status())

	require.NoError(t, c.Resume())
	require.True(t, c.IsRunning())

	require.NoError(t, c.Finish(time.Now()))
	require.Equal(t, domain.CampaignFinished, c.Status())
	require.NotNil(t, c.FinishedAt())
}

func TestCampaignDraftOnlyEditing(t *testing.T) {
	t.Parallel()
	c := newDraft(t)
	require.NoError(t, c.Recompose("New", "New subj", "<p>x</p>", "", "Acme", "hello", "domain-1"))
	require.Equal(t, "New", c.Name())

	require.NoError(t, c.Start(time.Now()))
	require.ErrorIs(t,
		c.Recompose("Nope", "s", "<p>x</p>", "", "Acme", "hello", "domain-1"),
		domain.ErrCampaignNotEditable)
}

func TestCampaignProgressAndAutoPause(t *testing.T) {
	t.Parallel()
	c, err := domain.NewCampaign("tenant-1", "C", "s", "<p>b</p>", "", "Acme", "news", "d", "", 2)
	require.NoError(t, err)
	require.NoError(t, c.Start(time.Now()))

	c.RecordProgress(5, 1)
	require.Equal(t, 5, c.SentCount())
	require.Equal(t, 1, c.FailedCount())
	require.False(t, c.ShouldAutoPause())

	c.RecordProgress(0, 2)
	require.Equal(t, 3, c.FailedCount())
	require.True(t, c.ShouldAutoPause(), "failed count exceeded the threshold")
}

func TestCampaignCancel(t *testing.T) {
	t.Parallel()
	c := newDraft(t)
	require.NoError(t, c.Cancel())
	require.Equal(t, domain.CampaignCancelled, c.Status())
	require.Error(t, c.Cancel(), "a cancelled campaign cannot be cancelled again")
}

func TestNewTemplateValidates(t *testing.T) {
	t.Parallel()
	_, err := domain.NewTemplate("t1", "Welcome", domain.KindCampaign, "Hi", "<p>b</p>", "")
	require.NoError(t, err)

	_, err = domain.NewTemplate("t1", "", domain.KindCampaign, "Hi", "<p>b</p>", "")
	require.ErrorIs(t, err, domain.ErrTemplateInvalid)

	_, err = domain.NewTemplate("t1", "T", domain.KindCampaign, "Hi", "", "")
	require.ErrorIs(t, err, domain.ErrTemplateInvalid, "needs a body")

	_, err = domain.NewTemplate("t1", "T", domain.Kind("bogus"), "Hi", "<p>b</p>", "")
	require.ErrorIs(t, err, domain.ErrTemplateInvalid, "rejects an unknown kind")
}

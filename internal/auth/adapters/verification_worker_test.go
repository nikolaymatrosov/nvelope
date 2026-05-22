package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// recordingVerificationMailer is a test double for domain.VerificationMailer.
type recordingVerificationMailer struct {
	last domain.VerificationEmail
	sent int
}

func (m *recordingVerificationMailer) Send(_ context.Context, msg domain.VerificationEmail) error {
	m.last = msg
	m.sent++
	return nil
}

func TestVerificationWorkerSendsEmail(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	users := adapters.NewUsers(pool)

	u, addr := newUser(t, "Ada Lovelace")
	created, err := users.Create(ctx, u, "hash")
	require.NoError(t, err)

	mailer := &recordingVerificationMailer{}
	worker := adapters.NewVerificationWorker(users, mailer,
		"nvelope.ru", "nvelope", "https://app.example.com")

	require.NoError(t, worker.Work(ctx, &river.Job[jobs.VerificationSendArgs]{
		Args: jobs.VerificationSendArgs{UserID: created.ID(), VerificationToken: "raw-token-123"},
	}))

	require.Equal(t, 1, mailer.sent)
	require.Equal(t, addr, mailer.last.To)
	require.Equal(t, "no-reply@nvelope.ru", mailer.last.FromAddress)
	require.Equal(t, "nvelope", mailer.last.FromName)
	require.Contains(t, mailer.last.HTMLBody,
		"https://app.example.com/verify-email?token=raw-token-123")
	require.Contains(t, mailer.last.TextBody, "raw-token-123")
}

func TestVerificationWorkerSkipsAlreadyVerifiedUser(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	users := adapters.NewUsers(pool)

	u, _ := newUser(t, "Ada")
	created, err := users.Create(ctx, u, "hash")
	require.NoError(t, err)
	require.NoError(t, users.MarkEmailVerified(ctx, created.ID(), time.Now()))

	mailer := &recordingVerificationMailer{}
	worker := adapters.NewVerificationWorker(users, mailer,
		"nvelope.ru", "nvelope", "https://app.example.com")

	require.NoError(t, worker.Work(ctx, &river.Job[jobs.VerificationSendArgs]{
		Args: jobs.VerificationSendArgs{UserID: created.ID(), VerificationToken: "raw"},
	}))
	require.Equal(t, 0, mailer.sent, "an already-verified account is not emailed")
}

func TestVerificationWorkerIgnoresMissingUser(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	users := adapters.NewUsers(pool)

	mailer := &recordingVerificationMailer{}
	worker := adapters.NewVerificationWorker(users, mailer,
		"nvelope.ru", "nvelope", "https://app.example.com")

	// A deleted account must not fail a River redelivery.
	require.NoError(t, worker.Work(context.Background(), &river.Job[jobs.VerificationSendArgs]{
		Args: jobs.VerificationSendArgs{
			UserID:            "00000000-0000-0000-0000-000000000000",
			VerificationToken: "raw",
		},
	}))
	require.Equal(t, 0, mailer.sent)
}

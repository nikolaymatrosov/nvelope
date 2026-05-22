package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/riverqueue/river"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// VerificationWorker is the River worker that sends one registration
// email-verification message. It is durable and retry-capable: a restarted
// worker re-sends rather than dropping the verification email, and a send
// failure fails the job so River retries with backoff.
type VerificationWorker struct {
	river.WorkerDefaults[jobs.VerificationSendArgs]
	users         domain.UserRepository
	mailer        domain.VerificationMailer
	senderDomain  string
	senderName    string
	publicBaseURL string
}

// NewVerificationWorker builds the verification worker, failing fast on a nil
// dependency or an empty sender domain.
func NewVerificationWorker(users domain.UserRepository, mailer domain.VerificationMailer,
	senderDomain, senderName, publicBaseURL string) *VerificationWorker {

	if users == nil || mailer == nil {
		panic("nil dependency")
	}
	if senderDomain == "" {
		panic("empty verification sender domain")
	}
	return &VerificationWorker{
		users:         users,
		mailer:        mailer,
		senderDomain:  senderDomain,
		senderName:    senderName,
		publicBaseURL: publicBaseURL,
	}
}

// Work sends one verification email.
func (w *VerificationWorker) Work(ctx context.Context, job *river.Job[jobs.VerificationSendArgs]) error {
	user, err := w.users.GetByID(ctx, job.Args.UserID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil // the account is gone — River redelivery is harmless
	}
	if err != nil {
		return err
	}
	if user.IsEmailVerified() {
		return nil // already verified — nothing to send
	}

	verifyURL := strings.TrimRight(w.publicBaseURL, "/") + "/verify-email?token=" +
		url.QueryEscape(job.Args.VerificationToken)
	lang := user.Locale().String()

	subject, htmlBody, textBody := renderVerificationEmail(lang, user.Name(), verifyURL)

	if err := w.mailer.Send(ctx, domain.VerificationEmail{
		FromName:    w.senderName,
		FromAddress: "no-reply@" + w.senderDomain,
		To:          user.Email().String(),
		Subject:     subject,
		HTMLBody:    htmlBody,
		TextBody:    textBody,
	}); err != nil {
		return fmt.Errorf("sending verification email: %w", err)
	}
	return nil
}

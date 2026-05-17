package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

func TestIssueAPIKeyHandler(t *testing.T) {
	t.Parallel()
	keys, audit := newFakeAPIKeys(), &fakeAudit{}
	h := command.NewIssueAPIKeyHandler(keys, audit)

	res, err := h.Handle(context.Background(), command.IssueAPIKey{
		TenantID: "t1", ActorID: "u1", Name: "CI",
		Permissions: []string{"subscribers:get"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Token, "the raw key is surfaced once")
	require.NotEmpty(t, res.KeyID)
	require.Len(t, audit.records, 1)
	require.Equal(t, "apikey.issue", audit.records[0].Action)

	// The stored key is the hash of the raw token, never the token itself.
	stored, err := keys.ByTokenHash(context.Background(), "t1", token.Hash(res.Token))
	require.NoError(t, err)
	require.Equal(t, res.KeyID, stored.ID())
}

func TestIssueAPIKeyRejectsUnknownPermission(t *testing.T) {
	t.Parallel()
	h := command.NewIssueAPIKeyHandler(newFakeAPIKeys(), &fakeAudit{})
	_, err := h.Handle(context.Background(), command.IssueAPIKey{
		TenantID: "t1", ActorID: "u1", Name: "K", Permissions: []string{"lists:explode"},
	})
	require.Error(t, err)
}

func TestRevokeAPIKeyHandler(t *testing.T) {
	t.Parallel()
	keys, audit := newFakeAPIKeys(), &fakeAudit{}
	issued, err := command.NewIssueAPIKeyHandler(keys, audit).Handle(context.Background(),
		command.IssueAPIKey{TenantID: "t1", ActorID: "u1", Name: "K"})
	require.NoError(t, err)

	require.NoError(t, command.NewRevokeAPIKeyHandler(keys, audit).Handle(context.Background(),
		command.RevokeAPIKey{TenantID: "t1", ActorID: "u1", KeyID: issued.KeyID}))
	require.Equal(t, "apikey.revoke", audit.records[1].Action)

	require.ErrorIs(t, command.NewRevokeAPIKeyHandler(keys, audit).Handle(context.Background(),
		command.RevokeAPIKey{TenantID: "t1", ActorID: "u1", KeyID: issued.KeyID}),
		domain.ErrAPIKeyNotFound)
}

func TestEnableTOTPHandler(t *testing.T) {
	t.Parallel()
	h := command.NewEnableTOTPHandler(fakeTOTP{})
	res, err := h.Handle(context.Background(), command.EnableTOTP{
		TenantID: "t1", UserID: "u1", AccountName: "user@example.com",
	})
	require.NoError(t, err)
	require.Equal(t, "FAKESECRET", res.Secret)
	require.Contains(t, res.URI, "otpauth://totp/")
}

func TestConfirmTOTPHandler(t *testing.T) {
	t.Parallel()
	users, recovery := newFakeUsers(), newFakeRecoveryCodes()
	u, err := domain.NewTenantUser("t1", "p1", "user@example.com", "Pat")
	require.NoError(t, err)
	users.add("user-1", u)

	h := command.NewConfirmTOTPHandler(users, recovery, fakeTOTP{validCode: "123456"})
	res, err := h.Handle(context.Background(), command.ConfirmTOTP{
		TenantID: "t1", UserID: "user-1", Secret: "FAKESECRET", Code: "123456",
	})
	require.NoError(t, err)
	require.Len(t, res.RecoveryCodes, 10, "enrolment mints a batch of recovery codes")

	got, err := users.Get(context.Background(), "t1", "user-1")
	require.NoError(t, err)
	require.True(t, got.TOTPEnabled())
}

func TestConfirmTOTPRejectsWrongCode(t *testing.T) {
	t.Parallel()
	users, recovery := newFakeUsers(), newFakeRecoveryCodes()
	u, _ := domain.NewTenantUser("t1", "p1", "user@example.com", "Pat")
	users.add("user-1", u)

	h := command.NewConfirmTOTPHandler(users, recovery, fakeTOTP{validCode: "123456"})
	_, err := h.Handle(context.Background(), command.ConfirmTOTP{
		TenantID: "t1", UserID: "user-1", Secret: "FAKESECRET", Code: "000000",
	})
	require.ErrorIs(t, err, domain.ErrTOTPInvalidCode)
}

func TestDisableTOTPHandler(t *testing.T) {
	t.Parallel()
	users, recovery := newFakeUsers(), newFakeRecoveryCodes()
	u, _ := domain.NewTenantUser("t1", "p1", "user@example.com", "Pat")
	require.NoError(t, u.EnableTOTP([]byte("enc:FAKESECRET")))
	users.add("user-1", u)
	require.NoError(t, recovery.AddBatch(context.Background(), "t1", "user-1", []string{"h1"}))

	require.NoError(t, command.NewDisableTOTPHandler(users, recovery).Handle(context.Background(),
		command.DisableTOTP{TenantID: "t1", UserID: "user-1"}))

	got, _ := users.Get(context.Background(), "t1", "user-1")
	require.False(t, got.TOTPEnabled())
	consumed, _ := recovery.Consume(context.Background(), "t1", "user-1", "h1")
	require.False(t, consumed, "disabling TOTP discards recovery codes")
}

// pendingSession builds and stores a totp-pending session for user-1, returning
// the raw token.
func pendingSession(t *testing.T, sessions *fakeSessions) string {
	t.Helper()
	raw := "pending-token"
	session, err := domain.NewSession("t1", "user-1", token.Hash(raw), true,
		time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = sessions.Add(context.Background(), "t1", session)
	require.NoError(t, err)
	return raw
}

func TestVerifyTOTPChallengeWithCode(t *testing.T) {
	t.Parallel()
	users, sessions, recovery := newFakeUsers(), newFakeSessions(), newFakeRecoveryCodes()
	u, _ := domain.NewTenantUser("t1", "p1", "user@example.com", "Pat")
	require.NoError(t, u.EnableTOTP([]byte("enc:FAKESECRET")))
	users.add("user-1", u)
	raw := pendingSession(t, sessions)

	h := command.NewVerifyTOTPChallengeHandler(sessions, users, recovery, fakeTOTP{validCode: "123456"})
	require.NoError(t, h.Handle(context.Background(), command.VerifyTOTPChallenge{
		TenantID: "t1", Token: raw, Code: "123456",
	}))

	s, err := sessions.ByTokenHash(context.Background(), "t1", token.Hash(raw))
	require.NoError(t, err)
	require.Equal(t, domain.SessionActive, s.State())
}

func TestVerifyTOTPChallengeWithRecoveryCode(t *testing.T) {
	t.Parallel()
	users, sessions, recovery := newFakeUsers(), newFakeSessions(), newFakeRecoveryCodes()
	u, _ := domain.NewTenantUser("t1", "p1", "user@example.com", "Pat")
	require.NoError(t, u.EnableTOTP([]byte("enc:FAKESECRET")))
	users.add("user-1", u)
	raw := pendingSession(t, sessions)
	require.NoError(t, recovery.AddBatch(context.Background(), "t1", "user-1",
		[]string{token.Hash("recovery-1")}))

	h := command.NewVerifyTOTPChallengeHandler(sessions, users, recovery, fakeTOTP{validCode: "123456"})
	require.NoError(t, h.Handle(context.Background(), command.VerifyTOTPChallenge{
		TenantID: "t1", Token: raw, Code: "recovery-1",
	}))

	s, _ := sessions.ByTokenHash(context.Background(), "t1", token.Hash(raw))
	require.Equal(t, domain.SessionActive, s.State())
}

func TestVerifyTOTPChallengeRejectsWrongCode(t *testing.T) {
	t.Parallel()
	users, sessions, recovery := newFakeUsers(), newFakeSessions(), newFakeRecoveryCodes()
	u, _ := domain.NewTenantUser("t1", "p1", "user@example.com", "Pat")
	require.NoError(t, u.EnableTOTP([]byte("enc:FAKESECRET")))
	users.add("user-1", u)
	raw := pendingSession(t, sessions)

	h := command.NewVerifyTOTPChallengeHandler(sessions, users, recovery, fakeTOTP{validCode: "123456"})
	require.ErrorIs(t, h.Handle(context.Background(), command.VerifyTOTPChallenge{
		TenantID: "t1", Token: raw, Code: "000000",
	}), domain.ErrUnauthenticated)
}

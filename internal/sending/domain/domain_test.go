package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

func TestNewSendingDomainValidates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		ok    bool
	}{
		{"valid subdomain", "mail.acme.com", true},
		{"valid apex", "acme.io", true},
		{"uppercase is lowercased", "Mail.ACME.com", true},
		{"empty", "", false},
		{"no tld", "acme", false},
		{"leading dot", ".acme.com", false},
		{"space", "ac me.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := domain.NewSendingDomain("tenant-1", tc.input)
			if !tc.ok {
				require.ErrorIs(t, err, domain.ErrDomainInvalid)
				return
			}
			require.NoError(t, err)
			require.Equal(t, domain.StatusPending, d.Status())
			require.False(t, d.IsVerified())
			require.True(t, d.IsPending())
		})
	}
}

func TestNewSendingDomainRequiresTenant(t *testing.T) {
	t.Parallel()
	_, err := domain.NewSendingDomain("", "acme.com")
	require.ErrorIs(t, err, domain.ErrDomainInvalid)
}

func TestSendingDomainMarkVerified(t *testing.T) {
	t.Parallel()
	d, err := domain.NewSendingDomain("tenant-1", "acme.com")
	require.NoError(t, err)

	at := time.Now()
	require.NoError(t, d.MarkVerified(at))
	require.Equal(t, domain.StatusVerified, d.Status())
	require.True(t, d.IsVerified())
	require.NotNil(t, d.VerifiedAt())

	// Verified is terminal — a second transition is rejected.
	require.ErrorIs(t, d.MarkVerified(at), domain.ErrDomainNotPending)
	require.ErrorIs(t, d.MarkFailed("x"), domain.ErrDomainNotPending)
}

func TestSendingDomainMarkFailed(t *testing.T) {
	t.Parallel()
	d, err := domain.NewSendingDomain("tenant-1", "acme.com")
	require.NoError(t, err)

	require.NoError(t, d.MarkFailed("dns records not found"))
	require.Equal(t, domain.StatusFailed, d.Status())
	require.Equal(t, "dns records not found", d.FailureReason())

	require.ErrorIs(t, d.MarkVerified(time.Now()), domain.ErrDomainNotPending)
}

func TestSendingDomainRecordCheck(t *testing.T) {
	t.Parallel()
	d, err := domain.NewSendingDomain("tenant-1", "acme.com")
	require.NoError(t, err)
	require.Nil(t, d.LastCheckedAt())

	at := time.Now()
	d.RecordCheck(at)
	require.NotNil(t, d.LastCheckedAt())
	require.Equal(t, domain.StatusPending, d.Status(), "RecordCheck does not change status")
}

func TestSendingDomainApplyProvisioning(t *testing.T) {
	t.Parallel()
	d, err := domain.NewSendingDomain("tenant-1", "acme.com")
	require.NoError(t, err)

	dkim := []domain.DNSRecord{{Type: "CNAME", Name: "sel._domainkey.acme.com", Value: "sel.dkim"}}
	d.ApplyProvisioning("identity-ref", dkim, "v=spf1 ~all", "v=DMARC1; p=none")
	require.Equal(t, "identity-ref", d.IdentityRef())
	require.Equal(t, dkim, d.DKIMRecords())
	require.Equal(t, "v=spf1 ~all", d.SPFRecord())
	require.Equal(t, "v=DMARC1; p=none", d.DMARCRecord())
}

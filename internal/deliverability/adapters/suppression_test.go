package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

func TestSuppressionsUpsertAndList(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.suppressions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		entry, err := domain.NewManualSuppression(tenantID, "blocked@acme.com", "by support")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, entry))
		// A second upsert of the same address is a no-op: the first entry stays.
		again, err := domain.NewSuppressionEntry(tenantID, "Blocked@acme.com", domain.ReasonComplaint, "")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, again))

		page, next, err := repo.List(ctx, tenantID, domain.SuppressionFilter{})
		require.NoError(t, err)
		require.Empty(t, next)
		require.Len(t, page, 1)
		require.NotEmpty(t, page[0].ID())
		require.Equal(t, tenantID, page[0].TenantID())
		require.Equal(t, "blocked@acme.com", page[0].Email())
		require.Equal(t, domain.ReasonManual, page[0].Reason())
		require.Equal(t, "by support", page[0].Note())
		require.Empty(t, page[0].SourceEventID())
		require.False(t, page[0].SuppressedAt().IsZero())
	})
}

func TestSuppressionsListFiltersAndPages(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.suppressions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		for _, e := range []struct {
			email  string
			reason domain.SuppressionReason
		}{
			{"alice@acme.com", domain.ReasonHardBounce},
			{"bob@acme.com", domain.ReasonComplaint},
			{"carol@acme.com", domain.ReasonHardBounce},
			{"ёлка@пример.рф", domain.ReasonManual},
		} {
			entry, err := domain.NewSuppressionEntry(tenantID, e.email, e.reason, "")
			require.NoError(t, err)
			require.NoError(t, repo.Upsert(ctx, entry))
		}

		hard, _, err := repo.List(ctx, tenantID, domain.SuppressionFilter{Reason: domain.ReasonHardBounce})
		require.NoError(t, err)
		require.Len(t, hard, 2)

		like, _, err := repo.List(ctx, tenantID, domain.SuppressionFilter{EmailLike: "ALI"})
		require.NoError(t, err)
		require.Len(t, like, 1, "the email filter ignores case")
		require.Equal(t, "alice@acme.com", like[0].Email())

		cyrillic, _, err := repo.List(ctx, tenantID, domain.SuppressionFilter{EmailLike: "ЁЛК"})
		require.NoError(t, err)
		require.Len(t, cyrillic, 1, "the email filter ignores case for non-ASCII too")

		// Four pages of one, in id order, walked by the cursor.
		var seen []string
		cursor := ""
		for range 4 {
			page, next, err := repo.List(ctx, tenantID, domain.SuppressionFilter{Limit: 1, Cursor: cursor})
			require.NoError(t, err)
			for _, e := range page {
				seen = append(seen, e.Email())
			}
			if next == "" {
				break
			}
			cursor = next
		}
		require.Len(t, seen, 4)
		require.ElementsMatch(t,
			[]string{"alice@acme.com", "bob@acme.com", "carol@acme.com", "ёлка@пример.рф"}, seen)
	})
}

func TestSuppressionsRemove(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.suppressions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		entry, err := domain.NewManualSuppression(tenantID, "gone@acme.com", "")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, entry))

		require.NoError(t, repo.Remove(ctx, tenantID, " Gone@acme.com "))
		require.ErrorIs(t, repo.Remove(ctx, tenantID, "gone@acme.com"), domain.ErrSuppressionNotFound)
	})
}

func TestSuppressionsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.suppressions(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		entryA, err := domain.NewManualSuppression(tenantA, "shared@acme.com", "")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, entryA))

		// The same address is mailable for tenant B — its list is empty.
		pageB, _, err := repo.List(ctx, tenantB, domain.SuppressionFilter{})
		require.NoError(t, err)
		require.Empty(t, pageB)
		// Tenant B cannot remove tenant A's entry.
		require.ErrorIs(t, repo.Remove(ctx, tenantB, "shared@acme.com"), domain.ErrSuppressionNotFound)

		pageA, _, err := repo.List(ctx, tenantA, domain.SuppressionFilter{})
		require.NoError(t, err)
		require.Len(t, pageA, 1)
	})
}

func TestSuppressionCheckerReturnsTheSuppressedSubset(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.suppressions(t)
		checker := b.checker(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		bounced, err := domain.NewSuppressionEntry(tenantA, "bounced@acme.com", domain.ReasonHardBounce, "")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, bounced))
		manual, err := domain.NewManualSuppression(tenantA, "manual@acme.com", "")
		require.NoError(t, err)
		require.NoError(t, repo.Upsert(ctx, manual))

		got, err := checker.Suppressed(ctx, tenantA,
			[]string{"Bounced@acme.com", "manual@acme.com", "fine@acme.com"})
		require.NoError(t, err)
		require.Equal(t, map[string]string{
			"bounced@acme.com": string(domain.ReasonHardBounce),
			"manual@acme.com":  string(domain.ReasonManual),
		}, got)

		empty, err := checker.Suppressed(ctx, tenantA, nil)
		require.NoError(t, err)
		require.Empty(t, empty)

		other, err := checker.Suppressed(ctx, tenantB, []string{"bounced@acme.com"})
		require.NoError(t, err)
		require.Empty(t, other, "another tenant's list does not suppress")
	})
}

func TestSettingsGetDefaultAndPut(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.settings(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		// No row yet — the defaults apply (both toggles on).
		got, err := repo.Get(ctx, tenantID)
		require.NoError(t, err)
		require.True(t, got.SuppressOnHardBounce())
		require.True(t, got.SuppressOnComplaint())

		require.NoError(t, repo.Put(ctx, tenantID, domain.NewBounceSettings(tenantID, true, false)))
		got, err = repo.Get(ctx, tenantID)
		require.NoError(t, err)
		require.True(t, got.SuppressOnHardBounce())
		require.False(t, got.SuppressOnComplaint())
		require.False(t, got.UpdatedAt().IsZero())

		// A second put overwrites the first.
		require.NoError(t, repo.Put(ctx, tenantID, domain.NewBounceSettings(tenantID, false, true)))
		got, err = repo.Get(ctx, tenantID)
		require.NoError(t, err)
		require.False(t, got.SuppressOnHardBounce())
		require.True(t, got.SuppressOnComplaint())
	})
}

func TestSettingsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.settings(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		require.NoError(t, repo.Put(ctx, tenantA, domain.NewBounceSettings(tenantA, false, false)))

		// Tenant B is unaffected — it still sees the defaults.
		gotB, err := repo.Get(ctx, tenantB)
		require.NoError(t, err)
		require.True(t, gotB.SuppressOnHardBounce())
		require.True(t, gotB.SuppressOnComplaint())
	})
}

func TestAuditLogRecordsAnEntry(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		audit := b.audit(t)
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		require.NoError(t, audit.Record(context.Background(), tenantA,
			uuid.NewString(), "suppression.add", "blocked@acme.com"))

		require.Equal(t, 1, b.auditCount(t, tenantA))
		require.Equal(t, 0, b.auditCount(t, tenantB))
	})
}

package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestAuditRecordAndAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.audit(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		rec := domain.NewAuditRecord(tenantID, userID, domain.PrincipalSession,
			"role.create", "role:Editor", map[string]any{"name": "Editor"})
		require.NoError(t, repo.Record(ctx, tenantID, rec))

		records, total, err := repo.All(ctx, tenantID, domain.Page{})
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Len(t, records, 1)
		got := records[0]
		require.NotEmpty(t, got.ID, "the adapter assigns an id")
		require.Equal(t, tenantID, got.TenantID)
		require.Equal(t, userID, got.ActorID)
		require.Equal(t, domain.PrincipalSession, got.ActorKind)
		require.Equal(t, "role.create", got.Action)
		require.Equal(t, "role:Editor", got.Target)
		require.Equal(t, map[string]any{"name": "Editor"}, got.Metadata)
		require.WithinDuration(t, time.Now(), got.CreatedAt, time.Minute, "the database stamps the insert")
	})
}

func TestAuditAllPagesNewestFirst(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.audit(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		for _, action := range []string{"first", "second", "third"} {
			require.NoError(t, repo.Record(ctx, tenantID,
				domain.NewAuditRecord(tenantID, userID, domain.PrincipalAPIKey, action, "", nil)))
		}
		actions := func(records []domain.AuditRecord) []string {
			out := make([]string, len(records))
			for i, r := range records {
				out[i] = r.Action
			}
			return out
		}

		page, total, err := repo.All(ctx, tenantID, domain.Page{Limit: 2})
		require.NoError(t, err)
		require.Equal(t, 3, total, "the total counts every record, not just the page")
		require.Equal(t, []string{"third", "second"}, actions(page))
		require.Equal(t, map[string]any{}, page[0].Metadata, "nil metadata is stored as an empty object")

		page, total, err = repo.All(ctx, tenantID, domain.Page{Offset: 2, Limit: 2})
		require.NoError(t, err)
		require.Equal(t, 3, total)
		require.Equal(t, []string{"first"}, actions(page))

		page, total, err = repo.All(ctx, b.newTenant(t), domain.Page{})
		require.NoError(t, err)
		require.Zero(t, total, "another tenant's records are not visible")
		require.Empty(t, page)
	})
}

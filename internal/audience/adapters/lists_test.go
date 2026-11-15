package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func newList(t *testing.T, tenantID, name string) *domain.List {
	t.Helper()
	l, err := domain.NewList(tenantID, name, "desc", domain.VisibilityPrivate, domain.OptInSingle, []string{"x"})
	require.NoError(t, err)
	return l
}

// addList persists a list and returns its id.
func addList(t *testing.T, repo domain.ListRepository, tenantID, name string) string {
	t.Helper()
	id, err := repo.Add(context.Background(), tenantID, newList(t, tenantID, name))
	require.NoError(t, err)
	return id
}

func TestListsAddGetAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addList(t, repo, tenantID, "Newsletter")
		addList(t, repo, tenantID, "Announcements")

		lists, total, err := repo.All(ctx, tenantID, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 2, total)
		require.Len(t, lists, 2)
		require.Equal(t, "Announcements", lists[0].Name(), "lists are ordered by name")

		got, err := repo.Get(ctx, tenantID, lists[0].ID())
		require.NoError(t, err)
		require.Equal(t, "Announcements", got.Name())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, "desc", got.Description())
		require.Equal(t, domain.VisibilityPrivate, got.Visibility())
		require.Equal(t, domain.OptInSingle, got.OptIn())
		require.Equal(t, []string{"x"}, got.Tags())
		require.False(t, got.CreatedAt().IsZero())

		page, total, err := repo.All(ctx, tenantID, domain.Page{Offset: 1, Limit: 1})
		require.NoError(t, err)
		require.Equal(t, 2, total, "the total ignores the page")
		require.Len(t, page, 1)
		require.Equal(t, "Newsletter", page[0].Name())
	})
}

func TestListsAddWithoutTagsReadsBackEmpty(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		l, err := domain.NewList(tenantID, "Plain", "", domain.VisibilityPublic, domain.OptInDouble, nil)
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, l)
		require.NoError(t, err)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.NotNil(t, got.Tags())
		require.Empty(t, got.Tags())
		require.Equal(t, domain.VisibilityPublic, got.Visibility())
		require.Equal(t, domain.OptInDouble, got.OptIn())
	})
}

func TestListsAddDuplicateNameConflicts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addList(t, repo, tenantID, "Dup")
		_, err := repo.Add(ctx, tenantID, newList(t, tenantID, "Dup"))
		require.ErrorIs(t, err, domain.ErrListNameTaken)

		// Another tenant may reuse the name.
		other := b.newTenant(t)
		addList(t, repo, other, "Dup")
	})
}

func TestListsUpdateAndDelete(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id := addList(t, repo, tenantID, "Old")

		require.NoError(t, repo.Update(ctx, tenantID, id, func(l *domain.List) (*domain.List, error) {
			return l, l.Rename("Renamed")
		}))
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Renamed", got.Name())

		require.NoError(t, repo.Delete(ctx, tenantID, id))
		_, err = repo.Get(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrListNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, id), domain.ErrListNotFound)
	})
}

func TestListsUpdateOntoTakenNameConflicts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		addList(t, repo, tenantID, "Taken")
		id := addList(t, repo, tenantID, "Free")

		err := repo.Update(ctx, tenantID, id, func(l *domain.List) (*domain.List, error) {
			return l, l.Rename("Taken")
		})
		require.ErrorIs(t, err, domain.ErrListNameTaken)
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Free", got.Name(), "a failed update writes nothing")

		require.ErrorIs(t, repo.Update(ctx, tenantID, "not-a-uuid",
			func(l *domain.List) (*domain.List, error) { return l, nil }), domain.ErrListNotFound)
	})
}

func TestListsGetMissing(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrListNotFound)
	})
}

func TestListsTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.lists(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)

		idA := addList(t, repo, tenantA, "Mine")

		lists, total, err := repo.All(ctx, tenantB, domain.DefaultPage)
		require.NoError(t, err)
		require.Zero(t, total)
		require.Empty(t, lists)
		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrListNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantB, idA), domain.ErrListNotFound)
		_, err = repo.Get(ctx, tenantA, idA)
		require.NoError(t, err, "another tenant's delete leaves the list alone")
	})
}

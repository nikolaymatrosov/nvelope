package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// fakeAPIKeys is a minimal in-memory domain.APIKeyRepository.
type fakeAPIKeys struct{ byHash map[string]*domain.APIKey }

func (f *fakeAPIKeys) Add(context.Context, string, *domain.APIKey) (string, error) {
	return "", nil
}
func (f *fakeAPIKeys) ByTokenHash(_ context.Context, _, hash string) (*domain.APIKey, error) {
	k, ok := f.byHash[hash]
	if !ok {
		return nil, domain.ErrAPIKeyNotFound
	}
	return k, nil
}
func (f *fakeAPIKeys) Revoke(context.Context, string, string) error        { return nil }
func (f *fakeAPIKeys) TouchLastUsed(context.Context, string, string) error { return nil }
func (f *fakeAPIKeys) All(context.Context, string) ([]*domain.APIKey, error) {
	out := make([]*domain.APIKey, 0, len(f.byHash))
	for _, k := range f.byHash {
		out = append(out, k)
	}
	return out, nil
}

func TestListAPIKeysHandler(t *testing.T) {
	t.Parallel()
	key := domain.HydrateAPIKey("k1", "t1", "CI", "hash",
		[]domain.Permission{domain.PermSubscribersGet}, "u1", time.Now(), nil, nil)
	h := query.NewListAPIKeysHandler(&fakeAPIKeys{byHash: map[string]*domain.APIKey{"hash": key}})

	views, err := h.Handle(context.Background(), query.ListAPIKeys{TenantID: "t1"})
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, "CI", views[0].Name)
	require.Equal(t, []string{"subscribers:get"}, views[0].Permissions)
}

func TestAuthenticateAPIKeyResolvesPrincipal(t *testing.T) {
	t.Parallel()
	raw := "raw-api-key"
	key := domain.HydrateAPIKey("k1", "t1", "CI", token.Hash(raw),
		[]domain.Permission{domain.PermSubscribersGet}, "u1", time.Now(), nil, nil)
	h := query.NewAuthenticateAPIKeyHandler(
		&fakeAPIKeys{byHash: map[string]*domain.APIKey{token.Hash(raw): key}})

	p, err := h.Handle(context.Background(), query.AuthenticateAPIKey{TenantID: "t1", RawKey: raw})
	require.NoError(t, err)
	require.Equal(t, domain.PrincipalAPIKey, p.Kind())
	require.Equal(t, "k1", p.ActorID())
	require.True(t, p.Can(domain.PermSubscribersGet))
	require.False(t, p.Can(domain.PermSubscribersManage))
}

func TestAuthenticateAPIKeyRejectsRevokedKey(t *testing.T) {
	t.Parallel()
	raw := "revoked-key"
	revokedAt := time.Now()
	key := domain.HydrateAPIKey("k1", "t1", "CI", token.Hash(raw), nil, "u1",
		time.Now(), nil, &revokedAt)
	h := query.NewAuthenticateAPIKeyHandler(
		&fakeAPIKeys{byHash: map[string]*domain.APIKey{token.Hash(raw): key}})

	_, err := h.Handle(context.Background(), query.AuthenticateAPIKey{TenantID: "t1", RawKey: raw})
	require.ErrorIs(t, err, domain.ErrUnauthenticated)
}

func TestAuthenticateAPIKeyRejectsUnknownKey(t *testing.T) {
	t.Parallel()
	h := query.NewAuthenticateAPIKeyHandler(&fakeAPIKeys{byHash: map[string]*domain.APIKey{}})
	_, err := h.Handle(context.Background(), query.AuthenticateAPIKey{TenantID: "t1", RawKey: "ghost"})
	require.ErrorIs(t, err, domain.ErrUnauthenticated)
}

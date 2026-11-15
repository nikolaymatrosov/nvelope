package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// TestYDBListsDeleteRemovesListRoles covers the stand-in for a Postgres
// cascade that no audience port can observe: the iam per-list role
// assignments on a deleted list. Postgres deletes them through the
// user_list_roles foreign key, so the test runs on YDB only.
func TestYDBListsDeleteRemovesListRoles(t *testing.T) {
	t.Parallel()
	d := dbtest.YDBMigrated(t)
	ctx := context.Background()
	tenantID, userID := uuid.NewString(), uuid.NewString()
	lists := adapters.NewYDBLists(d)
	gone := addList(t, lists, tenantID, "Gone")
	kept := addList(t, lists, tenantID, "Kept")
	for _, listID := range []string{gone, kept} {
		require.NoError(t, d.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $list_id AS Utf8;
DECLARE $role_id AS Utf8;
INSERT INTO user_list_roles (tenant_id, user_id, list_id, role_id)
VALUES ($tenant_id, $user_id, $list_id, $role_id);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$user_id").Text(userID).
				Param("$list_id").Text(listID).
				Param("$role_id").Text(uuid.NewString()).
				Build())))
	}

	require.NoError(t, lists.Delete(ctx, tenantID, gone))

	rs, err := d.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT list_id FROM user_list_roles WHERE tenant_id = $tenant_id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build()))
	require.NoError(t, err)
	defer func() { _ = rs.Close(ctx) }()
	var left []string
	for row, err := range rs.Rows(ctx) {
		require.NoError(t, err)
		var listID string
		require.NoError(t, row.Scan(&listID))
		left = append(left, listID)
	}
	require.Equal(t, []string{kept}, left, "only the deleted list's role assignments go")
}

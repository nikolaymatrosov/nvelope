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

// TestYDBSubscribersDeleteRemovesCampaignRecipients covers the stand-in for
// the Postgres cascade from subscribers into campaign_recipients, which no
// audience port can observe. Other subscribers' recipient rows stay.
func TestYDBSubscribersDeleteRemovesCampaignRecipients(t *testing.T) {
	t.Parallel()
	d := dbtest.YDBMigrated(t)
	ctx := context.Background()
	tenantID, campaignID := uuid.NewString(), uuid.NewString()
	subscribers := adapters.NewYDBSubscribers(d)
	gone := addSubscriber(t, subscribers, tenantID, "gone@acme.com")
	kept := addSubscriber(t, subscribers, tenantID, "kept@acme.com")
	for _, r := range []struct{ subscriberID, email string }{
		{gone, "gone@acme.com"}, {kept, "kept@acme.com"},
	} {
		require.NoError(t, d.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $subscriber_id AS Utf8;
DECLARE $email AS Utf8;
INSERT INTO campaign_recipients
    (tenant_id, campaign_id, id, subscriber_id, email, status, failure_reason)
VALUES ($tenant_id, $campaign_id, $id, $subscriber_id, $email, "pending"u, ""u);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Param("$id").Text(uuid.NewString()).
				Param("$subscriber_id").Text(r.subscriberID).
				Param("$email").Text(r.email).
				Build())))
	}

	require.NoError(t, subscribers.Delete(ctx, tenantID, gone))

	rs, err := d.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT subscriber_id FROM campaign_recipients WHERE tenant_id = $tenant_id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build()))
	require.NoError(t, err)
	defer func() { _ = rs.Close(ctx) }()
	var left []string
	for row, err := range rs.Rows(ctx) {
		require.NoError(t, err)
		var subscriberID string
		require.NoError(t, row.Scan(&subscriberID))
		left = append(left, subscriberID)
	}
	require.Equal(t, []string{kept}, left, "only the deleted subscriber's recipient rows go")
}

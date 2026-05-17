package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

func TestJobKindsAreStable(t *testing.T) {
	t.Parallel()
	require.Equal(t, "audience.import", jobs.ImportArgs{}.Kind())
	require.Equal(t, "audience.export", jobs.ExportArgs{}.Kind())
	require.Equal(t, "domain.verify", jobs.DomainVerifyArgs{}.Kind())
	require.Equal(t, "campaign.start", jobs.CampaignStartArgs{}.Kind())
	require.Equal(t, "campaign.batch", jobs.CampaignBatchArgs{}.Kind())
}

func TestDomainVerifyArgsCarryOnlyIdentifiers(t *testing.T) {
	t.Parallel()
	args := jobs.DomainVerifyArgs{TenantID: "t1", DomainID: "d1"}
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	require.JSONEq(t, `{"tenant_id":"t1","domain_id":"d1"}`, string(raw))

	var back jobs.DomainVerifyArgs
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Equal(t, args, back)
}

func TestCampaignStartArgsCarryOnlyIdentifiers(t *testing.T) {
	t.Parallel()
	args := jobs.CampaignStartArgs{TenantID: "t1", CampaignID: "c1"}
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	require.JSONEq(t, `{"tenant_id":"t1","campaign_id":"c1"}`, string(raw))

	var back jobs.CampaignStartArgs
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Equal(t, args, back)
}

func TestCampaignBatchArgsCarrySliceBounds(t *testing.T) {
	t.Parallel()
	args := jobs.CampaignBatchArgs{TenantID: "t1", CampaignID: "c1", Offset: 500, Limit: 500}
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"tenant_id":"t1","campaign_id":"c1","offset":500,"limit":500}`, string(raw))

	var back jobs.CampaignBatchArgs
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Equal(t, args, back)
}

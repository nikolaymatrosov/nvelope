package postbox_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/platform/postbox"
)

// TestPostboxIntegration exercises the real Postbox API. It is skipped unless
// NVELOPE_POSTBOX_INTEGRATION is set, since it needs live credentials and a
// reachable Postbox account — routine CI uses the stubbed client tests instead.
func TestPostboxIntegration(t *testing.T) {
	if os.Getenv("NVELOPE_POSTBOX_INTEGRATION") == "" {
		t.Skip("set NVELOPE_POSTBOX_INTEGRATION to run the real Postbox integration test")
	}

	client, err := postbox.New(postbox.Config{
		Endpoint:        envOrDefault("NVELOPE_POSTBOX_ENDPOINT", "https://postbox.cloud.yandex.net"),
		Region:          envOrDefault("NVELOPE_POSTBOX_REGION", "ru-central1"),
		AccessKeyID:     os.Getenv("NVELOPE_POSTBOX_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("NVELOPE_POSTBOX_SECRET_ACCESS_KEY"),
	})
	require.NoError(t, err)

	domain := os.Getenv("NVELOPE_POSTBOX_TEST_DOMAIN")
	if domain == "" {
		t.Skip("set NVELOPE_POSTBOX_TEST_DOMAIN to exercise identity provisioning")
	}

	res, err := client.CreateEmailIdentity(context.Background(), domain)
	require.NoError(t, err)
	require.NotEmpty(t, res.DKIMTokens, "a provisioned domain identity returns DKIM tokens")

	_, err = client.GetEmailIdentity(context.Background(), domain)
	require.NoError(t, err)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

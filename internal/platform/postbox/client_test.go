package postbox_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/platform/postbox"
)

// newStubClient builds a Client pointed at an httptest server running handler.
func newStubClient(t *testing.T, handler http.HandlerFunc) *postbox.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := postbox.New(postbox.Config{
		Endpoint:        srv.URL,
		Region:          "ru-central1",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
	})
	require.NoError(t, err)
	return client
}

func TestNewRejectsIncompleteConfig(t *testing.T) {
	t.Parallel()
	_, err := postbox.New(postbox.Config{Region: "r", AccessKeyID: "a", SecretAccessKey: "s"})
	require.Error(t, err, "endpoint is required")
	_, err = postbox.New(postbox.Config{Endpoint: "https://x", Region: "r"})
	require.Error(t, err, "credentials are required")
}

func TestCreateEmailIdentityReturnsDKIMTokens(t *testing.T) {
	t.Parallel()
	client := newStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v2/email/identities", r.URL.Path)
		require.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256"),
			"request must be SigV4-signed")
		body, _ := io.ReadAll(r.Body)
		var in map[string]string
		require.NoError(t, json.Unmarshal(body, &in))
		require.Equal(t, "mail.acme.com", in["EmailIdentity"])
		_ = json.NewEncoder(w).Encode(map[string]any{
			"DkimAttributes": map[string]any{"Tokens": []string{"tok1", "tok2", "tok3"}},
		})
	})

	res, err := client.CreateEmailIdentity(context.Background(), "mail.acme.com")
	require.NoError(t, err)
	require.Equal(t, []string{"tok1", "tok2", "tok3"}, res.DKIMTokens)
}

func TestGetEmailIdentityParsesVerifiedStatus(t *testing.T) {
	t.Parallel()
	client := newStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v2/email/identities/mail.acme.com", r.URL.Path)
		require.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256"))
		_ = json.NewEncoder(w).Encode(map[string]any{"VerifiedForSendingStatus": true})
	})

	status, err := client.GetEmailIdentity(context.Background(), "mail.acme.com")
	require.NoError(t, err)
	require.True(t, status.Verified)
}

func TestSendEmailReturnsMessageID(t *testing.T) {
	t.Parallel()
	client := newStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v2/email/outbound-emails", r.URL.Path)
		require.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256"))
		body, _ := io.ReadAll(r.Body)
		var in struct {
			Content struct {
				Raw struct{ Data string } `json:"Raw"`
			} `json:"Content"`
		}
		require.NoError(t, json.Unmarshal(body, &in))
		require.NotEmpty(t, in.Content.Raw.Data, "raw message must be base64-encoded")
		_ = json.NewEncoder(w).Encode(map[string]string{"MessageId": "msg-123"})
	})

	ref, err := client.SendEmail(context.Background(), []byte("From: a@x\r\n\r\nhi"))
	require.NoError(t, err)
	require.Equal(t, "msg-123", ref)
}

func TestNon2xxResponseIsAnError(t *testing.T) {
	t.Parallel()
	client := newStubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad identity"}`))
	})

	_, err := client.CreateEmailIdentity(context.Background(), "bad")
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad identity")
}

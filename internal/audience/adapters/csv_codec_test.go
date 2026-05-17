package adapters_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func TestDecodeUploadCSV(t *testing.T) {
	t.Parallel()
	csv := "email,name,plan\na@example.com,Alice,pro\nb@example.com,Bob,free\n"
	rows, err := adapters.DecodeUpload("subs.csv", []byte(csv))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "a@example.com", rows[0].Email)
	require.Equal(t, "Alice", rows[0].Name)
	require.Equal(t, "pro", rows[0].Attributes["plan"], "non-reserved columns map to attributes")
}

func TestDecodeUploadZIP(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("subscribers.csv")
	require.NoError(t, err)
	_, err = f.Write([]byte("email,state\nz@example.com,enabled\n"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	rows, err := adapters.DecodeUpload("upload.zip", buf.Bytes())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "z@example.com", rows[0].Email)
	require.Equal(t, "enabled", rows[0].State)
}

func TestEncodeCSVRoundTrip(t *testing.T) {
	t.Parallel()
	attrs, err := domain.NewAttributes(map[string]any{"plan": "pro"})
	require.NoError(t, err)
	sub := domain.HydrateSubscriber("s1", "t1", "a@example.com", "Alice",
		domain.StateEnabled, attrs, timeZeroAdapters(), timeZeroAdapters())

	data, err := adapters.EncodeCSV([]*domain.Subscriber{sub})
	require.NoError(t, err)

	rows, err := adapters.DecodeUpload("export.csv", data)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "a@example.com", rows[0].Email)
	require.Equal(t, "Alice", rows[0].Name)
	require.Equal(t, "pro", rows[0].Attributes["plan"])
}

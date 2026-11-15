package urlattr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllowed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		attr, url string
		want      bool
	}{
		{"href", "https://a.example", true},
		{"href", "HTTP://a.example", true},
		{"href", "mailto:a@example.com", true},
		{"href", "tel:+100", true},
		{"href", "profile/a:b", true},
		{"href", "//a.example", true},
		{"href", "javascript:alert(1)", false},
		{"href", " \x01JavaScript:alert(1)", false},
		{"href", "java\tscr\nipt:alert(1)", false},
		{"href", "vbscript:x", false},
		{"href", "data:image/png;base64,AA", false},
		{"src", "data:image/png;base64,AA", true},
		{"src", "data:text/html,<b>", false},
	} {
		require.Equal(t, tc.want, allowed(tc.attr, tc.url), "%s=%q", tc.attr, tc.url)
	}
}

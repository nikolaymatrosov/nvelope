package visualrender

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// emailDoc is a typical hand-written email: doctype, head with meta and a
// responsive <style>, an MSO conditional comment, legacy table attributes,
// unrestricted inline style, a data: image, and an outbound link.
const emailDoc = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Hi</title>
<style>@media (max-width:600px){.col{width:100%!important}} td > p{margin:0}</style>
<!--[if mso]><table><tr><td width="600"><![endif]--></head>
<body bgcolor="#ffffff"><table width="600" cellpadding="0"><tr><td bgcolor="#eeeeee" style="padding:8px;position:relative">
<center><font color="#333333" face="Arial">Hello</font></center>
<img src="data:image/png;base64,iVBORw0KGgo=" alt="dot"><a href="https://acme.example/sale">Sale</a>
</td></tr></table></body></html>`

func TestSanitizeDocument_KeepsEmailDocument(t *testing.T) {
	t.Parallel()
	out, warnings := SanitizeDocument(emailDoc)
	require.Empty(t, warnings, "a clean email document raises no warning")
	for _, want := range []string{
		`<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN"`,
		`<meta charset="utf-8">`,
		`<meta name="viewport" content="width=device-width">`,
		`<style>@media (max-width:600px){.col{width:100%!important}} td > p{margin:0}</style>`,
		`<!--[if mso]>`,
		`<body bgcolor="#ffffff">`,
		`style="padding:8px;position:relative"`,
		`<center><font color="#333333" face="Arial">Hello</font></center>`,
		`src="data:image/png;base64,iVBORw0KGgo="`,
		`href="https://acme.example/sale"`,
	} {
		require.Contains(t, out, want)
	}
}

func TestSanitizeDocument_StripsActiveContent(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"script":          `<p>x</p><script>alert(1)</script>`,
		"event handler":   `<body onload="steal()"><p>x</p></body>`,
		"javascript href": `<a href="javascript:alert(1)">x</a>`,
		"data html href":  `<a href="data:text/html,<b>x</b>">x</a>`,
		"form":            `<form action="https://evil.example"><input name="card"><button>Pay</button></form>`,
		"iframe":          `<iframe src="https://evil.example"></iframe>`,
		"meta refresh":    `<head><meta http-equiv="refresh" content="0;url=https://evil.example"></head>`,
		"base":            `<head><base href="https://evil.example/"></head>`,
		"svg script":      `<svg><script>alert(1)</script></svg>`,
		"link":            `<link rel="stylesheet" href="https://evil.example/x.css">`,
		"js background":   `<table><tr><td background="javascript:alert(1)">x</td></tr></table>`,
	}
	banned := []string{"<script", "alert(", "onload", "javascript:", "data:text", "<form",
		"<input", "<button", "<iframe", "http-equiv", "<base", "<svg", "<link"}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out, warnings := SanitizeDocument(in)
			require.Len(t, warnings, 1, "stripping must raise the sanitizer_stripped warning")
			require.Equal(t, "sanitizer_stripped", warnings[0].Kind)
			for _, b := range banned {
				require.NotContains(t, out, b)
			}
		})
	}
}

func TestSanitizeDocument_NoFalseWarnings(t *testing.T) {
	t.Parallel()
	_, warnings := SanitizeDocument(`<p>Update your profile: metadata: none</p>`)
	require.Empty(t, warnings, "words ending in 'file:' or 'data:' are not URL schemes")
}

func TestSanitizeDocument_DoctypeCannotCarryMarkup(t *testing.T) {
	t.Parallel()
	out, _ := SanitizeDocument(`<!DOCTYPE html "><script>alert(1)</script>`)
	require.NotContains(t, out, "<script")
}

func TestSanitizeDocument_Idempotent(t *testing.T) {
	t.Parallel()
	once, _ := SanitizeDocument(emailDoc + `<script>x</script>`)
	twice, warnings := SanitizeDocument(once)
	require.Equal(t, once, twice)
	require.Empty(t, warnings)
}

func TestSanitizeDocument_Empty(t *testing.T) {
	t.Parallel()
	out, warnings := SanitizeDocument("")
	require.Empty(t, out)
	require.Nil(t, warnings)
}

func TestSanitizeDocument_KeepsMergeTags(t *testing.T) {
	t.Parallel()
	in := `<p>Hi {{ subscriber.first_name }} {{{rows}}}</p>` +
		`<a href="{{ campaign.unsubscribe_url }}">U</a><a href="{{url}}">V</a>`
	out, warnings := SanitizeDocument(in)
	require.Contains(t, out, `href="{{ campaign.unsubscribe_url }}"`)
	require.Contains(t, out, `href="{{url}}"`)
	require.Contains(t, out, `{{ subscriber.first_name }}`)
	require.Contains(t, out, `{{{rows}}}`)
	require.Empty(t, warnings)

	again, _ := SanitizeDocument(out)
	require.Equal(t, out, again)
}

func TestSanitizeDocument_UnsafePlaceholderNotProtected(t *testing.T) {
	t.Parallel()
	out, _ := SanitizeDocument(`<a href="{{ x"onmouseover="alert(1) }}">x</a>`)
	require.NotContains(t, out, "onmouseover")
}

func TestSanitizeDocument_LiteralSentinelCarriesNothing(t *testing.T) {
	t.Parallel()
	out, _ := SanitizeDocument(`<p>nvelope-merge-tag-0-x nvelope-merge-tag-99-x</p>`)
	require.Equal(t, `<p>nvelope-merge-tag-0-x nvelope-merge-tag-99-x</p>`, out)
}

func TestSanitizeDocument_LiteralSentinelDoesNotMoveATag(t *testing.T) {
	t.Parallel()
	// A literal sentinel must not pick up a real tag from elsewhere in the
	// document: that would relocate {{{rows}}} into the href.
	out, _ := SanitizeDocument(`<a href="nvelope-merge-tag-0-x">x</a><p>{{{rows}}}</p>`)
	require.Equal(t, `<a href="nvelope-merge-tag-0-x" rel="nofollow">x</a><p>{{{rows}}}</p>`, out)
}

func TestSanitizeDocument_CommentContentPreservedVerbatim(t *testing.T) {
	t.Parallel()
	out, warnings := SanitizeDocument(`<p>x</p><!--[if mso]><script>x</script><![endif]-->`)
	require.Contains(t, out, "<!--[if mso]><script>x</script><![endif]-->", "comment text is kept as-is")
	require.NotEmpty(t, warnings, "the warning stays advisory")
}

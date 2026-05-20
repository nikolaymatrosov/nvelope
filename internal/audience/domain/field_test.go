package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func TestNewField_Valid(t *testing.T) {
	t.Parallel()
	f, err := domain.NewField("tenant-1", "country", "Country", domain.FieldTypeText, "US", 7)
	require.NoError(t, err)
	require.Equal(t, "tenant-1", f.TenantID())
	require.Equal(t, "country", f.Slug())
	require.Equal(t, "Country", f.DisplayName())
	require.Equal(t, domain.FieldTypeText, f.Type())
	require.Equal(t, "US", f.DefaultValue())
	require.Equal(t, 7, f.Position())
	require.False(t, f.BuiltIn(), "builtIn must be false on construction")
}

func TestNewField_TenantRequired(t *testing.T) {
	t.Parallel()
	_, err := domain.NewField("", "country", "Country", domain.FieldTypeText, "", 0)
	require.Error(t, err)
}

func TestNewField_RejectsInvalidSlugs(t *testing.T) {
	t.Parallel()
	bad := []string{
		"",
		" ",
		"Country",               // uppercase
		"1country",              // starts with digit
		"_country",              // starts with underscore
		"co-untry",              // contains hyphen
		"co untry",              // contains space
		"country!",              // punctuation
		strings.Repeat("a", 64), // 64 chars — one over the limit
	}
	for _, slug := range bad {
		t.Run("slug="+slug, func(t *testing.T) {
			t.Parallel()
			_, err := domain.NewField("tenant-1", slug, "X", domain.FieldTypeText, "", 0)
			require.ErrorIs(t, err, domain.ErrFieldInvalidSlug, "got: %v", err)
		})
	}
}

func TestNewField_AcceptsBoundarySlugs(t *testing.T) {
	t.Parallel()
	good := []string{
		"a",                           // single letter
		"a1",                          // letter + digit
		"a_",                          // letter + underscore
		"plan_tier",                   // common case
		strings.Repeat("a", 63),       // 63 chars — at the limit
		"a" + strings.Repeat("1", 62), // letter + 62 digits
	}
	for _, slug := range good {
		t.Run("slug="+slug, func(t *testing.T) {
			t.Parallel()
			_, err := domain.NewField("tenant-1", slug, "X", domain.FieldTypeText, "", 0)
			require.NoError(t, err)
		})
	}
}

func TestNewField_RejectsUnknownType(t *testing.T) {
	t.Parallel()
	_, err := domain.NewField("tenant-1", "x", "X", domain.FieldType("rich_text"), "", 0)
	require.ErrorIs(t, err, domain.ErrFieldInvalidType)
}

func TestNewField_RejectsBadDisplayName(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"   ",
		strings.Repeat("a", 129),
	}
	for _, name := range cases {
		_, err := domain.NewField("tenant-1", "x", name, domain.FieldTypeText, "", 0)
		require.ErrorIs(t, err, domain.ErrFieldInvalidDisplayName)
	}
}

func TestField_RenameAndRetype(t *testing.T) {
	t.Parallel()
	f, err := domain.NewField("tenant-1", "x", "X", domain.FieldTypeText, "", 0)
	require.NoError(t, err)

	require.ErrorIs(t, f.Rename(""), domain.ErrFieldInvalidDisplayName)
	require.NoError(t, f.Rename("New name"))
	require.Equal(t, "New name", f.DisplayName())

	require.ErrorIs(t, f.Retype(domain.FieldType("nope")), domain.ErrFieldInvalidType)
	require.NoError(t, f.Retype(domain.FieldTypeNumber))
	require.Equal(t, domain.FieldTypeNumber, f.Type())
}

func TestField_SetDefaultValueAndReposition(t *testing.T) {
	t.Parallel()
	f, _ := domain.NewField("tenant-1", "x", "X", domain.FieldTypeText, "", 0)
	f.SetDefaultValue("hello")
	require.Equal(t, "hello", f.DefaultValue())
	f.Reposition(42)
	require.Equal(t, 42, f.Position())
}

func TestBuiltinFields_ListAndOrder(t *testing.T) {
	t.Parallel()
	bf := domain.BuiltinFields()
	require.Len(t, bf, 5)
	gotSlugs := make([]string, 0, len(bf))
	for _, f := range bf {
		require.True(t, f.BuiltIn(), "%s must be marked builtIn", f.Slug())
		require.Empty(t, f.TenantID(), "built-in pseudo-rows must not carry a tenant id")
		require.Equal(t, "builtin:"+f.Slug(), f.ID())
		gotSlugs = append(gotSlugs, f.Slug())
	}
	// Sanity-check the canonical display order from the data-model spec.
	require.Equal(t, []string{"first_name", "last_name", "email", "name", "state"}, gotSlugs)
}

func TestBuiltinFieldSlugs_MatchBuiltinFields(t *testing.T) {
	t.Parallel()
	want := domain.BuiltinFieldSlugs()
	got := make([]string, 0, len(want))
	for _, f := range domain.BuiltinFields() {
		got = append(got, f.Slug())
	}
	// Both sides should contain the same set, regardless of order.
	require.ElementsMatch(t, want, got)
}

func TestIsBuiltinFieldSlug(t *testing.T) {
	t.Parallel()
	for _, slug := range domain.BuiltinFieldSlugs() {
		require.True(t, domain.IsBuiltinFieldSlug(slug), slug)
	}
	require.False(t, domain.IsBuiltinFieldSlug("country"))
	require.False(t, domain.IsBuiltinFieldSlug(""))
}

func TestHydrateField_PreservesBuiltinFlag(t *testing.T) {
	t.Parallel()
	// HydrateField is the only path to set builtIn=true; sanity-check that it
	// round-trips.
	f := domain.HydrateField("id-1", "tenant-1", "first_name", "First name",
		domain.FieldTypeText, "", 0, true, time.Time{}, time.Time{})
	require.True(t, f.BuiltIn())
}

// Compile-time check that the typed errors are exported as expected.
var _ = []error{
	domain.ErrFieldInvalidSlug,
	domain.ErrFieldInvalidDisplayName,
	domain.ErrFieldInvalidType,
	domain.ErrFieldNotFound,
	domain.ErrFieldSlugTaken,
	domain.ErrFieldBuiltinSlug,
	domain.ErrFieldBuiltin,
	domain.ErrFieldReorderIncomplete,
}

var _ = errors.Is // ensure errors is referenced even if no test inlines errors.Is

package domain

import "time"

// BuiltinFieldSlug returns the canonical set of built-in subscriber-field
// slugs reserved by the platform. Tenant-defined custom fields cannot reuse
// these slugs.
func BuiltinFieldSlugs() []string {
	return []string{
		"email",
		"first_name",
		"last_name",
		"name",
		"state",
	}
}

// IsBuiltinFieldSlug reports whether the given slug names a built-in field.
func IsBuiltinFieldSlug(slug string) bool {
	switch slug {
	case "email", "first_name", "last_name", "name", "state":
		return true
	}
	return false
}

// BuiltinFields returns the canonical built-in subscriber-field pseudo-rows
// in their fixed display order. Each carries a stable synthetic id
// "builtin:<slug>" so consumers can render them in the same picker as
// tenant-defined custom fields without confusion. Built-in pseudo-rows are
// not stored in the subscriber_fields table; they are constructed here and
// prepended by the ListFields query handler.
func BuiltinFields() []*Field {
	type def struct {
		slug, display string
		t             FieldType
	}
	// Order here is the canonical display order returned by the query layer.
	defs := []def{
		{"first_name", "First name", FieldTypeText},
		{"last_name", "Last name", FieldTypeText},
		{"email", "Email", FieldTypeURL},
		{"name", "Full name", FieldTypeText},
		{"state", "State", FieldTypeText},
	}
	out := make([]*Field, 0, len(defs))
	for i, d := range defs {
		out = append(out, HydrateField(
			"builtin:"+d.slug, // synthetic id
			"",                // no tenant — built-ins are platform-wide
			d.slug, d.display, d.t,
			"",          // default value
			i,           // position
			true,        // builtIn
			time.Time{}, // createdAt
			time.Time{}, // updatedAt
		))
	}
	return out
}

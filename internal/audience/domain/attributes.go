package domain

import (
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// Attributes is free-form structured key/value data attached to a subscriber.
// It is a value object: it guarantees its content is well-formed — every key
// is a non-empty string, and every value is a JSON scalar, array, or nested
// object. No tenant-defined schema is enforced.
type Attributes struct {
	values map[string]any
}

// NewAttributes builds an Attributes value object from raw key/value data,
// rejecting an empty key or a value that is not JSON-representable.
func NewAttributes(raw map[string]any) (Attributes, error) {
	values := make(map[string]any, len(raw))
	for k, v := range raw {
		if strings.TrimSpace(k) == "" {
			return Attributes{}, apperr.NewIncorrectInput("validation_failed",
				"attribute keys must be non-empty")
		}
		if !jsonValue(v) {
			return Attributes{}, apperr.NewIncorrectInput("validation_failed",
				"attribute values must be scalars, arrays, or objects")
		}
		values[k] = v
	}
	return Attributes{values: values}, nil
}

// HydrateAttributes reconstructs an Attributes value object from a persisted
// jsonb document. Persistence only — it performs no validation.
func HydrateAttributes(raw map[string]any) Attributes {
	if raw == nil {
		raw = map[string]any{}
	}
	return Attributes{values: raw}
}

// Values returns the attribute map. The returned map must not be mutated.
func (a Attributes) Values() map[string]any {
	if a.values == nil {
		return map[string]any{}
	}
	return a.values
}

// Get returns the value for a key and whether it was present.
func (a Attributes) Get(key string) (any, bool) {
	v, ok := a.values[key]
	return v, ok
}

// jsonValue reports whether v is a JSON-representable value: a scalar, a
// []any array, or a map[string]any object.
func jsonValue(v any) bool {
	switch t := v.(type) {
	case nil, bool, string, float64, float32,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return true
	case []any:
		for _, e := range t {
			if !jsonValue(e) {
				return false
			}
		}
		return true
	case map[string]any:
		for _, e := range t {
			if !jsonValue(e) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// FieldView is the read model for one subscriber custom field (or built-in
// pseudo-row).
type FieldView struct {
	ID           string
	Slug         string
	DisplayName  string
	Type         string
	DefaultValue string
	Position     int
	BuiltIn      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func fieldView(f *domain.Field) FieldView {
	return FieldView{
		ID:           f.ID(),
		Slug:         f.Slug(),
		DisplayName:  f.DisplayName(),
		Type:         string(f.Type()),
		DefaultValue: f.DefaultValue(),
		Position:     f.Position(),
		BuiltIn:      f.BuiltIn(),
		CreatedAt:    f.CreatedAt(),
		UpdatedAt:    f.UpdatedAt(),
	}
}

// ListFields is the request to list every subscriber custom field for a
// tenant, prefixed by the platform's built-in pseudo-rows.
type ListFields struct {
	TenantID string
}

// ListFieldsHandler handles the ListFields query.
type ListFieldsHandler struct {
	fields domain.FieldRepository
}

// NewListFieldsHandler builds the handler, failing fast on a nil dependency.
func NewListFieldsHandler(fields domain.FieldRepository) ListFieldsHandler {
	if fields == nil {
		panic("nil field repository")
	}
	return ListFieldsHandler{fields: fields}
}

// Handle returns the built-in pseudo-rows followed by the tenant's custom
// fields in display order. The merge-tag picker and the Phase 6
// subscription-page "visible profile fields" picker both consume this list.
func (h ListFieldsHandler) Handle(ctx context.Context, q ListFields) ([]FieldView, error) {
	builtIns := domain.BuiltinFields()
	custom, err := h.fields.All(ctx, q.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]FieldView, 0, len(builtIns)+len(custom))
	for _, f := range builtIns {
		out = append(out, fieldView(f))
	}
	for _, f := range custom {
		out = append(out, fieldView(f))
	}
	return out, nil
}

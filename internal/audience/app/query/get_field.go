package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// GetField is the request to fetch one subscriber custom field by id.
type GetField struct {
	TenantID string
	FieldID  string
}

// GetFieldHandler handles the GetField query.
type GetFieldHandler struct {
	fields domain.FieldRepository
}

// NewGetFieldHandler builds the handler, failing fast on a nil dependency.
func NewGetFieldHandler(fields domain.FieldRepository) GetFieldHandler {
	if fields == nil {
		panic("nil field repository")
	}
	return GetFieldHandler{fields: fields}
}

// Handle returns the requested field's read model, or ErrFieldNotFound.
func (h GetFieldHandler) Handle(ctx context.Context, q GetField) (FieldView, error) {
	f, err := h.fields.Get(ctx, q.TenantID, q.FieldID)
	if err != nil {
		return FieldView{}, err
	}
	return fieldView(f), nil
}

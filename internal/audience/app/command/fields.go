package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// CreateField is the request to create a subscriber custom field.
type CreateField struct {
	TenantID     string
	Slug         string
	DisplayName  string
	Type         string
	DefaultValue string
	Position     int
}

// CreateFieldResult carries the new field's id.
type CreateFieldResult struct {
	FieldID string
}

// CreateFieldHandler handles the CreateField command.
type CreateFieldHandler struct {
	fields domain.FieldRepository
}

// NewCreateFieldHandler builds the handler, failing fast on a nil dependency.
func NewCreateFieldHandler(fields domain.FieldRepository) CreateFieldHandler {
	if fields == nil {
		panic("nil field repository")
	}
	return CreateFieldHandler{fields: fields}
}

// Handle validates through the domain constructor and persists the field.
func (h CreateFieldHandler) Handle(ctx context.Context, cmd CreateField) (CreateFieldResult, error) {
	if domain.IsBuiltinFieldSlug(cmd.Slug) {
		return CreateFieldResult{}, domain.ErrFieldBuiltinSlug
	}
	f, err := domain.NewField(cmd.TenantID, cmd.Slug, cmd.DisplayName,
		domain.FieldType(cmd.Type), cmd.DefaultValue, cmd.Position)
	if err != nil {
		return CreateFieldResult{}, err
	}
	id, err := h.fields.Add(ctx, cmd.TenantID, f)
	if err != nil {
		return CreateFieldResult{}, err
	}
	return CreateFieldResult{FieldID: id}, nil
}

// UpdateField is the request to rename, retype, change default, or reposition
// an existing subscriber custom field. The slug is immutable.
type UpdateField struct {
	TenantID     string
	FieldID      string
	DisplayName  string
	Type         string
	DefaultValue string
	Position     int
}

// UpdateFieldHandler handles the UpdateField command.
type UpdateFieldHandler struct {
	fields domain.FieldRepository
}

// NewUpdateFieldHandler builds the handler, failing fast on a nil dependency.
func NewUpdateFieldHandler(fields domain.FieldRepository) UpdateFieldHandler {
	if fields == nil {
		panic("nil field repository")
	}
	return UpdateFieldHandler{fields: fields}
}

// Handle applies the new attributes inside the tenant-bound transaction.
func (h UpdateFieldHandler) Handle(ctx context.Context, cmd UpdateField) error {
	return h.fields.Update(ctx, cmd.TenantID, cmd.FieldID,
		func(f *domain.Field) (*domain.Field, error) {
			if f.BuiltIn() {
				return nil, domain.ErrFieldBuiltin
			}
			if err := f.Rename(cmd.DisplayName); err != nil {
				return nil, err
			}
			if err := f.Retype(domain.FieldType(cmd.Type)); err != nil {
				return nil, err
			}
			f.SetDefaultValue(cmd.DefaultValue)
			f.Reposition(cmd.Position)
			return f, nil
		})
}

// DeleteField is the request to remove a subscriber custom field. Campaigns
// already rendered keep working; new saves referencing the slug fail.
type DeleteField struct {
	TenantID string
	FieldID  string
}

// DeleteFieldHandler handles the DeleteField command.
type DeleteFieldHandler struct {
	fields domain.FieldRepository
}

// NewDeleteFieldHandler builds the handler, failing fast on a nil dependency.
func NewDeleteFieldHandler(fields domain.FieldRepository) DeleteFieldHandler {
	if fields == nil {
		panic("nil field repository")
	}
	return DeleteFieldHandler{fields: fields}
}

// Handle deletes the field after refusing built-in pseudo-rows.
func (h DeleteFieldHandler) Handle(ctx context.Context, cmd DeleteField) error {
	// Load first so we can reject built-ins with a typed error instead of a
	// generic not-found from the DELETE statement.
	f, err := h.fields.Get(ctx, cmd.TenantID, cmd.FieldID)
	if err != nil {
		return err
	}
	if f.BuiltIn() {
		return domain.ErrFieldBuiltin
	}
	return h.fields.Delete(ctx, cmd.TenantID, cmd.FieldID)
}

// ReorderFields is the request to apply a new display ordering across every
// tenant-defined field. The supplied id list MUST cover every custom field
// exactly once; built-in pseudo-rows are not included.
type ReorderFields struct {
	TenantID string
	IDs      []string
}

// ReorderFieldsHandler handles the ReorderFields command.
type ReorderFieldsHandler struct {
	fields domain.FieldRepository
}

// NewReorderFieldsHandler builds the handler, failing fast on a nil dependency.
func NewReorderFieldsHandler(fields domain.FieldRepository) ReorderFieldsHandler {
	if fields == nil {
		panic("nil field repository")
	}
	return ReorderFieldsHandler{fields: fields}
}

// Handle validates completeness and applies the new positions atomically.
func (h ReorderFieldsHandler) Handle(ctx context.Context, cmd ReorderFields) error {
	all, err := h.fields.All(ctx, cmd.TenantID)
	if err != nil {
		return err
	}
	expected := map[string]bool{}
	for _, f := range all {
		expected[f.ID()] = true
	}
	if len(cmd.IDs) != len(expected) {
		return domain.ErrFieldReorderIncomplete
	}
	seen := map[string]bool{}
	positions := map[string]int{}
	for i, id := range cmd.IDs {
		if !expected[id] {
			return domain.ErrFieldReorderIncomplete
		}
		if seen[id] {
			return domain.ErrFieldReorderIncomplete
		}
		seen[id] = true
		positions[id] = i
	}
	return h.fields.Reorder(ctx, cmd.TenantID, positions)
}

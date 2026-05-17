// Package command holds the audience context's state-changing handlers, named
// in business language.
package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// CreateList is the request to create a list.
type CreateList struct {
	TenantID    string
	Name        string
	Description string
	Visibility  string
	OptIn       string
	Tags        []string
}

// CreateListResult carries the new list's id.
type CreateListResult struct {
	ListID string
}

// CreateListHandler handles the CreateList command.
type CreateListHandler struct {
	lists domain.ListRepository
}

// NewCreateListHandler builds the handler, failing fast on a nil dependency.
func NewCreateListHandler(lists domain.ListRepository) CreateListHandler {
	if lists == nil {
		panic("nil list repository")
	}
	return CreateListHandler{lists: lists}
}

// Handle validates the request through the domain constructor and persists the
// new list.
func (h CreateListHandler) Handle(ctx context.Context, cmd CreateList) (CreateListResult, error) {
	l, err := domain.NewList(cmd.TenantID, cmd.Name, cmd.Description,
		domain.Visibility(cmd.Visibility), domain.OptIn(cmd.OptIn), cmd.Tags)
	if err != nil {
		return CreateListResult{}, err
	}
	id, err := h.lists.Add(ctx, cmd.TenantID, l)
	if err != nil {
		return CreateListResult{}, err
	}
	return CreateListResult{ListID: id}, nil
}

// UpdateList is the request to rename, describe, and retag a list.
type UpdateList struct {
	TenantID    string
	ListID      string
	Name        string
	Description string
	Visibility  string
	OptIn       string
	Tags        []string
}

// UpdateListHandler handles the UpdateList command.
type UpdateListHandler struct {
	lists domain.ListRepository
}

// NewUpdateListHandler builds the handler, failing fast on a nil dependency.
func NewUpdateListHandler(lists domain.ListRepository) UpdateListHandler {
	if lists == nil {
		panic("nil list repository")
	}
	return UpdateListHandler{lists: lists}
}

// Handle applies the new list attributes inside the tenant-bound transaction.
func (h UpdateListHandler) Handle(ctx context.Context, cmd UpdateList) error {
	return h.lists.Update(ctx, cmd.TenantID, cmd.ListID,
		func(l *domain.List) (*domain.List, error) {
			if err := l.Rename(cmd.Name); err != nil {
				return nil, err
			}
			l.Describe(cmd.Description)
			l.Retag(cmd.Tags)
			if err := l.SetVisibility(domain.Visibility(cmd.Visibility)); err != nil {
				return nil, err
			}
			if err := l.SetOptIn(domain.OptIn(cmd.OptIn)); err != nil {
				return nil, err
			}
			return l, nil
		})
}

// DeleteList is the request to delete a list.
type DeleteList struct {
	TenantID string
	ListID   string
}

// DeleteListHandler handles the DeleteList command.
type DeleteListHandler struct {
	lists domain.ListRepository
}

// NewDeleteListHandler builds the handler, failing fast on a nil dependency.
func NewDeleteListHandler(lists domain.ListRepository) DeleteListHandler {
	if lists == nil {
		panic("nil list repository")
	}
	return DeleteListHandler{lists: lists}
}

// Handle deletes the list and cascades its memberships.
func (h DeleteListHandler) Handle(ctx context.Context, cmd DeleteList) error {
	return h.lists.Delete(ctx, cmd.TenantID, cmd.ListID)
}

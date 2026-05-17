package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// ListLists is the request for a page of the tenant's lists.
type ListLists struct {
	TenantID string
	Page     domain.Page
}

// ListListsHandler handles the ListLists query.
type ListListsHandler struct {
	lists domain.ListRepository
}

// NewListListsHandler builds the handler, failing fast on a nil dependency.
func NewListListsHandler(lists domain.ListRepository) ListListsHandler {
	if lists == nil {
		panic("nil list repository")
	}
	return ListListsHandler{lists: lists}
}

// Handle returns a page of the tenant's lists.
func (h ListListsHandler) Handle(ctx context.Context, q ListLists) (ListPage, error) {
	lists, total, err := h.lists.All(ctx, q.TenantID, q.Page)
	if err != nil {
		return ListPage{}, err
	}
	page := ListPage{Total: total, Lists: make([]ListView, 0, len(lists))}
	for _, l := range lists {
		page.Lists = append(page.Lists, listView(l))
	}
	return page, nil
}

// GetList is the request for a single list.
type GetList struct {
	TenantID string
	ListID   string
}

// GetListHandler handles the GetList query.
type GetListHandler struct {
	lists domain.ListRepository
}

// NewGetListHandler builds the handler, failing fast on a nil dependency.
func NewGetListHandler(lists domain.ListRepository) GetListHandler {
	if lists == nil {
		panic("nil list repository")
	}
	return GetListHandler{lists: lists}
}

// Handle returns the requested list, or domain.ErrListNotFound.
func (h GetListHandler) Handle(ctx context.Context, q GetList) (ListView, error) {
	l, err := h.lists.Get(ctx, q.TenantID, q.ListID)
	if err != nil {
		return ListView{}, err
	}
	return listView(l), nil
}

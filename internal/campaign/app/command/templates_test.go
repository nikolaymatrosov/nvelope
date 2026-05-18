package command_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

func TestDeleteTemplate(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	id := seedTemplate(t, templates, domain.KindTransactional)

	h := command.NewDeleteTemplateHandler(templates)
	require.NoError(t, h.Handle(context.Background(), command.DeleteTemplate{
		TenantID: "tenant-1", TemplateID: id,
	}))

	_, err := templates.Get(context.Background(), "tenant-1", id)
	require.ErrorIs(t, err, domain.ErrTemplateNotFound)
}

func TestDeleteTemplateNotFound(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()

	h := command.NewDeleteTemplateHandler(templates)
	err := h.Handle(context.Background(), command.DeleteTemplate{
		TenantID: "tenant-1", TemplateID: "missing",
	})
	require.ErrorIs(t, err, domain.ErrTemplateNotFound)
}

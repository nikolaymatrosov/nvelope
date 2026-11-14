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

func TestCreateTemplateSanitizesRawHTML(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	h := command.NewCreateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true})
	res, err := h.Handle(context.Background(), command.CreateTemplate{
		TenantID: "tenant-1", Name: "Reset", Kind: "transactional", Subject: "Reset",
		BodyHTML: `<p>Reset</p><form action="https://evil.example"><input name="pw"></form>`,
	})
	require.NoError(t, err)
	tpl, err := templates.Get(context.Background(), "tenant-1", res.TemplateID)
	require.NoError(t, err)
	require.NotContains(t, tpl.BodyHTML(), "<form")
	require.Len(t, res.Warnings, 1)
}

func TestUpdateTemplateSanitizesRawHTML(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	id := seedTemplate(t, templates, domain.KindCampaign)
	h := command.NewUpdateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true})
	res, err := h.Handle(context.Background(), command.UpdateTemplate{
		TenantID: "tenant-1", TemplateID: id, Name: "Tpl", Subject: "S",
		BodyHTML: `<p>ok</p><script>alert(1)</script>`,
	})
	require.NoError(t, err)
	tpl, err := templates.Get(context.Background(), "tenant-1", id)
	require.NoError(t, err)
	require.NotContains(t, tpl.BodyHTML(), "<script")
	require.Len(t, res.Warnings, 1)
}

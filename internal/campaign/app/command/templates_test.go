package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

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
	h := command.NewCreateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true}, &fakeMediaPublisher{})
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
	h := command.NewUpdateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true}, &fakeMediaPublisher{})
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

const (
	libImage = "https://lib.test/x.png"
	pubImage = "https://pub.test/x.png"
)

func TestCreateTransactionalTemplatePublishesMedia(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	media := &fakeMediaPublisher{from: libImage, to: pubImage}
	h := command.NewCreateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true}, media)
	res, err := h.Handle(context.Background(), command.CreateTemplate{
		TenantID: "tenant-1", Name: "Receipt", Kind: "transactional", Subject: "Receipt",
		BodyHTML: `<p><img src="` + libImage + `"></p>`, BodyText: "logo " + libImage,
	})
	require.NoError(t, err)
	tpl, err := templates.Get(context.Background(), "tenant-1", res.TemplateID)
	require.NoError(t, err)
	require.Contains(t, tpl.BodyHTML(), pubImage)
	require.Equal(t, "logo "+pubImage, tpl.BodyText())
}

func TestCreateCampaignTemplateDoesNotPublish(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	media := &fakeMediaPublisher{from: libImage, to: pubImage}
	h := command.NewCreateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true}, media)
	res, err := h.Handle(context.Background(), command.CreateTemplate{
		TenantID: "tenant-1", Name: "Newsletter", Kind: "campaign", Subject: "News",
		BodyHTML: `<p><img src="` + libImage + `"></p>`,
	})
	require.NoError(t, err)
	tpl, err := templates.Get(context.Background(), "tenant-1", res.TemplateID)
	require.NoError(t, err)
	require.Contains(t, tpl.BodyHTML(), libImage, "campaign templates stay private")
	require.Zero(t, media.calls)
}

func TestUpdateTransactionalTemplateWithMissingMediaKeepsRow(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	id := seedTemplate(t, templates, domain.KindTransactional)
	missing := errors.New("media missing")
	h := command.NewUpdateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true},
		&fakeMediaPublisher{err: missing})
	_, err := h.Handle(context.Background(), command.UpdateTemplate{
		TenantID: "tenant-1", TemplateID: id, Name: "T", Subject: "S",
		BodyHTML: `<p><img src="` + libImage + `"></p>`,
	})
	require.ErrorIs(t, err, missing)
	tpl, err := templates.Get(context.Background(), "tenant-1", id)
	require.NoError(t, err)
	require.Equal(t, "<p>tpl</p>", tpl.BodyHTML())
}

func TestSaveVisualTransactionalTemplatePublishesMedia(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	stamp := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	templates.byID["tpl-v"] = domain.HydrateTemplate("tpl-v", "tenant-1", "Receipt",
		domain.KindTransactional, "Receipt", "<p>old</p>", "old", nil, nil, stamp, stamp)

	media := &fakeMediaPublisher{from: libImage, to: pubImage}
	h := command.NewSaveVisualTemplateHandler(templates, fakeFieldsProvider{},
		mediaPrefixValidator{prefix: "https://lib.test/"}, media)
	_, err := h.Handle(context.Background(), command.SaveVisualTemplate{
		TenantID: "tenant-1", TemplateID: "tpl-v", Subject: "Receipt",
		Doc: &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
			domain.Image{MediaRef: libImage, Alt: "logo"},
		}},
		BodyHTML:          `<p><img src="` + libImage + `" alt="logo"></p>`,
		BodyText:          "logo",
		IfUnmodifiedSince: stamp,
	})
	require.NoError(t, err)
	require.Contains(t, templates.byID["tpl-v"].BodyHTML(), pubImage)
	require.Equal(t, 1, media.calls)
}

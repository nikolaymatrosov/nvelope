package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

func TestNewMediaAsset_imagesNeedDimensions(t *testing.T) {
	_, err := domain.NewMediaAsset("id", "t1", "a.png", "image/png", 10, 100, 0, 0, "media/t1/id/a.png", "")
	require.Error(t, err)

	pdf, err := domain.NewMediaAsset("id", "t1", "a.pdf", "application/pdf", 10, 100, 0, 0, "media/t1/id/a.pdf", "")
	require.NoError(t, err)
	require.Zero(t, pdf.Width())
	require.False(t, pdf.IsPublished())
}

func TestNewMediaAsset_rejectsSVG(t *testing.T) {
	_, err := domain.NewMediaAsset("id", "t1", "a.svg", "image/svg+xml", 10, 100, 5, 5, "media/t1/id/a.svg", "")
	require.ErrorIs(t, err, domain.ErrUnsupportedMediaType)
}

func TestMediaAsset_Disposition(t *testing.T) {
	img, err := domain.NewMediaAsset("id", "t1", "a.png", "image/png", 10, 100, 5, 5, "k", "")
	require.NoError(t, err)
	require.Equal(t, "inline", img.Disposition())

	pdf, err := domain.NewMediaAsset("id", "t1", "price list.pdf", "application/pdf", 10, 100, 0, 0, "k", "")
	require.NoError(t, err)
	require.Equal(t, `attachment; filename="price list.pdf"`, pdf.Disposition())

	ru, err := domain.NewMediaAsset("id", "t1", "прайс.pdf", "application/pdf", 10, 100, 0, 0, "k", "")
	require.NoError(t, err)
	require.Equal(t, "attachment; filename*=utf-8''%D0%BF%D1%80%D0%B0%D0%B9%D1%81.pdf", ru.Disposition())
}

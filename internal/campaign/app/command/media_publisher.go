package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// MediaPublisher makes the media-library assets that content references
// publicly reachable and returns the content with their library URLs
// rewritten to the public copies, in the order given. It fails with the
// media context's media_missing error, publishing nothing, when a referenced
// asset no longer exists. Wired in internal/service over the media context.
type MediaPublisher interface {
	Publish(ctx context.Context, tenantID string, texts ...string) ([]string, error)
}

// publishIfTransactional publishes the media a transactional template's
// bodies reference. Such a template is live once saved, because the API can
// send it at any moment. Campaign templates stay private until a campaign
// built from them starts.
func publishIfTransactional(ctx context.Context, media MediaPublisher, tenantID string,
	kind domain.Kind, bodyHTML, bodyText string) (string, string, error) {
	if kind != domain.KindTransactional {
		return bodyHTML, bodyText, nil
	}
	out, err := media.Publish(ctx, tenantID, bodyHTML, bodyText)
	if err != nil {
		return "", "", err
	}
	return out[0], out[1], nil
}

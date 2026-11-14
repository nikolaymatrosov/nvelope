package command

import "context"

// MediaPublisher makes the media-library assets that content references
// publicly reachable and returns the content with their library URLs
// rewritten to the public copies, in the order given. It fails with the
// media context's media_missing error, publishing nothing, when a referenced
// asset no longer exists. Wired in internal/service over the media context.
type MediaPublisher interface {
	Publish(ctx context.Context, tenantID string, texts ...string) ([]string, error)
}

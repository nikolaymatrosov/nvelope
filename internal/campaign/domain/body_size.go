package domain

// MaxBodyBytes caps a campaign or template body, HTML or text. Gmail clips
// messages past ~102 KB, so legitimate mail sits far below this; the cap
// only stops abuse.
const MaxBodyBytes = 1 << 20

// checkBodySize rejects an HTML or text body over MaxBodyBytes.
func checkBodySize(bodyHTML, bodyText string) error {
	if len(bodyHTML) > MaxBodyBytes || len(bodyText) > MaxBodyBytes {
		return ErrBodyTooLarge
	}
	return nil
}

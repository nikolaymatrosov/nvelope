package domain

// VisualDoc is the structured, block-based representation of a campaign's or
// template's content that the visual editor authors. It is the editor's
// source of truth, persisted as JSON alongside the rendered HTML and plain
// text. The Phase 3 send pipeline reads only the rendered HTML and plain
// text; the visual document never reaches the worker.
//
// Persistence carries a version number so future shape changes can be
// detected. Version 1 is the only currently-supported shape.
type VisualDoc struct {
	Version int
	Nodes   []Node
}

// Node is a top-level block of the visual document or a child block inside a
// container (a column, a list item, a quote). The sealed visualNode method
// keeps the set closed to the block types declared in this file.
type Node interface {
	visualNode()
}

// Inline is a piece of content that lives inside a text-bearing block. The
// sealed visualInline method keeps the set closed.
type Inline interface {
	visualInline()
}

// Marks describes the optional formatting applied to a Text node.
type Marks struct {
	Bold      bool
	Italic    bool
	Underline bool
	Strike    bool
	// Color is a CSS color value (e.g. "#cc0000", "rgb(0,0,0)", named).
	// Empty means no explicit color.
	Color string
	// Link is the href the text should be wrapped in. Empty means no link.
	Link string
}

// ── Blocks ──────────────────────────────────────────────────────────────

// Paragraph is a block of prose. Children are text-and-mergetag inlines.
type Paragraph struct {
	Children []Inline
}

// Heading is a block-level heading. Level is 1, 2, or 3.
type Heading struct {
	Level    int
	Children []Inline
}

// BulletList is an unordered list of items.
type BulletList struct {
	Items []ListItem
}

// OrderedList is an ordered list of items.
type OrderedList struct {
	Items []ListItem
}

// ListItem is a single item in a bulleted or ordered list. An item may
// contain other blocks (a paragraph, a nested list, …).
type ListItem struct {
	Children []Node
}

// Quote is a block-quote. May contain any other block type.
type Quote struct {
	Children []Node
}

// Code is a fenced code block. The text is rendered verbatim — no marks
// apply.
type Code struct {
	Text string
}

// Image references a tenant media-library asset. Href is the optional link
// the image is wrapped in.
type Image struct {
	MediaRef string
	Alt      string
	Href     string
}

// Button is a call-to-action. The renderer emits a table-based clickable
// element so Outlook desktop renders it correctly.
type Button struct {
	Label string
	Href  string
}

// Divider is a horizontal rule.
type Divider struct{}

// Columns is a multi-column row. Cols carries 2, 3, or 4 sub-streams of
// blocks — one per column.
type Columns struct {
	Cols [][]Node
}

// RawHTML is an opaque region of verbatim HTML. Used to host (a) pre-existing
// raw-HTML content during opt-in migration to the visual editor (per
// FR-031), and (b) code-view edits the editor cannot round-trip into
// structured blocks (per FR-027). The renderer passes the bytes through
// after sanitization.
type RawHTML struct {
	HTML string
}

// ── Inlines ─────────────────────────────────────────────────────────────

// Text is a run of characters carrying optional marks.
type Text struct {
	Text  string
	Marks Marks
}

// MergeTagNamespace is the namespace of a merge tag — currently either
// "subscriber" (for fields in the tenant's subscriber custom-field registry,
// including built-in pseudo-rows) or "campaign" (for the platform's
// allow-list of campaign-level values).
type MergeTagNamespace string

const (
	// MergeTagSubscriber is the subscriber-field namespace.
	MergeTagSubscriber MergeTagNamespace = "subscriber"
	// MergeTagCampaign is the campaign-level value namespace.
	MergeTagCampaign MergeTagNamespace = "campaign"
)

// MergeTag is an inline placeholder. On render the literal
// "{{ namespace.key }}" string is emitted; substitution is performed by the
// send pipeline at send time per recipient.
type MergeTag struct {
	Namespace MergeTagNamespace
	Key       string
}

// AllowedCampaignMergeTags is the platform's fixed allow-list of values
// available in the "campaign" namespace. Used by Validate to reject
// references to unknown campaign-level names at save time.
//
// The list is intentionally small and platform-controlled — tenants cannot
// extend it. Add a new entry here and the renderer + substitutor at the
// same time.
var AllowedCampaignMergeTags = map[string]bool{
	"unsubscribe_url":     true,
	"preference_url":      true,
	"archive_url":         true,
	"view_in_browser_url": true,
	"tenant_name":         true,
	"current_date":        true,
}

// ── Sealed-interface markers ────────────────────────────────────────────
//
// These keep Node and Inline closed sets. New block/inline types must add
// the corresponding marker here.

func (Paragraph) visualNode()   {}
func (Heading) visualNode()     {}
func (BulletList) visualNode()  {}
func (OrderedList) visualNode() {}
func (Quote) visualNode()       {}
func (Code) visualNode()        {}
func (Image) visualNode()       {}
func (Button) visualNode()      {}
func (Divider) visualNode()     {}
func (Columns) visualNode()     {}
func (RawHTML) visualNode()     {}

func (Text) visualInline()     {}
func (MergeTag) visualInline() {}

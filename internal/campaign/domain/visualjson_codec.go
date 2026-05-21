package domain

import (
	"encoding/json"
	"fmt"
)

// MarshalVisualDoc encodes a typed VisualDoc into the canonical wire shape
// documented in contracts/tenant-api.md § "Structured-document JSON schema":
//
//	{ "version": 1, "type": "doc", "content": [Block, …] }
//
// Each typed block emits its `type` discriminator plus its attrs / content;
// inlines emit `type: "text"` with a `marks` array or `type: "mergeTag"` with
// `attrs: { namespace, key }`. The shape matches what the TipTap editor reads
// on reload and what the BFF renderer consumes, so a doc round-trips losslessly
// through Go without going via the BFF.
func MarshalVisualDoc(d *VisualDoc) (json.RawMessage, error) {
	if d == nil {
		return nil, nil
	}
	envelope := docEnvelope{
		Version: d.Version,
		Type:    "doc",
		Content: make([]json.RawMessage, 0, len(d.Nodes)),
	}
	for _, n := range d.Nodes {
		raw, err := marshalNode(n)
		if err != nil {
			return nil, err
		}
		envelope.Content = append(envelope.Content, raw)
	}
	return json.Marshal(envelope)
}

// docEnvelope is the top-level wire shape.
type docEnvelope struct {
	Version int               `json:"version"`
	Type    string            `json:"type"`
	Content []json.RawMessage `json:"content"`
}

func marshalNode(n Node) (json.RawMessage, error) {
	switch v := n.(type) {
	case Paragraph:
		return marshalBlock("paragraph", nil, marshalInlines(v.Children))
	case Heading:
		return marshalBlock("heading",
			map[string]any{"level": v.Level},
			marshalInlines(v.Children),
		)
	case BulletList:
		content, err := marshalListItems(v.Items)
		if err != nil {
			return nil, err
		}
		return marshalBlock("bulletList", nil, content)
	case OrderedList:
		content, err := marshalListItems(v.Items)
		if err != nil {
			return nil, err
		}
		return marshalBlock("orderedList", nil, content)
	case Quote:
		content, err := marshalBlockChildren(v.Children)
		if err != nil {
			return nil, err
		}
		return marshalBlock("blockquote", nil, content)
	case Code:
		// codeBlock carries a single text inline with no marks (per the
		// schema in contracts/tenant-api.md).
		inner, err := json.Marshal([]any{map[string]any{"type": "text", "text": v.Text}})
		if err != nil {
			return nil, err
		}
		return marshalBlock("codeBlock", nil, inner)
	case Image:
		return marshalBlockNoContent("image", map[string]any{
			"mediaRef": v.MediaRef,
			"alt":      v.Alt,
			"href":     v.Href,
		})
	case Button:
		return marshalBlockNoContent("button", map[string]any{
			"label": v.Label,
			"href":  v.Href,
		})
	case Divider:
		return marshalBlockNoContent("divider", nil)
	case Columns:
		cols := make([]json.RawMessage, 0, len(v.Cols))
		for _, col := range v.Cols {
			children, err := marshalBlockChildren(col)
			if err != nil {
				return nil, err
			}
			column, err := marshalBlock("column", nil, children)
			if err != nil {
				return nil, err
			}
			cols = append(cols, column)
		}
		colsContent, err := json.Marshal(cols)
		if err != nil {
			return nil, err
		}
		return marshalBlock("columns",
			map[string]any{"count": len(v.Cols)},
			colsContent,
		)
	case RawHTML:
		return marshalBlockNoContent("rawHtml", map[string]any{"html": v.HTML})
	default:
		return nil, fmt.Errorf("visualdoc: unknown node type %T", n)
	}
}

func marshalListItems(items []ListItem) (json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(items))
	for _, it := range items {
		children, err := marshalBlockChildren(it.Children)
		if err != nil {
			return nil, err
		}
		item, err := marshalBlock("listItem", nil, children)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return json.Marshal(out)
}

func marshalBlockChildren(children []Node) (json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(children))
	for _, child := range children {
		raw, err := marshalNode(child)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return json.Marshal(out)
}

func marshalInlines(items []Inline) json.RawMessage {
	out := make([]json.RawMessage, 0, len(items))
	for _, in := range items {
		raw, err := marshalInline(in)
		if err != nil {
			continue
		}
		out = append(out, raw)
	}
	encoded, _ := json.Marshal(out)
	return encoded
}

func marshalInline(in Inline) (json.RawMessage, error) {
	switch v := in.(type) {
	case Text:
		obj := map[string]any{"type": "text", "text": v.Text}
		if marks := marshalMarks(v.Marks); marks != nil {
			obj["marks"] = marks
		}
		return json.Marshal(obj)
	case MergeTag:
		return json.Marshal(map[string]any{
			"type": "mergeTag",
			"attrs": map[string]any{
				"namespace": string(v.Namespace),
				"key":       v.Key,
			},
		})
	default:
		return nil, fmt.Errorf("visualdoc: unknown inline type %T", in)
	}
}

func marshalMarks(m Marks) []map[string]any {
	var out []map[string]any
	if m.Bold {
		out = append(out, map[string]any{"type": "bold"})
	}
	if m.Italic {
		out = append(out, map[string]any{"type": "italic"})
	}
	if m.Underline {
		out = append(out, map[string]any{"type": "underline"})
	}
	if m.Strike {
		out = append(out, map[string]any{"type": "strike"})
	}
	if m.Color != "" {
		out = append(out, map[string]any{
			"type":  "color",
			"attrs": map[string]any{"color": m.Color},
		})
	}
	if m.Link != "" {
		out = append(out, map[string]any{
			"type":  "link",
			"attrs": map[string]any{"href": m.Link},
		})
	}
	return out
}

// marshalBlock emits a block object with a content array (raw JSON) and
// optional attrs.
func marshalBlock(kind string, attrs map[string]any, content json.RawMessage) (json.RawMessage, error) {
	obj := map[string]any{"type": kind}
	if attrs != nil {
		obj["attrs"] = attrs
	}
	if content != nil {
		obj["content"] = content
	}
	return json.Marshal(obj)
}

// marshalBlockNoContent emits a block object with only attrs and no content
// array (used by Image, Button, Divider, RawHTML).
func marshalBlockNoContent(kind string, attrs map[string]any) (json.RawMessage, error) {
	obj := map[string]any{"type": kind}
	if attrs != nil {
		obj["attrs"] = attrs
	}
	return json.Marshal(obj)
}

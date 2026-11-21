package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"reflect"
)

const GridSchemaVersion = "1"

// Grid is the stable presentation contract returned by feature endpoints.
// Product packages build it explicitly so response shape never depends on key-name inference.
type Grid struct {
	Layout        string `json:"layout"`
	SchemaVersion string `json:"schema_version"`
	Total         int    `json:"total"`
	Items         []Item `json:"items"`
}
type Field struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Value  any    `json:"value"`
	Status string `json:"status"`
	Format string `json:"format,omitempty"`
}
type Source struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}
type Item struct {
	ID       string         `json:"id"`
	Kind     string         `json:"kind"`
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle"`
	Fields   []Field        `json:"fields"`
	Sources  []Source       `json:"sources"`
	Cautions []any          `json:"cautions"`
	Data     map[string]any `json:"data"`
}

func NewGrid(items []Item) Grid {
	if items == nil {
		items = []Item{}
	}
	return Grid{Layout: "grid", SchemaVersion: GridSchemaVersion, Total: len(items), Items: items}
}
func Available(key, label string, value any, format string) Field {
	return Field{Key: key, Label: label, Value: value, Status: "available", Format: format}
}
func Optional(key, label string, value any, format string) Field {
	missing := value == nil
	if text, ok := value.(string); ok && text == "" {
		missing = true
	}
	if !missing {
		rv := reflect.ValueOf(value)
		missing = (rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface || rv.Kind() == reflect.Slice || rv.Kind() == reflect.Map) && rv.IsNil()
	}
	if missing {
		return Field{Key: key, Label: label, Value: nil, Status: "unknown", Format: format}
	}
	return Available(key, label, value, format)
}
func URLSource(raw string) []Source {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return []Source{}
	}
	return []Source{{Label: u.Hostname(), URL: raw}}
}
func NewItem(kind, stableID, title, subtitle string, fields []Field, sources []Source, cautions []any, data map[string]any) Item {
	if fields == nil {
		fields = []Field{}
	}
	if sources == nil {
		sources = []Source{}
	}
	if cautions == nil {
		cautions = []any{}
	}
	if data == nil {
		data = map[string]any{}
	}
	if stableID == "" {
		encoded, _ := json.Marshal([]any{kind, title, subtitle, data})
		hash := sha256.Sum256(encoded)
		stableID = hex.EncodeToString(hash[:12])
	}
	return Item{ID: stableID, Kind: kind, Title: title, Subtitle: subtitle, Fields: fields, Sources: sources, Cautions: cautions, Data: data}
}
func SourceGrid(urls []string) Grid {
	items := []Item{}
	seen := map[string]bool{}
	for _, raw := range urls {
		sources := URLSource(raw)
		if len(sources) == 0 || seen[raw] {
			continue
		}
		seen[raw] = true
		items = append(items, NewItem("research_source", raw, sources[0].Label, "Public research source", []Field{Available("source_url", "Source", raw, "url")}, sources, []any{map[string]any{"code": "source_requires_review", "message": "This source is research evidence, not a verified provider match or final price."}}, map[string]any{"source_url": raw}))
	}
	return NewGrid(items)
}

// Present remains for the shared encoder; product projection is explicit.
func Present(value any) any { return value }

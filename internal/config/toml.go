package config

import (
	"bytes"
	"errors"
	"slices"

	"github.com/pelletier/go-toml/v2"

	// shortcut: unstable has no semver promise; it is pinned at v2.4.3 and used from this file only.
	"github.com/pelletier/go-toml/v2/unstable"
)

// tomlKey is one key of a TOML document with its byte positions in the prepared buffer.
type tomlKey struct {
	Path   []string // full key path incl. current table prefix, e.g. ["mcp_servers","a","command"]
	Offset int      // byte offset of the LAST key part
	Header bool     // from a [table] or [[array]] header
	// Header: Start is the '[' opening the header line, End is just past that line's '\n' (or EOF).
	// Key-value: the `key = value` expression span, trailing comment excluded.
	Start, End int
}

// tomlKeys lists every table header and key-value of src in document order, recursing into inline tables.
// It returns the parser error for invalid TOML.
func tomlKeys(src []byte) ([]tomlKey, error) {
	var (
		keys   []tomlKey
		prefix []string
		p      = unstable.Parser{KeepComments: true}
	)
	p.Reset(src)
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind { //nolint:exhaustive // top-level expressions are only tables, key-values and comments
		case unstable.Table, unstable.ArrayTable:
			path, last := keyParts(e, nil)
			lineStart := bytes.LastIndexByte(src[:last], '\n') + 1
			start := lineStart + bytes.IndexByte(src[lineStart:last], '[')
			end := len(src)
			if i := bytes.IndexByte(src[last:], '\n'); i >= 0 {
				end = last + i + 1
			}
			prefix = path
			keys = append(keys, tomlKey{Path: path, Offset: last, Header: true, Start: start, End: end})
		case unstable.KeyValue:
			keys = appendKeyValue(keys, e, prefix)
		default: // Comment: Key() panics on it
		}
	}
	return keys, p.Error()
}

// appendKeyValue appends the KeyValue kv under prefix, then its inline-table children.
func appendKeyValue(keys []tomlKey, kv *unstable.Node, prefix []string) []tomlKey {
	path, last := keyParts(kv, prefix)
	keys = append(keys, tomlKey{Path: path, Offset: last, Start: int(kv.Raw.Offset), End: int(kv.Raw.Offset + kv.Raw.Length)})
	if v := kv.Value(); v.Kind == unstable.InlineTable {
		it := v.Children()
		for it.Next() {
			if n := it.Node(); n.Kind == unstable.KeyValue { // multi-line inline tables also hold Comment nodes
				keys = appendKeyValue(keys, n, path)
			}
		}
	}
	return keys
}

// keyParts returns prefix + the decoded key parts of n and the offset of the last part.
func keyParts(n *unstable.Node, prefix []string) (path []string, last int) {
	path = slices.Clone(prefix)
	it := n.Key()
	for it.Next() {
		k := it.Node()
		path = append(path, string(k.Data))
		last = int(k.Raw.Offset)
	}
	return path, last
}

// TOMLDoc is a decoded TOML document with key positions and original section bytes.
type TOMLDoc struct {
	BOM    bool
	Values map[string]any // toml.Unmarshal result: string, int64, float64, bool, []any, map[string]any, time values
	body   []byte         // BOM-stripped input, without the newline prepare may append
	keys   []tomlKey
}

// ParseTOML decodes src. It never returns an error: doc is nil iff a KindSyntax problem is returned.
// Problem messages are fixed texts; positions are on the BOM-stripped buffer.
func ParseTOML(src []byte) (*TOMLDoc, []Problem) {
	body, hasBOM, addedNL := prepare(src)
	var probs []Problem
	if hasBOM {
		probs = append(probs, Problem{Kind: KindBOM, Line: 1, Col: 1, Message: MsgBOM})
	}
	values := map[string]any{}
	if err := toml.Unmarshal(body, &values); err != nil {
		p := Problem{Kind: KindSyntax, Message: MsgSyntax}
		var de *toml.DecodeError
		if errors.As(err, &de) {
			p.Line, p.Col = de.Position()
			p.Message = syntaxMessage(de.Error()) // never de.String(): it prints source lines
		}
		return nil, append(probs, p)
	}
	keys, err := tomlKeys(body)
	if err != nil {
		keys = nil // unreachable after a successful Unmarshal; keep the values without positions
	}
	if addedNL {
		body = body[:len(body)-1]
	}
	return &TOMLDoc{BOM: hasBOM, Values: values, body: body, keys: keys}, probs
}

// Pos is the 1-based line:col of the first key whose path equals path, or of the first key under path
// (dotted shape: the start of its `a.b.c = v` expression); 0,0 when absent.
func (d *TOMLDoc) Pos(path ...string) (line, col int) {
	for _, k := range d.keys {
		if slices.Equal(k.Path, path) {
			return LineCol(d.body, k.Offset)
		}
	}
	for _, k := range d.keys {
		if tomlUnder(k.Path, path) {
			return LineCol(d.body, k.Start)
		}
	}
	return 0, 0
}

// Bytes is the original section of table path, nil when absent. Header shape: from its header to the next
// header outside path after the last header under path (or EOF), so sub-tables placed after other tables
// stay inside. Inline and dotted shapes: from the first to the last key-value under path.
// Limits (docs/decisions/0001): separated sub-tables and interleaved dotted keys yield the covering range,
// which then holds the unrelated tables in between; comment lines just above the next header belong to
// the preceding span.
func (d *TOMLDoc) Bytes(path ...string) []byte {
	for i, k := range d.keys {
		if !k.Header || !slices.Equal(k.Path, path) {
			continue
		}
		last := i
		for j := i + 1; j < len(d.keys); j++ {
			if d.keys[j].Header && tomlUnder(d.keys[j].Path, path) {
				last = j
			}
		}
		end := len(d.body)
		for _, n := range d.keys[last+1:] {
			if n.Header && !tomlUnder(n.Path, path) {
				end = n.Start
				break
			}
		}
		return d.body[k.Start:min(end, len(d.body))]
	}
	start, end := -1, -1
	for _, k := range d.keys {
		if !k.Header && tomlUnder(k.Path, path) {
			if start < 0 {
				start = k.Start
			}
			end = max(end, k.End)
		}
	}
	if start < 0 {
		return nil
	}
	return d.body[start:min(end, len(d.body))]
}

// tomlUnder reports whether p equals prefix or lies below it.
func tomlUnder(p, prefix []string) bool {
	return len(p) >= len(prefix) && slices.Equal(p[:len(prefix)], prefix)
}

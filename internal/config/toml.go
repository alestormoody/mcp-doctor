package config

import (
	"bytes"
	"slices"

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
			keys = appendKeyValue(keys, it.Node(), path)
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

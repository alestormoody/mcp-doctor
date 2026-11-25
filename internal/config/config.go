// Package config decodes JSON, JSONC and TOML bytes already read by the caller into documents with
// byte positions. It never touches the filesystem and never copies file content into a message.
package config

import (
	"bytes"
	"strings"
)

// Kind classifies a format-level Problem.
type Kind string

// Problem kinds.
const (
	KindSyntax      Kind = "syntax"      // the document cannot be parsed: no document is returned
	KindNonStandard Kind = "nonstandard" // JSON with comments or trailing commas (only when strict)
	KindBOM         Kind = "bom"         // leading UTF-8 BOM, stripped before parsing
	KindDuplicate   Kind = "duplicate"   // duplicate object key (JSON only; TOML duplicates are KindSyntax)
	KindType        Kind = "type"        // a fragment has the wrong JSON type (from Member.Decode)
)

// Problem is one format-level issue. Message is always one of the fixed Msg* texts; Key is an object key
// name (an identifier, never a value) for KindDuplicate. Positions are 1-based byte columns computed on
// the BOM-stripped buffer; 0 when unknown.
type Problem struct {
	Kind              Kind
	Line, Col         int
	Message           string
	Key               string
	PrevLine, PrevCol int // KindDuplicate: first occurrence
}

// Fixed problem messages: library error texts can echo file content, so they are never shown.
const (
	MsgEmpty       = "file is empty"
	MsgEOF         = "unexpected end of file"
	MsgChar        = "invalid character"
	MsgComma       = "missing comma or closing bracket"
	MsgEscape      = "invalid string escape"
	MsgDuplicate   = "duplicate key"
	MsgValue       = "invalid value"
	MsgDepth       = "nesting too deep"
	MsgType        = "value has the wrong type"
	MsgSyntax      = "syntax error"
	MsgNonStandard = "comments or trailing commas"
	MsgBOM         = "file starts with a byte order mark"
)

var messages = map[string]bool{
	MsgEmpty: true, MsgEOF: true, MsgChar: true, MsgComma: true, MsgEscape: true, MsgDuplicate: true,
	MsgValue: true, MsgDepth: true, MsgType: true, MsgSyntax: true, MsgNonStandard: true, MsgBOM: true,
}

var bom = []byte("\xEF\xBB\xBF")

// LineCol converts byte offset n of b to a 1-based line and byte column (same convention as hujson and
// go-toml). n is clamped to [0, len(b)].
func LineCol(b []byte, n int) (line, col int) {
	n = max(0, min(n, len(b)))
	line = 1 + bytes.Count(b[:n], []byte("\n"))
	col = 1 + n - (bytes.LastIndexByte(b[:n], '\n') + 1)
	return line, col
}

// prepare strips a leading UTF-8 BOM (both libraries reject it) and appends "\n" to a non-empty body
// that does not end in one (hujson rejects a line comment at EOF without a newline).
func prepare(src []byte) (body []byte, hasBOM, addedNL bool) {
	body, hasBOM = bytes.CutPrefix(src, bom)
	if len(body) > 0 && body[len(body)-1] != '\n' {
		body = append(bytes.Clone(body), '\n')
		addedNL = true
	}
	return body, hasBOM, addedNL
}

// unprepare is the exact inverse of prepare.
func unprepare(body []byte, hasBOM, addedNL bool) []byte {
	if addedNL {
		body = body[:len(body)-1]
	}
	if hasBOM {
		body = append(bytes.Clone(bom), body...)
	}
	return body
}

// syntaxMessage maps a library error text to a fixed message. It never returns any part of errText.
func syntaxMessage(errText string) string {
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(errText, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("unexpected EOF", "unexpected end"):
		return MsgEOF
	case has("max depth"):
		return MsgDepth
	case has("already defined", "already exists"):
		return MsgDuplicate
	case has("escape"):
		return MsgEscape
	case has("after object key", "after array element", "expected"):
		return MsgComma
	case has("invalid character", "invalid literal"):
		return MsgChar
	case has("invalid"):
		return MsgValue
	default:
		return MsgSyntax
	}
}

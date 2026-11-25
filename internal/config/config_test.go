package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestLineCol(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		n         int
		line, col int
	}{
		{"start", "abc", 0, 1, 1},
		{"after newline", "a\nb", 2, 2, 1},
		{"crlf", "a\r\nb", 3, 2, 1},
		{"multi-byte counts bytes", "é!", 2, 1, 3},
		{"beyond end clamps", "ab\nc", 99, 2, 2},
		{"negative clamps", "ab", -1, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, col := LineCol([]byte(tt.in), tt.n)
			if line != tt.line || col != tt.col {
				t.Errorf("LineCol(%q, %d) = %d:%d, want %d:%d", tt.in, tt.n, line, col, tt.line, tt.col)
			}
		})
	}
}

func TestPrepare(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		body         string
		bom, addedNL bool
	}{
		{"plain", "{}\n", "{}\n", false, false},
		{"bom", "\xEF\xBB\xBF{}\n", "{}\n", true, false},
		{"no final newline", "{}", "{}\n", false, true},
		{"bom and no newline", "\xEF\xBB\xBF{}", "{}\n", true, true},
		{"crlf kept", strings.ReplaceAll("{\n}\n", "\n", "\r\n"), "{\r\n}\r\n", false, false},
		{"empty", "", "", false, false},
		{"bom only", "\xEF\xBB\xBF", "", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, bom, nl := prepare([]byte(tt.in))
			if string(body) != tt.body || bom != tt.bom || nl != tt.addedNL {
				t.Errorf("prepare(%q) = %q, %v, %v; want %q, %v, %v", tt.in, body, bom, nl, tt.body, tt.bom, tt.addedNL)
			}
			if got := unprepare(body, bom, nl); !bytes.Equal(got, []byte(tt.in)) {
				t.Errorf("unprepare(prepare(%q)) = %q", tt.in, got)
			}
		})
	}
}

func TestSyntaxMessage(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hujson: line 1, column 2: parsing value: unexpected EOF", MsgEOF},
		{"unexpected end of JSON input", MsgEOF},
		{"exceeded max depth", MsgDepth},
		{"toml: key a is already defined", MsgDuplicate},
		{"table already exists", MsgDuplicate},
		{"invalid escape in string", MsgEscape},
		{"invalid character 'x' after object key", MsgComma},
		{"invalid character '}' after array element", MsgComma},
		{"invalid character 'x' after object key:value pair", MsgComma},
		{"expected comma", MsgComma},
		{"invalid character 'x' looking for beginning of value", MsgChar},
		{"invalid literal: CANARY-abcdef", MsgChar},
		{"invalid number", MsgValue},
		{"something else CANARY-abcdef", MsgSyntax},
		{"", MsgSyntax},
	}
	for _, tt := range tests {
		got := syntaxMessage(tt.in)
		if got != tt.want {
			t.Errorf("syntaxMessage(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if strings.Contains(got, "CANARY") || !messages[got] {
			t.Errorf("syntaxMessage(%q) = %q is not a fixed message", tt.in, got)
		}
	}
}

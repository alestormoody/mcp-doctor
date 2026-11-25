package config

import (
	"bytes"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const tomlDir = "testdata/toml"

// tomlParse parses src and fails the test on any problem other than a BOM.
func tomlParse(t *testing.T, src []byte) *TOMLDoc {
	t.Helper()
	doc, probs := ParseTOML(src)
	for _, p := range probs {
		if p.Kind != KindBOM {
			t.Fatalf("unexpected problem %+v", p)
		}
	}
	if doc == nil {
		t.Fatal("nil doc")
	}
	return doc
}

// tomlServerValue returns mcp_servers.<name>.<field> from the decoded values.
func tomlServerValue(doc *TOMLDoc, name, field string) any {
	servers, _ := doc.Values["mcp_servers"].(map[string]any)
	entry, _ := servers[name].(map[string]any)
	return entry[field]
}

func TestParseTOMLShapes(t *testing.T) {
	corpus := readCorpus(t, tomlDir)
	cases := []struct {
		file, name              string
		serverLine, serverCol   int
		commandLine, commandCol int
	}{
		{"table.toml", "a", 4, 14, 5, 1},
		{"inline.toml", "a", 2, 1, 2, 7},
		{"dotted.toml", "a", 2, 1, 2, 15},
		{"quoted.toml", "my.server", 1, 14, 2, 1},
		{"unicode.toml", "café", 2, 14, 3, 1},
		{"no-newline.toml", "a", 1, 14, 2, 1},
	}
	for _, c := range cases {
		for v, src := range variants(corpus[c.file]) {
			t.Run(c.file+"/"+v, func(t *testing.T) {
				doc := tomlParse(t, src)
				if doc.BOM != (v == "bom") {
					t.Errorf("BOM = %v", doc.BOM)
				}
				if got := tomlServerValue(doc, c.name, "command"); got == nil {
					t.Errorf("command missing: %v", doc.Values)
				}
				servers, _ := doc.Values["mcp_servers"].(map[string]any)
				if _, ok := servers[c.name]; !ok {
					t.Errorf("server %q missing: %v", c.name, slices.Collect(maps.Keys(servers)))
				}
				if l, col := doc.Pos("mcp_servers", c.name); l != c.serverLine || col != c.serverCol {
					t.Errorf("Pos(server) = %d:%d, want %d:%d", l, col, c.serverLine, c.serverCol)
				}
				if l, col := doc.Pos("mcp_servers", c.name, "command"); l != c.commandLine || col != c.commandCol {
					t.Errorf("Pos(command) = %d:%d, want %d:%d", l, col, c.commandLine, c.commandCol)
				}
				if l, col := doc.Pos("mcp_servers", "absent"); l != 0 || col != 0 {
					t.Errorf("Pos(absent) = %d:%d", l, col)
				}
				if b := doc.Bytes("mcp_servers", "absent"); b != nil {
					t.Errorf("Bytes(absent) = %q", b)
				}
			})
		}
	}
	for _, f := range []string{"table.toml", "inline.toml", "dotted.toml"} {
		if got := tomlServerValue(tomlParse(t, corpus[f]), "a", "command"); got != "x" {
			t.Errorf("%s: command = %v, want x", f, got)
		}
	}
	quoted := tomlParse(t, corpus["quoted.toml"])
	if servers, _ := quoted.Values["mcp_servers"].(map[string]any); len(servers) != 1 {
		t.Errorf("quoted: servers = %v, want one key my.server", servers)
	}
}

func TestTOMLBytes(t *testing.T) {
	corpus := readCorpus(t, tomlDir)
	between := func(src []byte, from, to string) []byte {
		i := bytes.Index(src, []byte(from))
		j := len(src)
		if to != "" {
			j = bytes.Index(src, []byte(to))
		}
		return src[i:j]
	}
	cases := []struct {
		file, variant, from, to string
	}{
		{"subtable.toml", "lf", "[mcp_servers.a]", "[mcp_servers.b]"},
		{"subtable.toml", "bom", "[mcp_servers.a]", "[mcp_servers.b]"},
		{"table.toml", "crlf", "[mcp_servers.a]", "[mcp_servers.b]"},
		{"inline.toml", "lf", "a = {", "\nb = {"},
		{"dotted.toml", "crlf", "mcp_servers.a.command", "\r\nmcp_servers.b"},
		{"no-newline.toml", "lf", "[mcp_servers.a]", ""},
		{"no-newline.toml", "bom", "[mcp_servers.a]", ""},
	}
	for _, c := range cases {
		t.Run(c.file+"/"+c.variant, func(t *testing.T) {
			src := variants(corpus[c.file])[c.variant]
			want := between(src, c.from, c.to)
			if got := tomlParse(t, src).Bytes("mcp_servers", "a"); !bytes.Equal(got, want) {
				t.Errorf("Bytes = %q, want %q", got, want)
			}
		})
	}
	if got := tomlParse(t, corpus["subtable.toml"]).Bytes("mcp_servers", "b"); string(got) != "[mcp_servers.b]\ncommand = \"y\"\n" {
		t.Errorf("last table Bytes = %q", got)
	}
	if got := tomlParse(t, corpus["table.toml"]).Bytes("mcp_servers", "a"); !bytes.Contains(got, []byte("\n")) || bytes.Contains(got, []byte("\r")) {
		t.Errorf("lf Bytes = %q", got)
	}
	if got := tomlParse(t, crlfVariant(corpus["table.toml"])).Bytes("mcp_servers", "a"); !bytes.Contains(got, []byte("\r\n")) {
		t.Errorf("crlf Bytes lost CRLF: %q", got)
	}
}

func TestParseTOMLProblems(t *testing.T) {
	corpus := readCorpus(t, tomlDir)
	cases := []struct {
		file      string
		msg       string // "" = any fixed message
		line, col int    // 0 = any position > 0
	}{
		{"dup-key.toml", MsgDuplicate, 3, 1},
		{"dup-table.toml", MsgDuplicate, 4, 2},
		{"bad-string.toml", "", 0, 0},
		{"bad-value.toml", "", 0, 0},
	}
	for _, c := range cases {
		for v, src := range variants(corpus[c.file]) {
			t.Run(c.file+"/"+v, func(t *testing.T) {
				doc, probs := ParseTOML(src)
				if doc != nil {
					t.Fatal("doc != nil on a syntax error")
				}
				var syn *Problem
				for i, p := range probs {
					if strings.Contains(p.Message, "CANARY") || !messages[p.Message] {
						t.Errorf("unsafe message %q", p.Message)
					}
					if p.Kind == KindSyntax {
						syn = &probs[i]
					}
				}
				if syn == nil {
					t.Fatalf("no syntax problem: %+v", probs)
				}
				if c.msg != "" && syn.Message != c.msg {
					t.Errorf("message = %q, want %q", syn.Message, c.msg)
				}
				if c.line == 0 && (syn.Line <= 0 || syn.Col <= 0) || c.line != 0 && (syn.Line != c.line || syn.Col != c.col) {
					t.Errorf("position = %d:%d, want %d:%d", syn.Line, syn.Col, c.line, c.col)
				}
				if hasBOM := slices.ContainsFunc(probs, func(p Problem) bool { return p.Kind == KindBOM }); hasBOM != (v == "bom") {
					t.Errorf("BOM problem = %v", hasBOM)
				}
			})
		}
	}
}

func TestParseTOMLEmptyAndBOM(t *testing.T) {
	doc, probs := ParseTOML(readCorpus(t, tomlDir)["empty.toml"])
	if doc == nil || len(probs) != 0 || len(doc.Values) != 0 || doc.Values == nil {
		t.Fatalf("empty: doc=%v probs=%v", doc, probs)
	}
	if doc.Bytes("mcp_servers") != nil {
		t.Error("empty: Bytes != nil")
	}
	src := readCorpus(t, tomlDir)["table.toml"]
	doc, probs = ParseTOML(bomVariant(src))
	if doc == nil || len(probs) != 1 || probs[0] != (Problem{Kind: KindBOM, Line: 1, Col: 1, Message: MsgBOM}) {
		t.Fatalf("bom: probs = %+v", probs)
	}
	if plain := tomlParse(t, src); !reflect.DeepEqual(plain.Values, doc.Values) {
		t.Errorf("bom values differ: %v vs %v", plain.Values, doc.Values)
	}
	if plain := tomlParse(t, src); !reflect.DeepEqual(plain.Values, tomlParse(t, crlfVariant(src)).Values) {
		t.Error("crlf values differ")
	}
}

func TestTOMLZeroDoc(t *testing.T) {
	// A doc without keys (tomlKeys failed after Unmarshal succeeded) must still be safe.
	d := &TOMLDoc{}
	if l, c := d.Pos("a"); l != 0 || c != 0 || d.Bytes("a") != nil {
		t.Error("zero doc not safe")
	}
}

func FuzzParseTOML(f *testing.F) {
	seeds := readCorpus(f, tomlDir)
	for name, b := range readCorpus(f, spikeDir) {
		if strings.HasSuffix(name, ".toml") {
			seeds["spike/"+name] = b
		}
	}
	for _, b := range seeds {
		for _, v := range variants(b) {
			f.Add(v)
		}
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		doc, probs := ParseTOML(src)
		syntax := false
		for _, p := range probs {
			if p.Line < 0 || p.Col < 0 || !messages[p.Message] {
				t.Fatalf("bad problem %+v", p)
			}
			syntax = syntax || p.Kind == KindSyntax
		}
		if (doc == nil) != syntax {
			t.Fatalf("doc nil = %v, syntax problem = %v", doc == nil, syntax)
		}
		if doc == nil {
			return
		}
		orig, _ := bytes.CutPrefix(src, []byte("\xEF\xBB\xBF"))
		for _, k := range doc.keys {
			doc.Pos(k.Path...)
			if b := doc.Bytes(k.Path...); b != nil && !bytes.Contains(orig, b) {
				t.Fatalf("Bytes(%q) = %q not in input", k.Path, b)
			}
		}
	})
}

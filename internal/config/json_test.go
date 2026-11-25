package config

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

const jsonDir = "testdata/json"

// jsonCheckProblems fails on any problem with a message outside the allowlist or echoing a canary.
func jsonCheckProblems(t *testing.T, name string, ps []Problem) {
	t.Helper()
	for _, p := range ps {
		if !messages[p.Message] || strings.Contains(p.Message, "CANARY") {
			t.Errorf("%s: unsafe message %q", name, p.Message)
		}
	}
}

// jsonKinds returns the problems of kind k.
func jsonKinds(ps []Problem, k Kind) []Problem {
	var out []Problem
	for _, p := range ps {
		if p.Kind == k {
			out = append(out, p)
		}
	}
	return out
}

func TestParseJSONValid(t *testing.T) {
	corpus := readCorpus(t, jsonDir)
	for _, tc := range []struct {
		file         string
		strictNonStd int
	}{
		{"valid.json", 0},
		{"comments.jsonc", 1},
		{"trailing.jsonc", 1},
		{"unicode.jsonc", 1},
		{"eof-comment.jsonc", 1},
	} {
		for v, src := range variants(corpus[tc.file]) {
			name := tc.file + "/" + v
			for _, strict := range []bool{false, true} {
				doc, ps := ParseJSON(src, strict)
				jsonCheckProblems(t, name, ps)
				if doc == nil || len(jsonKinds(ps, KindSyntax)) > 0 {
					t.Fatalf("%s: doc=%v problems=%+v", name, doc, ps)
				}
				want := 0
				if strict {
					want = tc.strictNonStd
				}
				if got := len(jsonKinds(ps, KindNonStandard)); got != want {
					t.Errorf("%s strict=%v: %d nonstandard problems, want %d", name, strict, got, want)
				}
				wantBOM := 0
				if v == "bom" {
					wantBOM = 1
				}
				if bp := jsonKinds(ps, KindBOM); len(bp) != wantBOM || (wantBOM == 1 && (bp[0].Line != 1 || bp[0].Col != 1 || bp[0].Message != MsgBOM)) {
					t.Errorf("%s: bom problems %+v", name, bp)
				}
				if doc.BOM != (v == "bom") {
					t.Errorf("%s: BOM = %v", name, doc.BOM)
				}
			}
		}
	}
}

func TestParseJSONNonStandardPosition(t *testing.T) {
	corpus := readCorpus(t, jsonDir)
	for _, tc := range []struct {
		file      string
		line, col int
	}{
		{"comments.jsonc", 1, 1},  // "// user servers"
		{"trailing.jsonc", 5, 27}, // the first trailing comma, after "server.js"
	} {
		_, ps := ParseJSON(corpus[tc.file], true)
		ns := jsonKinds(ps, KindNonStandard)
		if len(ns) != 1 || ns[0].Line != tc.line || ns[0].Col != tc.col || ns[0].Message != MsgNonStandard {
			t.Errorf("%s: %+v, want %d:%d", tc.file, ns, tc.line, tc.col)
		}
	}
}

func TestParseJSONErrors(t *testing.T) {
	corpus := readCorpus(t, jsonDir)
	for _, tc := range []struct {
		file      string
		line, col int
		msg       string
	}{
		{"truncated.json", 4, 1, MsgEOF},
		{"bad-literal.json", 3, 24, MsgChar},
		{"missing-comma.json", 5, 7, MsgChar}, // hujson: "after object value"
		{"bad-escape.json", 4, 18, MsgChar},
		{"empty.json", 1, 1, MsgEmpty},
		{"blank.json", 1, 1, MsgEmpty},
	} {
		for _, strict := range []bool{false, true} {
			doc, ps := ParseJSON(corpus[tc.file], strict)
			jsonCheckProblems(t, tc.file, ps)
			syn := jsonKinds(ps, KindSyntax)
			if doc != nil || len(syn) != 1 {
				t.Fatalf("%s: doc=%v problems=%+v", tc.file, doc, ps)
			}
			if p := syn[0]; p.Line != tc.line || p.Col != tc.col || p.Message != tc.msg {
				t.Errorf("%s: got %d:%d %q, want %d:%d %q", tc.file, p.Line, p.Col, p.Message, tc.line, tc.col, tc.msg)
			}
		}
	}
}

func TestParseJSONErrorVariants(t *testing.T) {
	// CRLF keeps line numbers; BOM keeps line 1 column 1 as the first character after the mark.
	src := readCorpus(t, jsonDir)["bad-literal.json"]
	for v, in := range variants(src) {
		_, ps := ParseJSON(in, false)
		syn := jsonKinds(ps, KindSyntax)
		if len(syn) != 1 || syn[0].Line != 3 || syn[0].Col != 24 {
			t.Errorf("%s: %+v", v, ps)
		}
	}
	_, ps := ParseJSON(bomVariant([]byte("x")), false)
	if syn := jsonKinds(ps, KindSyntax); len(syn) != 1 || syn[0].Line != 1 || syn[0].Col != 1 {
		t.Errorf("bom: %+v", ps)
	}
}

func TestParseJSONDepth(t *testing.T) {
	// Trailing comma on line 1 proves offsets survive standardization.
	src := "{\"pad\": [1,],\n\"mcpServers\":{\"a\":{\"x\": " + strings.Repeat("[", 10001) + strings.Repeat("]", 10001) + "}}}\n"
	doc, ps := ParseJSON([]byte(src), false)
	jsonCheckProblems(t, "depth", ps)
	syn := jsonKinds(ps, KindSyntax)
	// Depth 10001 is the 9998th '[' (three objects above it); the first '[' is at column 25.
	if doc != nil || len(syn) != 1 || syn[0].Line != 2 || syn[0].Col != 25+9997 || syn[0].Message != MsgDepth {
		t.Errorf("doc=%v problems=%+v", doc, ps)
	}
}

func TestJSONMembersOrder(t *testing.T) {
	src := readCorpus(t, jsonDir)["comments.jsonc"]
	for v, in := range variants(src) {
		doc, _ := ParseJSON(in, false)
		root := doc.Root()
		if root.Line != 2 || root.Col != 1 || !root.IsObject() {
			t.Errorf("%s: root %+v", v, root)
		}
		top, ok, ps := root.Members()
		if !ok || len(ps) != 0 || len(top) != 1 || top[0].Name != "mcpServers" || top[0].Line != 3 || top[0].Col != 3 {
			t.Fatalf("%s: top = %+v %v %+v", v, top, ok, ps)
		}
		servers, ok, _ := top[0].Members()
		got := [][3]any{}
		for _, m := range servers {
			got = append(got, [3]any{m.Name, m.Line, m.Col})
		}
		if want := [][3]any{{"a", 5, 5}, {"b", 9, 5}}; !ok || !slices.Equal(got, want) {
			t.Errorf("%s: servers = %v, want %v", v, got, want)
		}
		fields, _, _ := servers[0].Members()
		if len(fields) != 2 || fields[0].Name != "command" || fields[1].Name != "args" || fields[1].Line != 7 {
			t.Errorf("%s: fields = %+v", v, fields)
		}
		if ms, ok, ps := fields[1].Members(); ms != nil || ok || ps != nil || fields[1].IsObject() {
			t.Errorf("%s: array Members = %v %v %v", v, ms, ok, ps)
		}
	}
}

func TestJSONDuplicateKeys(t *testing.T) {
	corpus := readCorpus(t, jsonDir)

	doc, ps := ParseJSON(corpus["dup-server.json"], true)
	if len(ps) != 0 {
		t.Fatalf("parse problems %+v", ps)
	}
	top, _, _ := doc.Root().Members()
	servers, _, ps := top[0].Members()
	want := Problem{Kind: KindDuplicate, Line: 5, Col: 5, Message: MsgDuplicate, Key: "a", PrevLine: 3, PrevCol: 5}
	if len(ps) != 1 || ps[0] != want {
		t.Errorf("problems = %+v, want %+v", ps, want)
	}
	if len(servers) != 2 || servers[0].Name != "b" || servers[1].Name != "a" || servers[1].Line != 5 ||
		!bytes.Contains(servers[1].Bytes(), []byte("second")) {
		t.Errorf("servers = %+v", servers)
	}

	doc, _ = ParseJSON(corpus["dup-field.json"], true)
	top, _, _ = doc.Root().Members()
	servers, _, ps = top[0].Members()
	if len(ps) != 0 {
		t.Errorf("server level problems %+v", ps)
	}
	fields, _, ps := servers[0].Members()
	want = Problem{Kind: KindDuplicate, Line: 5, Col: 7, Message: MsgDuplicate, Key: "command", PrevLine: 4, PrevCol: 7}
	if len(ps) != 1 || ps[0] != want || len(fields) != 1 {
		t.Errorf("fields = %+v problems = %+v", fields, ps)
	}
	var s string
	if p := fields[0].Decode(&s); p != nil || s != "second" {
		t.Errorf("last wins: %q %+v", s, p)
	}
}

// jsonFind walks object members by name.
func jsonFind(t *testing.T, m Member, names ...string) Member {
	t.Helper()
	for _, n := range names {
		ms, _, _ := m.Members()
		i := slices.IndexFunc(ms, func(x Member) bool { return x.Name == n })
		if i < 0 {
			t.Fatalf("no member %q", n)
		}
		m = ms[i]
	}
	return m
}

func TestJSONDecodeFragmentOffset(t *testing.T) {
	src := readCorpus(t, jsonDir)["nested-type.json"]
	for v, in := range variants(src) {
		doc, _ := ParseJSON(in, false)
		args := jsonFind(t, doc.Root(), "mcpServers", "a", "args")
		p := args.Decode(&[]string{})
		if p == nil || p.Kind != KindType || p.Line != 3 || p.Col != 38 || p.Message != MsgType {
			t.Errorf("%s: %+v, want type at 3:38", v, p)
		}
		// A wrong-typed field inside the decoded fragment is positioned past its value (best effort).
		var entry struct {
			Args []string `json:"args"`
		}
		p = jsonFind(t, doc.Root(), "mcpServers", "a").Decode(&entry)
		if p == nil || p.Kind != KindType || p.Line != 3 || p.Col != 59 {
			t.Errorf("%s: field %+v, want type at 3:59", v, p)
		}
	}

	// Syntax errors in a fragment: Offset counts bytes read, so the offending byte is Offset-1.
	doc, _ := ParseJSON([]byte("{\n  \"a\": [1, 2]\n}\n"), false)
	a := jsonFind(t, doc.Root(), "a")
	frag := Member{doc: doc, v: a.v, Start: a.Start + 2, End: a.End} // ", 2]"
	p := frag.Decode(new(any))
	if p == nil || p.Kind != KindSyntax || p.Line != 2 || p.Col != 10 || p.Message != MsgChar {
		t.Errorf("syntax fragment: %+v, want 2:10", p)
	}

	var n any
	if p := a.Decode(&n); p != nil || len(n.([]any)) != 2 {
		t.Errorf("decode = %v %+v", n, p)
	}
}

func TestJSONTrailingCommaPositions(t *testing.T) {
	// hujson Clone drops trailing commas; standardizing one would shift every later offset.
	src := []byte("{\n  \"x\": [1, 2,],\n  \"y\": {\"k\": 1,},\n  \"mcpServers\": {\"a\": {\"args\": \"CANARY\",},},\n}\n")
	for v, in := range variants(src) {
		doc, ps := ParseJSON(in, false)
		if doc == nil || len(ps) > 1 {
			t.Fatalf("%s: %+v", v, ps)
		}
		if len(doc.std) != len(doc.body) {
			t.Fatalf("%s: standardized length %d != %d", v, len(doc.std), len(doc.body))
		}
		args := jsonFind(t, doc.Root(), "mcpServers", "a", "args")
		wl, wc := LineCol(doc.body, bytes.Index(doc.body, []byte(`"CANARY"`)))
		if args.Line != 4 || args.Col != 24 {
			t.Errorf("%s: key at %d:%d", v, args.Line, args.Col)
		}
		if p := args.Decode(&[]string{}); p == nil || p.Line != wl || p.Col != wc || wl != 4 || wc != 32 {
			t.Errorf("%s: %+v, want %d:%d", v, p, wl, wc)
		}
		var s string
		if p := args.Decode(&s); p != nil || s != "CANARY" {
			t.Errorf("%s: %q %+v", v, s, p)
		}
	}
}

func TestJSONBytesIdentity(t *testing.T) {
	src := readCorpus(t, jsonDir)["comments.jsonc"]
	for v, in := range variants(src) {
		doc, _ := ParseJSON(in, false)
		a := jsonFind(t, doc.Root(), "mcpServers", "a")
		body, _, _ := prepare(in)
		start := bytes.Index(body, []byte(`"a": {`)) + len(`"a": `)
		end := bytes.IndexByte(body[start:], '}') + start + 1 // entry a has no nested object
		want := body[start:end]
		if !bytes.Contains(want, []byte("// the runtime")) || !bytes.Equal(a.Bytes(), want) {
			t.Errorf("%s: Bytes = %q, want %q", v, a.Bytes(), want)
		}
		if root := doc.Root(); !bytes.Equal(root.Bytes(), bytes.TrimRight(body[bytes.IndexByte(body, '{'):], "\r\n")) {
			t.Errorf("%s: root bytes differ", v)
		}
	}
}

func FuzzParseJSON(f *testing.F) {
	for _, dir := range []string{jsonDir, spikeDir} {
		for name, b := range readCorpus(f, dir) {
			if strings.HasSuffix(name, ".toml") {
				continue
			}
			for _, vb := range variants(b) {
				f.Add(vb)
			}
		}
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		for _, strict := range []bool{false, true} {
			doc, ps := ParseJSON(src, strict)
			for _, p := range ps {
				if p.Line < 0 || p.Col < 0 || !messages[p.Message] {
					t.Fatalf("bad problem %+v", p)
				}
			}
			if (doc == nil) != (len(jsonKinds(ps, KindSyntax)) > 0) {
				t.Fatalf("doc=%v problems=%+v", doc, ps)
			}
			if doc == nil {
				continue
			}
			_, _, addedNL := prepare(src)
			if !bytes.Equal(unprepare(doc.root.Pack(), doc.BOM, addedNL), src) {
				t.Fatal("pack is not byte-identical")
			}
			if len(doc.std) != len(doc.body) {
				t.Fatalf("standardized length %d != %d", len(doc.std), len(doc.body))
			}
			jsonWalk(t, doc.Root(), 3)
		}
	})
}

// jsonWalk decodes every member down to depth levels and checks each problem message.
func jsonWalk(t *testing.T, m Member, depth int) {
	if p := m.Decode(new(any)); p != nil && !messages[p.Message] {
		t.Fatalf("bad decode problem %+v", p)
	}
	if depth == 0 {
		return
	}
	ms, _, ps := m.Members()
	for _, p := range ps {
		if !messages[p.Message] {
			t.Fatalf("bad member problem %+v", p)
		}
	}
	for _, c := range ms {
		jsonWalk(t, c, depth-1)
	}
}

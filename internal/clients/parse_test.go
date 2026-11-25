package clients

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/alestormoody/mcp-doctor/internal/config"
	"github.com/alestormoody/mcp-doctor/internal/diag"
)

// loadSwapClients replaces the client table for one test.
func loadSwapClients(t *testing.T, cs ...Client) {
	t.Helper()
	saved := all
	all = cs
	t.Cleanup(func() { all = saved })
}

func TestSourcePosition(t *testing.T) {
	if got := (Source{Path: "/p"}).Position(); got != "/p" {
		t.Errorf("Line 0: %q", got)
	}
	if got := (Source{Path: "/p", Line: 3, Col: 5}).Position(); got != "/p:3:5" {
		t.Errorf("Line 3: %q", got)
	}
}

func TestRedactURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://user:CANARY@h.example:8443/mcp/CANARY?k=CANARY#f", "https://h.example:8443/…"},
		{"https://h.example", "https://h.example"},
		{"https://h.example/", "https://h.example"},
		{"http://h/p", "http://h/…"},
		{"http://h?q=CANARY", "http://h/…"},
		{"http://h#CANARY", "http://h/…"},
		{"https://CANARY@h", "https://h/…"},
		{"${URL}", "(URL not shown)"},
		{"not a url", "(URL not shown)"},
		{"mailto:CANARY", "(URL not shown)"},
		{"", "(URL not shown)"},
	}
	for _, c := range cases {
		got := RedactURL(c.in)
		if got != c.want {
			t.Errorf("RedactURL(%q) = %q, want %q", c.in, got, c.want)
		}
		if strings.Contains(got, "CANARY") {
			t.Errorf("RedactURL(%q) leaks: %q", c.in, got)
		}
	}
}

func loadRank(s Source) (int, bool) {
	switch s.Scope {
	case ScopeManaged:
		return 4, true
	case ScopeLocal:
		return 3, true
	case ScopeProject:
		return 2, true
	case ScopeUser:
		return 1, true
	}
	return 0, false
}

func TestMarkShadowed(t *testing.T) {
	srv := func(name string, sc Scope) Server { return Server{Name: name, Source: Source{Scope: sc}} }
	shadowed := func(ss []Server) []bool {
		out := make([]bool, len(ss))
		for i, s := range ss {
			out[i] = s.Shadowed
		}
		return out
	}
	cases := []struct {
		name string
		in   []Server
		want []bool
	}{
		{"user under project", []Server{srv("a", ScopeUser), srv("a", ScopeProject)}, []bool{true, false}},
		{"all four", []Server{srv("a", ScopeUser), srv("a", ScopeManaged), srv("a", ScopeLocal), srv("a", ScopeProject)},
			[]bool{true, false, true, true}},
		{"different names", []Server{srv("a", ScopeUser), srv("b", ScopeProject)}, []bool{false, false}},
		{"ok=false never shadows", []Server{srv("a", ScopeUser), srv("a", "other")}, []bool{false, false}},
		{"ok=false never shadowed", []Server{srv("a", "other"), srv("a", ScopeManaged)}, []bool{false, false}},
		{"equal top ranks", []Server{srv("a", ScopeProject), srv("a", ScopeProject)}, []bool{false, false}},
	}
	for _, c := range cases {
		markShadowed(c.in, loadRank)
		if got := shadowed(c.in); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

type loadCall struct {
	paths []string
	srcs  []string
}

// loadClient is a test-only row whose parse records its input and returns one server per file, named by base name.
func loadClient(id string, cands []Candidate, calls *[]loadCall, rank func(Source) (int, bool)) Client {
	return Client{ID: id, Name: id, homes: func(Env) []string { return nil },
		candidates: func(Env) []Candidate { return cands },
		rank:       rank,
		parse: func(_ Env, files []Loaded) ([]Server, []diag.Finding) {
			var c loadCall
			var out []Server
			for _, f := range files {
				c.paths = append(c.paths, f.Path)
				c.srcs = append(c.srcs, string(f.Src))
				out = append(out, Server{Name: "same", Source: Source{Client: id, Path: f.Path, Scope: f.Scope}})
			}
			*calls = append(*calls, c)
			return out, nil
		}}
}

func TestLoad(t *testing.T) {
	env, _ := fixtureEnv(t)
	p := func(n string) string { return filepath.Join(env.Home, n) }
	contents := map[string]string{"shared.json": `{"s":1}`, "a.json": `{"a":2}`, "b.json": `{"b":3}`,
		"big.json": "{}", "bad.json": "{}"}
	for n, c := range contents {
		writeFileBytes(t, p(n), c)
	}
	big, bad := p("big.json"), p("bad.json")
	reads := map[string]int{}
	readFile := env.ReadFile
	env.ReadFile = func(name string) ([]byte, error) {
		reads[filepath.Clean(name)]++
		switch filepath.Clean(name) {
		case big:
			return make([]byte, MaxFileSize+1), nil
		case bad:
			return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrPermission}
		}
		return readFile(name)
	}
	user := func(n string) Candidate { return Candidate{Scope: ScopeUser, Path: p(n), Format: FormatJSON} }
	proj := func(n string) Candidate { return Candidate{Scope: ScopeProject, Path: p(n), Format: FormatJSON} }
	var callsA, callsB, callsC []loadCall
	loadSwapClients(t,
		loadClient("a", []Candidate{user("a.json"), proj("shared.json"), user("missing.json"), user("big.json")}, &callsA, loadRank),
		loadClient("b", []Candidate{user("shared.json"), user("b.json"), user("bad.json")}, &callsB, nil),
		loadClient("c", []Candidate{user("missing.json")}, &callsC, loadRank),
	)

	_, servers, findings := Load(env)

	if len(callsA) != 1 || len(callsB) != 1 {
		t.Fatalf("parse calls: a=%d b=%d, want 1 each", len(callsA), len(callsB))
	}
	if len(callsC) != 0 {
		t.Errorf("client without readable files was parsed: %+v", callsC)
	}
	wantA := []string{p("a.json"), p("shared.json")}
	if strings.Join(callsA[0].paths, "|") != strings.Join(wantA, "|") ||
		strings.Join(callsA[0].srcs, "|") != contents["a.json"]+"|"+contents["shared.json"] {
		t.Errorf("client a got %+v", callsA[0])
	}
	if strings.Join(callsB[0].paths, "|") != p("shared.json")+"|"+p("b.json") {
		t.Errorf("client b got %+v", callsB[0])
	}
	if reads[p("shared.json")] != 2 || reads[p("a.json")] != 1 || reads[p("b.json")] != 1 || reads[big] != 1 || reads[bad] != 1 {
		t.Errorf("reads = %v", reads)
	}
	if reads[p("missing.json")] != 0 {
		t.Errorf("missing file read %d times", reads[p("missing.json")])
	}

	var codes []string
	for _, f := range findings {
		codes = append(codes, f.Code)
		if err := f.Validate(); err != nil {
			t.Error(err)
		}
		switch f.Code {
		case "MCP_DISC_001":
			if f.Source != bad || f.Category != "discovery" || f.Client != "b" {
				t.Errorf("DISC_001 = %+v", f)
			}
		case "MCP_PARSE_007":
			if f.Source != big || f.Category != "parse" || f.Client != "a" || f.Resolution == "" {
				t.Errorf("PARSE_007 = %+v", f)
			}
		}
	}
	if strings.Join(codes, ",") != "MCP_PARSE_007,MCP_DISC_001" {
		t.Errorf("finding codes = %v", codes)
	}

	var order []string
	for _, s := range servers {
		order = append(order, s.Source.Client+":"+filepath.Base(s.Source.Path)+":"+strconv.FormatBool(s.Shadowed))
	}
	// a: user a.json is shadowed by project shared.json; b has no rank; across clients nothing is shadowed.
	want := "a:a.json:true|a:shared.json:false|b:shared.json:false|b:b.json:false"
	if strings.Join(order, "|") != want {
		t.Errorf("servers = %v, want %s", order, want)
	}
}

func TestLoadReadAlwaysFails(t *testing.T) {
	table := append([]Client(nil), all...)
	env, _ := fixtureEnv(t)
	writeFileBytes(t, filepath.Join(env.Home, "x.json"), "{}")
	env.ReadFile = func(string) ([]byte, error) { return nil, errors.New("boom") }
	var calls []loadCall
	loadSwapClients(t, loadClient("a", []Candidate{userFile(filepath.Join(env.Home, "x.json"))}, &calls, nil))
	_, servers, findings := Load(env)
	if len(servers) != 0 || len(calls) != 0 || len(findings) != 1 || findings[0].Code != "MCP_DISC_001" {
		t.Errorf("servers=%v calls=%v findings=%+v", servers, calls, findings)
	}

	// The real table with every file unreadable never panics.
	loadSwapClients(t, table...)
	writeFileBytes(t, filepath.Join(env.Cwd, ".mcp.json"), "{}")
	if _, _, findings := Load(env); len(findings) == 0 {
		t.Error("no findings for an unreadable .mcp.json")
	}
}

func TestTableParse(t *testing.T) {
	ranked := map[string]bool{"claude-code": true, "codex": true}
	for _, c := range all {
		if c.parse == nil {
			t.Errorf("%s: parse is nil", c.ID)
		}
		if (c.rank != nil) != ranked[c.ID] {
			t.Errorf("%s: rank set = %v, want %v", c.ID, c.rank != nil, ranked[c.ID])
		}
	}
}

func writeFileBytes(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil { // #nosec G703 -- test fixture path under t.TempDir
		t.Fatal(err)
	}
}

func TestParseHelpers(t *testing.T) {
	if f := jsonFile("/p", FormatJSONC, ScopeProject); f.Path != "/p" || f.Format != FormatJSONC || f.Scope != ScopeProject || f.Status != StatusFound {
		t.Errorf("jsonFile = %+v", f)
	}
	if got := string(stripBOM([]byte("\xEF\xBB\xBF{}"))); got != "{}" {
		t.Errorf("stripBOM = %q", got)
	}
	if got := string(stripBOM([]byte("{}"))); got != "{}" {
		t.Errorf("stripBOM without BOM = %q", got)
	}
	if m := readFixtures(t, "testdata/core"); len(m["mcp.json"]) == 0 {
		t.Errorf("readFixtures = %v", m)
	}
	if got := secrets([]diag.Secret{diag.NewSecret("a"), diag.NewSecret("b")}); strings.Join(got, ",") != "a,b" {
		t.Errorf("secrets = %v", got)
	}
	if secretMap(nil) != nil || secretMap(map[string]diag.Secret{"k": diag.NewSecret("v")})["k"] != "v" {
		t.Error("secretMap")
	}
}

// parseOpen parses src as a file at /p and fails the test on a syntax error.
func parseOpen(t *testing.T, format Format, src string) (File, *config.JSONDoc) {
	t.Helper()
	f := jsonFile("/p", format, ScopeUser)
	doc, fds := openJSON("test", f, []byte(src))
	if doc == nil {
		t.Fatalf("openJSON: nil doc, findings %+v", fds)
	}
	return f, doc
}

// parseTop returns the top-level member name of doc.
func parseTop(t *testing.T, f File, doc *config.JSONDoc, name string) config.Member {
	t.Helper()
	fields, ok, _ := objectFields("test", f, doc.Root())
	if !ok {
		t.Fatal("root is not an object")
	}
	return fields[name]
}

func parseCodes(fds []diag.Finding) string {
	var cs []string
	for _, f := range fds {
		cs = append(cs, f.Code)
	}
	return strings.Join(cs, ",")
}

func parseCheckFindings(t *testing.T, fds []diag.Finding) {
	t.Helper()
	for _, f := range fds {
		if err := f.Validate(); err != nil {
			t.Error(err)
		}
		if strings.Contains(f.Message, "CANARY") || strings.Contains(f.Source, "CANARY") {
			t.Errorf("finding leaks input: %+v", f)
		}
		if f.Resolution == "" {
			t.Errorf("no resolution: %+v", f)
		}
	}
}

func TestOpenJSON(t *testing.T) {
	f := jsonFile("/p", FormatJSON, ScopeUser)
	doc, fds := openJSON("test", f, []byte("{\n  // c\n  \"a\": \"CANARY-1\"\n}\n"))
	if doc == nil || parseCodes(fds) != "MCP_PARSE_006" || fds[0].Source != "/p:2:3" {
		t.Errorf("json with comment: doc=%v %+v", doc != nil, fds)
	}
	parseCheckFindings(t, fds)

	fc := jsonFile("/p", FormatJSONC, ScopeUser)
	if doc, fds := openJSON("test", fc, []byte("{\n  // c\n  \"a\": 1,\n}\n")); doc == nil || len(fds) != 0 {
		t.Errorf("jsonc: doc=%v %+v", doc != nil, fds)
	}

	doc, fds = openJSON("test", f, []byte("{\n  \"a\": \"CANARY-2\",\n  \"b\": tru"))
	if doc != nil || parseCodes(fds) != "MCP_PARSE_001" || !strings.HasPrefix(fds[0].Source, "/p:3:") ||
		!strings.HasPrefix(fds[0].Message, "Syntax error: ") {
		t.Errorf("truncated: doc=%v %+v", doc != nil, fds)
	}
	parseCheckFindings(t, fds)

	doc, fds = openJSON("test", f, append([]byte("\xEF\xBB\xBF"), `{"a": "CANARY-3"}`...))
	if doc == nil || parseCodes(fds) != "MCP_PARSE_010" || fds[0].Source != "/p:1:1" {
		t.Errorf("BOM: doc=%v %+v", doc != nil, fds)
	}
	parseCheckFindings(t, fds)

	fd := problemFinding("test", f, config.Problem{Kind: config.KindType, Line: 2, Col: 4, Message: config.MsgType})
	if fd.Code != "MCP_PARSE_002" || fd.Source != "/p:2:4" || fd.Message != "Invalid structure: "+config.MsgType {
		t.Errorf("type problem: %+v", fd)
	}
}

func TestJSONEntries(t *testing.T) {
	src := "{\n  \"servers\": {\n    \"a\": {\"command\": \"x\"},\n    \"b\": 5,\n    \"c\": {\"command\": \"y\", \"command\": \"z\"},\n    \"a\": {\"command\": \"w\"}\n  }\n}\n"
	f, doc := parseOpen(t, FormatJSONC, src)
	es, fds := jsonEntries("test", f, ScopeLocal, "lbl", parseTop(t, f, doc, "servers"))
	var names []string
	for _, e := range es {
		names = append(names, e.Name+"@"+e.Src.Position())
		if e.Src.Scope != ScopeLocal || e.Src.Label != "lbl" || e.Src.Client != "test" || e.Src.Format != FormatJSONC {
			t.Errorf("Src = %+v", e.Src)
		}
	}
	if got := strings.Join(names, " "); got != "c@/p:5:5 a@/p:6:5" {
		t.Errorf("entries = %s", got)
	}
	if parseCodes(fds) != "MCP_PARSE_004,MCP_PARSE_002,MCP_PARSE_004" {
		t.Errorf("findings = %+v", fds)
	}
	for _, fd := range fds {
		if fd.Code == "MCP_PARSE_002" && (fd.Source != "/p:4:5" || fd.Server != "b") {
			t.Errorf("002 = %+v", fd)
		}
	}
	parseCheckFindings(t, fds)
	if s, _ := es[0].str("command"); s != "z" {
		t.Errorf("duplicate field: last wins, got %q", s)
	}
	if s, _ := es[1].str("command"); s != "w" {
		t.Errorf("duplicate server: last wins, got %q", s)
	}

	f, doc = parseOpen(t, FormatJSON, "{\n  \"mcpServers\": [\"CANARY\"]\n}")
	es, fds = jsonEntries("test", f, ScopeUser, "", parseTop(t, f, doc, "mcpServers"))
	if len(es) != 0 || parseCodes(fds) != "MCP_PARSE_002" || fds[0].Source != "/p:2:3" ||
		!strings.Contains(fds[0].Message, `"mcpServers" is not an object`) {
		t.Errorf("non-object servers: %v %+v", es, fds)
	}
	if es, fds := jsonEntries("test", f, ScopeUser, "", config.Member{}); es != nil || fds != nil {
		t.Errorf("absent: %v %v", es, fds)
	}
}

func TestRawPreservesUnknown(t *testing.T) {
	entry := "{\n      // keep me\n      \"comand\": \"CANARY-x\",\n      \"nested\": {\"deep\": [1, 2, {\"k\": \"CANARY-y\"}]},\n    }"
	src := "{\n  \"mcpServers\": {\n    \"x\": " + entry + "\n  }\n}\n"
	f, doc := parseOpen(t, FormatJSONC, src)
	es, _ := jsonEntries("test", f, ScopeUser, "", parseTop(t, f, doc, "mcpServers"))
	if len(es) != 1 {
		t.Fatalf("entries = %v", es)
	}
	i := strings.Index(src, entry)
	want := []byte(src[i : i+len(entry)])
	if !bytes.Equal(es[0].raw, want) || !bytes.Contains(es[0].raw, []byte(`"comand": "CANARY-x"`)) {
		t.Errorf("raw = %q", es[0].raw)
	}
	if got := es[0].unknown("command"); strings.Join(got, ",") != "comand,nested" {
		t.Errorf("unknown = %v", got)
	}
	s := Server{Name: "x", raw: &es[0].raw}
	for _, verb := range []string{"%s", "%v", "%+v", "%#v"} {
		if out := fmt.Sprintf(verb, s); strings.Contains(out, "CANARY") {
			t.Errorf("%s prints raw: %s", verb, out)
		}
	}
	if !bytes.Equal(*s.raw, want) {
		t.Error("raw changed after printing")
	}
	fd := unknownFinding("test", Server{Name: "x", Source: es[0].Src}, es[0].unknown("command"), "not supported in this file")
	if fd.Code != "MCP_PARSE_005" || fd.Message != "Unknown field(s): comand, nested (not supported in this file)" ||
		fd.Source != "/p:3:5" || fd.Server != "x" || fd.Validate() != nil {
		t.Errorf("unknownFinding = %+v", fd)
	}
	if fd := unknownFinding("test", Server{Name: "x"}, []string{"a"}, ""); fd.Message != "Unknown field(s): a" {
		t.Errorf("no note: %q", fd.Message)
	}
}

func TestFieldTypes(t *testing.T) {
	src := "{\n  \"mcpServers\": {\n    \"a\": {\"command\": \"x\"},\n    \"b\": {\"command\": 42, \"args\": \"CANARY-s\", \"env\": {\"K\": 1}, \"ok\": [\"p\"], \"m\": {\"K\": \"v\"}}\n  }\n}\n"
	f, doc := parseOpen(t, FormatJSON, src)
	es, _ := jsonEntries("test", f, ScopeUser, "", parseTop(t, f, doc, "mcpServers"))
	b := es[1]
	_, fd := b.str("command")
	if fd == nil || fd.Code != "MCP_PARSE_002" || fd.Source != "/p:4:22" || fd.Server != "b" {
		t.Errorf("str on number: %+v", fd)
	}
	if _, fd := b.strs("args"); fd == nil || fd.Source != "/p:4:34" || strings.Contains(fd.Message, "CANARY") {
		t.Errorf("strs on string: %+v", fd)
	}
	if _, fd := b.strMap("env"); fd == nil || fd.Code != "MCP_PARSE_002" || !strings.HasPrefix(fd.Source, "/p:4:") {
		t.Errorf("strMap non-string: %+v", fd)
	}
	if v, fd := b.strs("ok"); fd != nil || strings.Join(v, ",") != "p" {
		t.Errorf("strs ok: %v %+v", v, fd)
	}
	if v, fd := b.strMap("m"); fd != nil || v["K"] != "v" {
		t.Errorf("strMap ok: %v %+v", v, fd)
	}
	a := es[0]
	if s, fd := a.str("url"); s != "" || fd != nil {
		t.Errorf("absent str: %q %v", s, fd)
	}
	if v, fd := a.strs("args"); v != nil || fd != nil {
		t.Errorf("absent strs: %v %v", v, fd)
	}
	if v, fd := a.strMap("env"); v != nil || fd != nil {
		t.Errorf("absent strMap: %v %v", v, fd)
	}
	parseCheckFindings(t, []diag.Finding{*fd})
}

func TestResolveTransport(t *testing.T) {
	aliases := map[string]string{"streamable-http": "http", "local": "stdio"}
	allowed := []string{"stdio", "http", "sse", "ws", "sdk"}
	cases := []struct {
		stated              string
		cmd, url, needsType bool
		t                   string
		inferred            bool
		code                string
	}{
		{"", true, false, false, "stdio", true, ""},
		{"", true, true, true, "stdio", true, ""},
		{"", false, true, false, "http", true, ""},
		{"", false, true, true, "", false, "MCP_PARSE_008"},
		{"", false, false, false, "", false, "MCP_PARSE_003"},
		{"stdio", true, false, false, "stdio", false, ""},
		{"local", true, false, false, "stdio", false, ""},
		{"stdio", false, true, false, "stdio", false, "MCP_PARSE_008"},
		{"stdio", false, false, false, "stdio", false, "MCP_PARSE_003"},
		{"streamable-http", false, true, true, "http", false, ""},
		{"http", false, false, false, "http", false, "MCP_PARSE_008"},
		{"sse", false, true, false, "sse", false, ""},
		{"ws", true, false, false, "ws", false, "MCP_PARSE_008"},
		{"sdk", false, false, false, "sdk", false, ""},
		{"CANARY-type", true, true, false, "", false, "MCP_PARSE_008"},
	}
	for _, c := range cases {
		tr, inf, code, msg := resolveTransport(c.stated, c.cmd, c.url, c.needsType, aliases, allowed...)
		if tr != c.t || inf != c.inferred || code != c.code || (code == "") != (msg == "") {
			t.Errorf("%+v: got %q %v %q %q", c, tr, inf, code, msg)
		}
		if strings.Contains(msg, "CANARY") {
			t.Errorf("message echoes the type: %q", msg)
		}
	}
	if _, _, _, msg := resolveTransport("x", false, false, false, nil, "stdio", "http"); msg != "transport type is not one of: stdio, http" {
		t.Errorf("unknown type msg = %q", msg)
	}
	if _, _, _, msg := resolveTransport("sse", false, false, false, nil, "sse"); msg != "sse server has no url" {
		t.Errorf("no url msg = %q", msg)
	}
	if _, _, _, msg := resolveTransport("stdio", false, true, false, nil, "stdio"); msg != "stdio server has a url but no command" {
		t.Errorf("stdio url msg = %q", msg)
	}
}

func TestSamePath(t *testing.T) {
	cases := []struct {
		goos, a, b string
		want       bool
	}{
		{"windows", `C:\Users\A\proj`, "c:/users/a/proj/", true},
		{"windows", `C:\Users\A\proj`, `C:\Users\A\work`, false},
		{"linux", "/a/b/", "/a/b", true},
		{"linux", "/a/B", "/a/b", false},
		{"darwin", "/a/./b", "/a/b", true},
		{"linux", "", "/a", false},
		{"windows", "", `C:\`, false},
		{"windows", `C:\repo\.\sub`, `C:\repo\sub`, true},
		{"windows", `C:\repo\x\..\sub`, `c:/REPO/sub`, true},
		{"windows", `C:\repo\sub\..`, `C:\repo\sub`, false},
		{"linux", "/a/b/../c", "/a/c", true},
		{"darwin", "/a/b/../c", "/a/b/c", false},
		{"linux", `/a\b`, "/a/b", false}, // a backslash is a name character outside Windows
	}
	for _, c := range cases {
		if got := samePath(c.goos, c.a, c.b); got != c.want {
			t.Errorf("samePath(%s, %q, %q) = %v", c.goos, c.a, c.b, got)
		}
	}
}

func TestVarRefs(t *testing.T) {
	got := dollarRefs("args[0]", "a ${TOKEN} b ${X:-CANARY-default} $BARE")
	want := []VarRef{{"args[0]", "env", "TOKEN", false}, {"args[0]", "env", "X", true}}
	if !slices.Equal(got, want) {
		t.Errorf("dollarRefs = %+v", got)
	}
	if strings.Contains(fmt.Sprintf("%+v", got), "CANARY") {
		t.Error("default value stored")
	}
	got = colonRefs("env.K", "${env:A} ${input:key} ${workspaceFolder} ${userHome} ${pathSeparator} ${config:x}")
	var kinds, names []string
	for _, r := range got {
		kinds, names = append(kinds, r.Kind), append(names, r.Name)
		if r.Field != "env.K" {
			t.Errorf("field = %q", r.Field)
		}
	}
	if strings.Join(kinds, ",") != "env,input,workspace,userHome,other,other" ||
		strings.Join(names, ",") != "A,key,workspaceFolder,userHome,pathSeparator,config:x" {
		t.Errorf("colonRefs kinds=%v names=%v", kinds, names)
	}
	for _, s := range []string{"${", "${}", "$", "plain"} {
		if r := append(dollarRefs("f", s), colonRefs("f", s)...); len(r) != 0 {
			t.Errorf("%q: %+v", s, r)
		}
	}
	if r := colonRefs("f", "${workspaceFolderBasename}${/}"); len(r) != 2 || r[0].Kind != "workspace" || r[1].Kind != "other" {
		t.Errorf("basename and slash: %+v", r)
	}
}

func TestJSONNullRejected(t *testing.T) {
	cases := map[string]string{
		"command":       `{"command":null}`,
		"args element":  `{"command":"run","args":[null]}`,
		"args":          `{"command":"run","args":null}`,
		"args number":   `{"command":"run","args":[1]}`,
		"env value":     `{"command":"run","env":{"A":null}}`,
		"headers value": `{"type":"http","url":"https://h","headers":{"H":null}}`,
		"headers":       `{"type":"http","url":"https://h","headers":null}`,
	}
	for name, entry := range cases {
		src := []byte(`{"mcpServers":{"s":` + entry + `,"ok":{"command":"y"}}}`)
		ss, _, fds := claudeCodeFile(fakeEnv("linux", nil), jsonFile("/p", FormatJSON, ScopeProject), src)
		if parseCodes(fds) != "MCP_PARSE_002" {
			t.Errorf("%s: codes %s: %+v", name, parseCodes(fds), fds)
		}
		if len(ss) != 1 || ss[0].Name != "ok" {
			t.Errorf("%s: kept %+v", name, ss)
		}
	}
	// the shared decoder covers every JSON client
	ss, fds := cursorFile(jsonFile("/c", FormatJSON, ScopeUser), []byte(`{"mcpServers":{"s":{"command":"run","env":{"A":null}}}}`))
	if len(ss) != 0 || parseCodes(fds) != "MCP_PARSE_002" {
		t.Errorf("cursor: %d servers, %+v", len(ss), fds)
	}
}

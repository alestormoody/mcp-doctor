package clients

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

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

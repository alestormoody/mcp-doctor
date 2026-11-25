package clients

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/alestormoody/mcp-doctor/internal/diag"
)

const codexParseDir = "testdata/codex/parse/"

// codexParseFixture parses one fixture as a user file at /c/<name>.
func codexParseFixture(t *testing.T, name string) ([]Server, []diag.Finding, []byte) {
	t.Helper()
	src, err := os.ReadFile(codexParseDir + name) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatal(err)
	}
	ss, _, fds := codexFile(Env{}, jsonFile("/c/"+name, FormatTOML, ScopeUser), src)
	parseCheckFindings(t, fds)
	return ss, fds, src
}

func TestCodexParseShapes(t *testing.T) {
	cases := []struct{ file, from, to string }{
		{"shapes-table.toml", "[mcp_servers.docs]", "[other]"},
		{"shapes-inline.toml", "docs = {", "\n\n[other]"},
		{"shapes-dotted.toml", "mcp_servers.docs.command", "\n\n[other]"},
	}
	var first Server
	for i, c := range cases {
		ss, fds, src := codexParseFixture(t, c.file)
		if len(fds) != 0 || len(ss) != 1 {
			t.Fatalf("%s: servers %d findings %+v", c.file, len(ss), fds)
		}
		s := ss[0]
		from := bytes.Index(src, []byte(c.from))
		to := bytes.Index(src, []byte(c.to))
		if s.raw == nil || !bytes.Equal(*s.raw, src[from:to]) {
			t.Errorf("%s: raw mismatch", c.file)
		}
		if s.Source.Line == 0 || s.Source.Col == 0 {
			t.Errorf("%s: no position", c.file)
		}
		if s.Name != "docs" || s.Command != "npx" || s.Transport != "stdio" || !s.TransportInferred ||
			strings.Join(secrets(s.Args), " ") != "--token CANARY-a" || secretMap(s.Env)["K"] != "CANARY-e" {
			t.Errorf("%s: %+v", c.file, s)
		}
		s.raw, s.Source.Line, s.Source.Col, s.Source.Path = nil, 0, 0, ""
		if i == 0 {
			first = s
		} else if !reflect.DeepEqual(s, first) {
			t.Errorf("%s differs from table shape:\n%+v\n%+v", c.file, s, first)
		}
	}
}

func TestCodexParseRemote(t *testing.T) {
	ss, fds, _ := codexParseFixture(t, "remote.toml")
	if len(fds) != 0 || len(ss) != 1 {
		t.Fatalf("servers %d findings %+v", len(ss), fds)
	}
	s := ss[0]
	if s.Transport != "http" || !s.TransportInferred || !s.Disabled ||
		s.URL.Reveal() != "https://h.example/mcp/CANARY-p" || secretMap(s.Headers)["Authorization"] != "Bearer CANARY-h" ||
		secretMap(s.Headers)["X-Key"] != "${MY_KEY}" || len(s.Headers) != 2 || len(s.Unknown) != 0 {
		t.Errorf("%+v", s)
	}
	want := []VarRef{
		{Field: "env_vars[0]", Kind: "env", Name: "A"},
		{Field: "env_vars[1]", Kind: "env", Name: "B"},
		{Field: "bearer_token_env_var", Kind: "env", Name: "TOKEN_VAR"},
		{Field: "env_http_headers.X-Key", Kind: "env", Name: "MY_KEY"},
	}
	if !reflect.DeepEqual(s.Refs, want) {
		t.Errorf("refs = %+v", s.Refs)
	}
	for _, f := range []string{"%v", "%+v", "%#v", "%s"} {
		if out := fmt.Sprintf(f, s); strings.Contains(out, "CANARY") {
			t.Errorf("%s leaks: %s", f, out)
		}
	}
}

func TestCodexParseIncomplete(t *testing.T) {
	ss, fds, _ := codexParseFixture(t, "incomplete.toml")
	if parseCodes(fds) != "MCP_PARSE_003,MCP_PARSE_002,MCP_PARSE_005" {
		t.Fatalf("findings %+v", fds)
	}
	if fds[1].Source != "/c/incomplete.toml:6:1" || fds[1].Server != "badargs" {
		t.Errorf("002 at %q server %q", fds[1].Source, fds[1].Server)
	}
	if !strings.Contains(fds[2].Message, "comand") {
		t.Errorf("005 = %q", fds[2].Message)
	}
	if len(ss) != 1 || ss[0].Name != "typo" || !reflect.DeepEqual(ss[0].Unknown, []string{"comand"}) {
		t.Errorf("servers %+v", ss)
	}
}

func TestCodexParseCorrupted(t *testing.T) {
	for _, name := range []string{"corrupted.toml", "dup.toml"} {
		ss, fds, _ := codexParseFixture(t, name)
		if len(ss) != 0 || parseCodes(fds) != "MCP_PARSE_001" {
			t.Errorf("%s: servers %d findings %+v", name, len(ss), fds)
			continue
		}
		if !strings.HasPrefix(fds[0].Source, "/c/"+name+":") || strings.HasSuffix(fds[0].Source, ":0:0") {
			t.Errorf("%s: source %q", name, fds[0].Source)
		}
	}
}

func TestCodexParseStructure(t *testing.T) {
	cases := []struct{ src, codes string }{
		{"mcp_servers = 1\n", "MCP_PARSE_002"},
		{"[mcp_servers]\na = 1\n", "MCP_PARSE_002"},
		{"x = 1\n", ""},
		{"\xEF\xBB\xBF[mcp_servers.a]\ncommand = \"n\"\n", "MCP_PARSE_010"},
		{"[mcp_servers.a]\ncommand = 1\n", "MCP_PARSE_002"},
		{"[mcp_servers.a]\ncommand = \"n\"\nargs = [1]\n", "MCP_PARSE_002"},
		{"[mcp_servers.a]\ncommand = \"n\"\nenv = 1\n", "MCP_PARSE_002"},
		{"[mcp_servers.a]\ncommand = \"n\"\nenv = { K = 1 }\n", "MCP_PARSE_002"},
		{"[mcp_servers.a]\ncommand = \"n\"\nenv_vars = 1\n", "MCP_PARSE_002"},
		{"[mcp_servers.a]\ncommand = \"n\"\nenv_vars = [{ source = \"x\" }]\n", "MCP_PARSE_002"},
		{"[mcp_servers.a]\ncommand = \"n\"\nenabled = \"no\"\n", "MCP_PARSE_002"},
		{"[mcp_servers.a]\ncommand = \"n\"\nurl = \"https://h\"\nrequired = true\n", ""},
	}
	for _, c := range cases {
		_, _, fds := codexFile(Env{}, jsonFile("/c", FormatTOML, ScopeUser), []byte(c.src))
		parseCheckFindings(t, fds)
		if parseCodes(fds) != c.codes {
			t.Errorf("%q: %+v", c.src, fds)
		}
	}
}

// codexTrustTOML is a user config trusting (or not) each dir.
func codexTrustTOML(levels map[string]string) string {
	var b strings.Builder
	for dir, level := range levels {
		fmt.Fprintf(&b, "[projects.%s]\ntrust_level = %q\n", strconv.Quote(dir), level)
	}
	return b.String()
}

// codexLoad runs Load with only the Codex client and returns its servers.
func codexLoad(t *testing.T, env Env) []Server {
	t.Helper()
	loadSwapClients(t, lookupClient(t, "codex"))
	_, ss, fds := Load(env)
	parseCheckFindings(t, fds)
	return ss
}

func TestCodexTrust(t *testing.T) {
	const untrusted = "not trusted: Codex ignores this file"
	tmpl, err := os.ReadFile(codexParseDir + "user-trust.toml")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		user  func(cwd string) string // "" -> no user config
		label string
	}{
		{"cwd trusted", func(cwd string) string {
			return strings.Replace(string(tmpl), `"<DIR>"`, strconv.Quote(cwd), 1)
		}, "trusted project"},
		{"parent trusted", func(cwd string) string {
			return strings.Replace(string(tmpl), `"<DIR>"`, strconv.Quote(filepath.Dir(cwd)), 1)
		}, "trusted project"},
		{"nearer untrusted wins", func(cwd string) string {
			return codexTrustTOML(map[string]string{filepath.Dir(cwd): "trusted", cwd: "untrusted"})
		}, untrusted},
		{"untrusted", func(cwd string) string { return codexTrustTOML(map[string]string{cwd: "untrusted"}) }, untrusted},
		{"no projects", func(string) string { return "model = \"x\"\n" }, untrusted},
		{"corrupted user", func(string) string { return "[projects\n" }, untrusted},
		{"no user config", func(string) string { return "" }, untrusted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env, _ := fixtureEnv(t)
			place(t, codexParseDir+"shapes-table.toml", filepath.Join(env.Cwd, ".codex", "config.toml"))
			if u := c.user(env.Cwd); u != "" {
				writeFileBytes(t, filepath.Join(env.Home, ".codex", "config.toml"), u)
			}
			ss := codexLoad(t, env)
			if len(ss) != 1 || ss[0].Source.Scope != ScopeProject || ss[0].Source.Label != c.label {
				t.Errorf("servers %+v", ss)
			}
		})
	}
}

func TestCodexReadOnce(t *testing.T) {
	for _, withUser := range []bool{true, false} {
		env, _ := fixtureEnv(t)
		parent := filepath.Dir(env.Cwd)
		writeFileBytes(t, filepath.Join(parent, ".git"), "")
		codexDir := filepath.Join(env.Home, ".codex")
		user := filepath.Join(codexDir, "config.toml")
		paths := []string{filepath.Join(codexDir, "work.config.toml"),
			filepath.Join(parent, ".codex", "config.toml"), filepath.Join(env.Cwd, ".codex", "config.toml")}
		for _, p := range paths {
			place(t, codexParseDir+"shapes-table.toml", p)
		}
		if withUser {
			writeFileBytes(t, user, codexTrustTOML(map[string]string{parent: "trusted"}))
		}
		reads := map[string]int{}
		env.ReadFile = func(p string) ([]byte, error) {
			reads[p]++
			return os.ReadFile(p) // #nosec G304 -- test fixture paths under t.TempDir
		}
		ss := codexLoad(t, env)
		for _, p := range paths {
			if reads[p] != 1 {
				t.Errorf("user=%v %s read %d times", withUser, p, reads[p])
			}
		}
		if want := map[bool]int{true: 1, false: 0}[withUser]; reads[user] != want {
			t.Errorf("user=%v user config read %d times", withUser, reads[user])
		}
		for _, s := range ss {
			if s.Source.Scope == ScopeProject && (s.Source.Label == "trusted project") != withUser {
				t.Errorf("user=%v %s label %q", withUser, s.Source.Path, s.Source.Label)
			}
		}
	}
}

func TestCodexPrecedence(t *testing.T) {
	setup := func(t *testing.T, levels func(cwd string) map[string]string) (Env, map[string]Server) {
		t.Helper()
		env, _ := fixtureEnv(t)
		parent := filepath.Dir(env.Cwd)
		writeFileBytes(t, filepath.Join(parent, ".git"), "")
		codexDir := filepath.Join(env.Home, ".codex")
		for _, p := range []string{filepath.Join(codexDir, "work.config.toml"),
			filepath.Join(parent, ".codex", "config.toml"), filepath.Join(env.Cwd, ".codex", "config.toml")} {
			place(t, codexParseDir+"shapes-table.toml", p)
		}
		user := codexTrustTOML(levels(env.Cwd)) + "\n" + string(codexMust(os.ReadFile(codexParseDir+"shapes-table.toml")))
		writeFileBytes(t, filepath.Join(codexDir, "config.toml"), user)
		byWhere := map[string]Server{}
		for _, s := range codexLoad(t, env) {
			switch {
			case strings.HasPrefix(s.Source.Label, "profile "):
				byWhere["profile"] = s
			case s.Source.Scope == ScopeUser:
				byWhere["user"] = s
			case filepath.Dir(filepath.Dir(s.Source.Path)) == env.Cwd:
				byWhere["cwd"] = s
			default:
				byWhere["parent"] = s
			}
		}
		if len(byWhere) != 4 {
			t.Fatalf("servers %+v", byWhere)
		}
		return env, byWhere
	}
	shadowed := func(m map[string]Server) map[string]bool {
		out := map[string]bool{}
		for k, s := range m {
			out[k] = s.Shadowed
		}
		return out
	}

	t.Run("closest project wins", func(t *testing.T) {
		_, m := setup(t, func(cwd string) map[string]string { return map[string]string{filepath.Dir(cwd): "trusted"} })
		want := map[string]bool{"cwd": false, "parent": true, "user": true, "profile": false}
		if got := shadowed(m); !reflect.DeepEqual(got, want) {
			t.Errorf("shadowed = %v", got)
		}
		if m["profile"].Source.Label != "profile work (only with --profile)" {
			t.Errorf("profile label %q", m["profile"].Source.Label)
		}
	})
	t.Run("untrusted cwd shadows nothing", func(t *testing.T) {
		_, m := setup(t, func(cwd string) map[string]string {
			return map[string]string{filepath.Dir(cwd): "trusted", cwd: "untrusted"}
		})
		want := map[string]bool{"cwd": false, "parent": false, "user": true, "profile": false}
		if got := shadowed(m); !reflect.DeepEqual(got, want) {
			t.Errorf("shadowed = %v", got)
		}
	})
	t.Run("system below user, profile shadows nothing", func(t *testing.T) {
		src := codexMust(os.ReadFile(codexParseDir + "shapes-table.toml"))
		env := fakeEnv("linux", nil)
		sys := File{Candidate: Candidate{Scope: ScopeManaged, Path: "/etc/codex/config.toml", Format: FormatTOML}, Status: StatusFound}
		user := jsonFile("/h/.codex/config.toml", FormatTOML, ScopeUser)
		prof := jsonFile("/h/.codex/work.config.toml", FormatTOML, ScopeUser)
		prof.Label = "profile work"
		ss, fds := parseCodex(env, []Loaded{{File: sys, Src: src}, {File: user, Src: src}, {File: prof, Src: src}})
		if len(fds) != 0 || len(ss) != 3 {
			t.Fatalf("servers %d findings %+v", len(ss), fds)
		}
		markShadowed(ss, codexRank)
		if !ss[0].Shadowed || ss[1].Shadowed || ss[2].Shadowed {
			t.Errorf("shadowed system/user/profile = %v %v %v", ss[0].Shadowed, ss[1].Shadowed, ss[2].Shadowed)
		}
	})
}

func TestCodexRank(t *testing.T) {
	cases := []struct {
		src  Source
		rank int
		ok   bool
	}{
		{Source{Scope: ScopeManaged}, 1, true},
		{Source{Scope: ScopeUser, Label: "CODEX_HOME"}, 2, true},
		{Source{Scope: ScopeUser, Label: "profile w (only with --profile)"}, 0, false},
		{Source{Scope: ScopeProject, Path: "/a/b/.codex/config.toml", Label: "trusted project"}, 102, true},
		{Source{Scope: ScopeProject, Path: "/a/b/.codex/config.toml", Label: "not trusted: Codex ignores this file"}, 0, false},
		{Source{Scope: ScopeLocal}, 0, false},
	}
	for _, c := range cases {
		if r, ok := codexRank(c.src); r != c.rank || ok != c.ok {
			t.Errorf("%+v: %d %v", c.src, r, ok)
		}
	}
}

func codexMust[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func FuzzParseCodex(f *testing.F) {
	for _, dir := range []string{"testdata/codex/parse", "testdata/codex"} {
		for _, b := range readFixtures(f, dir) {
			f.Add(b)
		}
	}
	env := fakeEnv("linux", nil)
	user := jsonFile("/h/.codex/config.toml", FormatTOML, ScopeUser)
	proj := jsonFile("/w/proj/.codex/config.toml", FormatTOML, ScopeProject)
	f.Fuzz(func(t *testing.T, src []byte) {
		body := stripBOM(src)
		check := func(ss []Server, fds []diag.Finding) {
			for _, fd := range fds {
				if err := fd.Validate(); err != nil || fd.Client != "codex" {
					t.Errorf("finding %+v: %v", fd, err)
				}
			}
			for _, s := range ss {
				if s.raw != nil && !bytes.Contains(body, *s.raw) {
					t.Errorf("raw of %q not in input", s.Name)
				}
			}
		}
		for _, file := range []File{user, proj} {
			ss, _, fds := codexFile(env, file, src)
			check(ss, fds)
		}
		check(parseCodex(env, []Loaded{{File: user, Src: src}, {File: proj, Src: src}}))
	})
}

func TestCodexEnvHeaderKeepsStaticValue(t *testing.T) {
	var s Server
	entry := map[string]any{"url": "https://h", "http_headers": map[string]any{"X-Key": "CANARY-v"}, "env_http_headers": map[string]any{"X-Key": "K"}}
	if !codexEntry(&s, entry, func(string, string) { t.Error("bad") }) {
		t.Fatal("rejected")
	}
	if got := secretMap(s.Headers); len(got) != 1 || got["X-Key"] != "CANARY-v" {
		t.Errorf("headers = %v", got)
	}
}

package clients

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/alestormoody/mcp-doctor/internal/diag"
)

const ccDir = "testdata/claudecode/parse"

// ccView is the comparable shape of a Server (secrets revealed, tests only).
type ccView struct {
	Name, Scope, Label, Pos string
	Transport               string
	Inferred                bool
	Command                 string
	Args                    []string
	Env, Headers            map[string]string
	URL                     string
	Refs                    []VarRef
	Unknown                 []string
	Disabled, Shadowed      bool
}

func ccViews(ss []Server) []ccView {
	out := make([]ccView, 0, len(ss))
	for _, s := range ss {
		var args []string
		if s.Args != nil {
			args = secrets(s.Args)
		}
		out = append(out, ccView{Name: s.Name, Scope: string(s.Source.Scope), Label: s.Source.Label,
			Pos: strconv.Itoa(s.Source.Line) + ":" + strconv.Itoa(s.Source.Col), Transport: s.Transport,
			Inferred: s.TransportInferred, Command: s.Command, Args: args, Env: secretMap(s.Env),
			Headers: secretMap(s.Headers), URL: s.URL.Reveal(), Refs: s.Refs, Unknown: s.Unknown,
			Disabled: s.Disabled, Shadowed: s.Shadowed})
	}
	return out
}

func ccRead(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(ccDir, name)) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ccUser is user.json with the "<CWD>" project key set to cwd.
func ccUser(t testing.TB, cwd string) []byte {
	t.Helper()
	key, err := json.Marshal(cwd)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Replace(ccRead(t, "user.json"), []byte(`"<CWD>"`), key, 1)
}

func ccParse(t *testing.T, name string) ([]Server, []diag.Finding) {
	t.Helper()
	ss, dis, fds := claudeCodeFile(fakeEnv("linux", nil), jsonFile("/p", FormatJSON, ScopeProject), ccRead(t, name))
	if len(dis) != 0 {
		t.Errorf("%s: disabled from a project file: %q", name, dis)
	}
	parseCheckFindings(t, fds)
	for _, f := range fds {
		if f.Client != "claude-code" {
			t.Errorf("client %q: %+v", f.Client, f)
		}
	}
	return ss, fds
}

func TestClaudeCodeParseValid(t *testing.T) {
	ss, fds := ccParse(t, "valid-project.json")
	if len(fds) != 0 {
		t.Fatalf("findings: %+v", fds)
	}
	want := []ccView{
		{Name: "local-tool", Scope: "project", Pos: "3:5", Transport: "stdio", Inferred: true, Command: "npx",
			Args: []string{"-y", "${X:-CANARY-def}"}, Env: map[string]string{"API_TOKEN": "${TOKEN}"},
			Refs: []VarRef{{Field: "args[1]", Kind: "env", Name: "X", HasDefault: true}, {Field: "env.API_TOKEN", Kind: "env", Name: "TOKEN"}}},
		{Name: "remote", Scope: "project", Pos: "8:5", Transport: "http", URL: "https://api.example/mcp/CANARY-p",
			Headers: map[string]string{"Authorization": "Bearer CANARY-h"}},
		{Name: "events", Scope: "project", Pos: "13:5", Transport: "sse", URL: "https://sse.example/CANARY-s"},
		{Name: "socket", Scope: "project", Pos: "14:5", Transport: "ws", URL: "wss://ws.example/CANARY-w"},
		{Name: "streamed", Scope: "project", Pos: "15:5", Transport: "http", URL: "https://st.example/CANARY-t"},
		{Name: "explicit", Scope: "project", Pos: "16:5", Transport: "stdio", Command: "uvx"},
	}
	if got := ccViews(ss); !reflect.DeepEqual(got, want) {
		t.Errorf("servers:\n got %+v\nwant %+v", got, want)
	}
	for _, s := range ss {
		if s.Source.Client != "claude-code" || s.Source.Path != "/p" || s.Source.Format != FormatJSON {
			t.Errorf("source %+v", s.Source)
		}
	}
}

func TestClaudeCodeParseIncomplete(t *testing.T) {
	src := ccRead(t, "incomplete.json")
	ss, fds := ccParse(t, "incomplete.json")
	if got := parseCodes(fds); got != "MCP_PARSE_003,MCP_PARSE_008,MCP_PARSE_008,MCP_PARSE_002,MCP_PARSE_005" {
		t.Fatalf("codes %s: %+v", got, fds)
	}
	for i, name := range []string{"no-target", "url-no-type", "http-no-url", "bad-args", "typo"} {
		if fds[i].Server != name {
			t.Errorf("finding %d server %q, want %q", i, fds[i].Server, name)
		}
	}
	if !strings.Contains(fds[1].Message, "no transport type") || !strings.Contains(fds[2].Message, "no url") {
		t.Errorf("008 messages: %q / %q", fds[1].Message, fds[2].Message)
	}
	// 002 positioned at the args value
	for i, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, `"bad-args"`) {
			want := "/p:" + strconv.Itoa(i+1) + ":" + strconv.Itoa(strings.Index(line, `"CANARY-a"`)+1)
			if fds[3].Source != want {
				t.Errorf("002 at %s, want %s", fds[3].Source, want)
			}
		}
	}
	if !strings.Contains(fds[4].Message, "comand") {
		t.Errorf("005 message %q", fds[4].Message)
	}
	var names []string
	for _, s := range ss {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "typo,ok" {
		t.Errorf("kept %q", names)
	}
	if !reflect.DeepEqual(ss[0].Unknown, []string{"comand"}) || ss[0].Command != "y" {
		t.Errorf("typo: %+v", ccViews(ss[:1]))
	}
}

func TestClaudeCodeParseCorrupted(t *testing.T) {
	ss, fds := ccParse(t, "corrupted.json")
	if len(ss) != 0 || parseCodes(fds) != "MCP_PARSE_001" || !regexp.MustCompile(`^/p:\d+:\d+$`).MatchString(fds[0].Source) {
		t.Errorf("corrupted: %d servers, %+v", len(ss), fds)
	}
	ss, fds = ccParse(t, "not-object.json")
	if len(ss) != 0 || parseCodes(fds) != "MCP_PARSE_002" {
		t.Errorf("not-object: %d servers, %+v", len(ss), fds)
	}
	ss, _, fds = claudeCodeFile(fakeEnv("linux", nil), jsonFile("/p", FormatJSON, ScopeUser), []byte("[1]"))
	if len(ss) != 0 || parseCodes(fds) != "MCP_PARSE_002" || fds[0].Source != "/p:1:1" {
		t.Errorf("array root: %d servers, %+v", len(ss), fds)
	}
	parseCheckFindings(t, fds)
}

func TestClaudeCodeUserProjects(t *testing.T) {
	env := fakeEnv("linux", nil)
	f := jsonFile("/h/.claude.json", FormatJSON, ScopeUser)
	cases := []struct{ name, src, codes, servers, disabled string }{
		{"no cwd match", `{"projects": {"/x": {"mcpServers": {"a": {"command": "c"}}}}}`, "", "", ""},
		{"projects not object", `{"projects": [], "mcpServers": {"u": {"command": "c"}}}`, "MCP_PARSE_002", "user u", ""},
		{"project not object", `{"projects": {"/w/proj": 1}}`, "MCP_PARSE_002", "", ""},
		{"disabled wrong type", `{"projects": {"/w/proj": {"disabledMcpServers": "a"}}}`, "MCP_PARSE_002", "", ""},
		{"duplicate cwd key", `{"projects": {"/w/proj": {}, "/o": {}, "/o": {}, "/w/proj/": {"disabledMcpServers": ["a"]}}}`, "", "", "a"},
		{"duplicate inside cwd", `{"projects": {"/w/proj": {"disabledMcpServers": ["a"], "disabledMcpServers": ["b"]}}}`, "MCP_PARSE_004", "", "b"},
		{"local", `{"projects": {"/w/proj": {"mcpServers": {"l": {"command": "c"}}}}}`, "", "local l project /w/proj", ""},
	}
	for _, c := range cases {
		ss, dis, fds := claudeCodeFile(env, f, []byte(c.src))
		parseCheckFindings(t, fds)
		var got []string
		for _, s := range ss {
			got = append(got, strings.TrimSpace(string(s.Source.Scope)+" "+s.Name+" "+s.Source.Label))
		}
		if parseCodes(fds) != c.codes || strings.Join(got, "|") != c.servers || strings.Join(dis, ",") != c.disabled {
			t.Errorf("%s: codes %q servers %q disabled %q", c.name, parseCodes(fds), got, dis)
		}
	}
	// Cwd "" matches no project; a project file never yields disabled names.
	noCwd := env
	noCwd.Cwd = ""
	if ss, dis, _ := claudeCodeFile(noCwd, f, []byte(`{"projects": {"": {"disabledMcpServers": ["a"]}}}`)); len(ss)+len(dis) != 0 {
		t.Errorf("empty cwd: %v %q", ss, dis)
	}
	pf := jsonFile("/w/proj/.mcp.json", FormatJSON, ScopeProject)
	if ss, dis, _ := claudeCodeFile(env, pf, []byte(`{"projects": {"/w/proj": {"disabledMcpServers": ["a"], "mcpServers": {"l": {"command": "c"}}}}}`)); len(ss)+len(dis) != 0 {
		t.Errorf("projects in a project file: %v %q", ss, dis)
	}
}

// ccClient is the real Claude Code row with the managed file moved under root.
func ccClient(root string) Client {
	return Client{ID: "claude-code", Name: "Claude Code", homes: claudeCodeHomes, parse: parseClaudeCode, rank: claudeCodeRank,
		candidates: func(env Env) []Candidate {
			cs := claudeCodeCandidates(env)
			cs[0].Path = filepath.Join(root, "managed", "managed-mcp.json")
			return cs
		}}
}

// ccPlace writes the user file (with its cwd key), .mcp.json and, if managed, the managed file.
func ccPlace(t *testing.T, env Env, root string, user, managed bool) {
	t.Helper()
	if user {
		writeFileBytes(t, filepath.Join(env.Home, ".claude.json"), string(ccUser(t, env.Cwd)))
	}
	place(t, filepath.Join(ccDir, "project.json"), filepath.Join(env.Cwd, ".mcp.json"))
	if managed {
		place(t, filepath.Join(ccDir, "managed.json"), filepath.Join(root, "managed", "managed-mcp.json"))
	}
}

func ccSummary(ss []Server) []string {
	var out []string
	for _, s := range ss {
		line := string(s.Source.Scope) + " " + s.Name + " " + s.Command + s.URL.Reveal()
		if s.Disabled {
			line += " disabled"
		}
		if s.Shadowed {
			line += " shadowed"
		}
		out = append(out, line)
	}
	return out
}

func TestClaudeCodeLocalScope(t *testing.T) {
	env, root := fixtureEnv(t)
	loadSwapClients(t, ccClient(root))
	ccPlace(t, env, root, true, false)
	_, ss, fds := Load(env)
	parseCheckFindings(t, fds)
	if len(fds) != 0 {
		t.Errorf("findings %+v", fds)
	}
	want := []string{
		"user shared user-shared disabled shadowed",
		"user user-only user-only",
		"local shared local-shared disabled",
		"local local-only local-only",
		"project shared project-shared disabled shadowed",
		"project proj-off proj-off disabled",
	}
	if got := ccSummary(ss); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("servers:\n got %q\nwant %q", got, want)
	}
	for _, s := range ss {
		if s.Source.Scope == ScopeLocal && (s.Source.Label != "project "+env.Cwd || s.Source.Path != filepath.Join(env.Home, ".claude.json")) {
			t.Errorf("local source %+v", s.Source)
		}
	}
}

func TestClaudeCodePrecedence(t *testing.T) {
	env, root := fixtureEnv(t)
	loadSwapClients(t, ccClient(root))
	ccPlace(t, env, root, true, true)
	_, ss, _ := Load(env)
	var shared []string
	for _, s := range ccSummary(ss) {
		if strings.Contains(s, " shared ") {
			shared = append(shared, s)
		}
	}
	want := "managed shared https://managed.example/mcp disabled|user shared user-shared disabled shadowed|" +
		"local shared local-shared disabled shadowed|project shared project-shared disabled shadowed"
	if strings.Join(shared, "|") != want {
		t.Errorf("with managed: %q", shared)
	}

	if err := os.Remove(filepath.Join(root, "managed", "managed-mcp.json")); err != nil {
		t.Fatal(err)
	}
	_, ss, _ = Load(env)
	for _, s := range ss {
		if s.Name == "shared" && s.Shadowed == (s.Source.Scope == ScopeLocal) {
			t.Errorf("without managed: %+v", ccSummary([]Server{s}))
		}
	}
}

func TestClaudeCodeReadOnce(t *testing.T) {
	for _, withUser := range []bool{true, false} {
		env, root := fixtureEnv(t)
		loadSwapClients(t, ccClient(root))
		ccPlace(t, env, root, withUser, true)
		reads := map[string]int{}
		env.ReadFile = func(p string) ([]byte, error) {
			reads[p]++
			return os.ReadFile(p) // #nosec G304 -- test fixture path
		}
		_, ss, _ := Load(env)
		userPath := filepath.Join(env.Home, ".claude.json")
		if want := map[bool]int{true: 1, false: 0}[withUser]; reads[userPath] != want {
			t.Errorf("user file present=%v read %d times", withUser, reads[userPath])
		}
		if len(reads) != map[bool]int{true: 3, false: 2}[withUser] {
			t.Errorf("reads %v", reads)
		}
		for _, s := range ss {
			if !withUser && s.Disabled {
				t.Errorf("disabled without the user file: %s", s.Name)
			}
		}
	}
	// A user file that cannot be read disables nothing and is not read again.
	env, root := fixtureEnv(t)
	loadSwapClients(t, ccClient(root))
	ccPlace(t, env, root, true, false)
	n := 0
	env.ReadFile = func(p string) ([]byte, error) {
		if strings.HasSuffix(p, ".claude.json") {
			n++
			return nil, errors.New("boom")
		}
		return os.ReadFile(p) // #nosec G304 -- test fixture path
	}
	_, ss, _ := Load(env)
	for _, s := range ss {
		if s.Disabled {
			t.Errorf("disabled with an unreadable user file: %s", s.Name)
		}
	}
	if n != 1 {
		t.Errorf("unreadable user file read %d times", n)
	}
}

func TestClaudeCodeRawPreserved(t *testing.T) {
	for _, name := range []string{"valid-project.json", "incomplete.json"} {
		src := ccRead(t, name)
		ss, _ := ccParse(t, name)
		for _, s := range ss {
			key := []byte(strconv.Quote(s.Name) + ": ")
			i := bytes.Index(src, key)
			if i < 0 || s.raw == nil || !bytes.HasPrefix(src[i+len(key):], *s.raw) || !bytes.HasSuffix(*s.raw, []byte("}")) {
				t.Errorf("%s %s: raw not the entry slice", name, s.Name)
			}
		}
		if name == "incomplete.json" && !bytes.Equal(*ss[0].raw, []byte(`{"comand": "x", "command": "y"}`)) {
			t.Errorf("typo raw %q", *ss[0].raw)
		}
	}
}

func TestClaudeCodeRank(t *testing.T) {
	for sc, want := range map[Scope]int{ScopeManaged: 4, ScopeLocal: 3, ScopeProject: 2, ScopeUser: 1} {
		if r, ok := claudeCodeRank(Source{Scope: sc}); !ok || r != want {
			t.Errorf("%s: %d %v", sc, r, ok)
		}
	}
	if _, ok := claudeCodeRank(Source{Scope: "other"}); ok {
		t.Error("unknown scope ranked")
	}
}

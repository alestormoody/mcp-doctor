package clients

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alestormoody/mcp-doctor/internal/diag"
)

// fullFixture places every client's user and project files in one fixtureEnv tree:
// Codex with a profile and a .git one level above Cwd, VS Code with one profile.
func fullFixture(t *testing.T) (env Env, root string) {
	t.Helper()
	env, root = fixtureEnv(t)
	if env.GOOS == "windows" {
		vars := map[string]string{
			"APPDATA":      filepath.Join(root, "AppData", "Roaming"),
			"LOCALAPPDATA": filepath.Join(root, "AppData", "Local"),
		}
		env.Getenv = mapEnv(vars)
	}
	h, w, ucd := env.Home, env.Cwd, userConfigDir(env)
	work := filepath.Dir(w)
	if err := os.MkdirAll(filepath.Join(work, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := [][2]string{
		{"testdata/claudecode/claude.json", filepath.Join(h, ".claude.json")},
		{"testdata/claudecode/mcp.json", filepath.Join(w, ".mcp.json")},
		{"testdata/codex/config.toml", filepath.Join(h, ".codex", "config.toml")},
		{"testdata/codex/work.config.toml", filepath.Join(h, ".codex", "work.config.toml")},
		{"testdata/codex/project.toml", filepath.Join(work, ".codex", "config.toml")},
		{"testdata/codex/project.toml", filepath.Join(w, ".codex", "config.toml")},
		{"testdata/claudedesktop/claude_desktop_config.json", filepath.Join(ucd, "Claude", claudeDesktopFile)},
		{"testdata/cursor/mcp.json", filepath.Join(h, ".cursor", "mcp.json")},
		{"testdata/cursor/mcp.json", filepath.Join(w, ".cursor", "mcp.json")},
		{"testdata/vscode/mcp.json", filepath.Join(ucd, "Code", "User", "mcp.json")},
		{"testdata/vscode/mcp.json", filepath.Join(ucd, "Code", "User", "profiles", "p1", "mcp.json")},
		{"testdata/vscode/mcp.json", filepath.Join(ucd, "Code - Insiders", "User", "mcp.json")},
		{"testdata/vscode/mcp-config.json", filepath.Join(h, ".copilot", "mcp-config.json")},
		{"testdata/vscode/mcp.json", filepath.Join(w, ".vscode", "mcp.json")},
	}
	if env.GOOS == "windows" {
		lad := localAppData(env)
		files = append(files,
			[2]string{"testdata/claudedesktop/claude_desktop_config.json",
				filepath.Join(lad, "Packages", "Claude_pzs8sxrjxfjjc", "LocalCache", "Roaming", "Claude", claudeDesktopFile)},
			[2]string{"testdata/claudedesktop/claude_desktop_config.json",
				filepath.Join(lad, "Claude-Data", "Claude_pzs8sxrjxfjjc", claudeDesktopFile)})
	}
	for _, f := range files {
		place(t, f[0], f[1])
	}
	return env, root
}

func TestDiscoverAllFound(t *testing.T) {
	env, _ := fullFixture(t)
	results, findings := Discover(env)
	if len(findings) != 0 {
		t.Errorf("findings = %+v", findings)
	}
	wantFiles := map[string]int{"claude-code": 2, "codex": 4, "claude-desktop": 1, "cursor": 2, "vscode": 6}
	if env.GOOS == "windows" {
		wantFiles["claude-desktop"] = 3
	}
	for _, r := range results {
		if !r.Detected {
			t.Errorf("%s not detected", r.ID)
		}
		n := 0
		for _, f := range r.Files {
			if f.Scope == ScopeManaged {
				continue
			}
			n++
			if f.Status != StatusFound {
				t.Errorf("%s: %s is %s (%s)", r.ID, f.Path, f.Status, f.Reason)
			}
		}
		if n != wantFiles[r.ID] {
			t.Errorf("%s: %d non-managed files, want %d: %+v", r.ID, n, wantFiles[r.ID], r.Files)
		}
	}
}

func TestSharedMcpJSONNotDetection(t *testing.T) {
	env, _ := fixtureEnv(t)
	shared := filepath.Join(env.Cwd, ".mcp.json")
	place(t, "testdata/claudecode/mcp.json", shared)
	results, findings := Discover(env)
	if len(findings) != 0 {
		t.Errorf("findings = %+v", findings)
	}
	for _, r := range results {
		if r.Detected {
			t.Errorf("%s detected from the shared file alone", r.ID)
		}
		st, listed := statuses(r)[filepath.ToSlash(shared)]
		wantListed := r.ID == "claude-code" || r.ID == "vscode"
		if listed != wantListed || (listed && st != StatusFound) {
			t.Errorf("%s: shared file listed=%v status=%q", r.ID, listed, st)
		}
	}
}

func TestLoadAllClients(t *testing.T) {
	env, _ := fullFixture(t)
	results, servers, findings := Load(env)
	for _, f := range findings {
		if f.Severity == diag.SeverityError || f.Severity == diag.SeverityCritical {
			t.Errorf("error finding: %+v", f)
		}
	}
	found := map[string]bool{}
	for _, r := range results {
		for _, f := range r.Files {
			if f.Status == StatusFound {
				found[r.ID+"\x00"+f.Path] = true
			}
		}
	}
	perClient := map[string]int{}
	for _, s := range servers {
		perClient[s.Source.Client]++
		if !filepath.IsAbs(s.Source.Path) || !found[s.Source.Client+"\x00"+s.Source.Path] {
			t.Errorf("%s/%s: source %s is not a found file of that client", s.Source.Client, s.Name, s.Source.Path)
		}
		if s.Source.Line <= 0 {
			t.Errorf("%s/%s: line %d", s.Source.Client, s.Name, s.Source.Line)
		}
	}
	for _, r := range results {
		if perClient[r.ID] == 0 {
			t.Errorf("no server for %s", r.ID)
		}
	}
}

func TestSharedMcpJSONParsedPerClient(t *testing.T) {
	env, _ := fixtureEnv(t)
	shared := filepath.Join(env.Cwd, ".mcp.json")
	writeFileBytes(t, shared, `{"inputs": [], "mcpServers": {"a": {"command": "x", "args": ["${A:-x}"]}}}`)
	_, servers, findings := Load(env)
	byClient := map[string][]Server{}
	for _, s := range servers {
		if s.Source.Path == shared {
			byClient[s.Source.Client] = append(byClient[s.Source.Client], s)
		}
	}
	want := []VarRef{{Field: "args[0]", Kind: "env", Name: "A", HasDefault: true}}
	for _, id := range []string{"claude-code", "vscode"} {
		ss := byClient[id]
		if len(ss) != 1 || ss[0].Name != "a" {
			t.Errorf("%s: servers %+v", id, ss)
			continue
		}
		if !reflect.DeepEqual(ss[0].Refs, want) {
			t.Errorf("%s: refs %+v, want %+v", id, ss[0].Refs, want)
		}
	}
	if len(byClient) != 2 {
		t.Errorf("shared file parsed by %d clients, want 2", len(byClient))
	}
	for _, f := range findings {
		if f.Code == "MCP_PARSE_005" && f.Client != "vscode" {
			t.Errorf("005 for %s: %+v", f.Client, f)
		}
	}
	if !strings.Contains(parseCodes(findings), "MCP_PARSE_005") {
		t.Errorf("no 005 for inputs in the portable file: %s", parseCodes(findings))
	}
}

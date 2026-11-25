package clients

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeCodeCandidates(t *testing.T) {
	user := "user /h/.claude.json json"
	project := "project /w/proj/.mcp.json json (shared)"
	winDefault := filepath.ToSlash(filepath.Join(`C:\Program Files`, "ClaudeCode", "managed-mcp.json"))
	cases := []struct {
		goos string
		vars map[string]string
		want []string
	}{
		{"darwin", nil, []string{"managed /Library/Application Support/ClaudeCode/managed-mcp.json json", user, project}},
		{"linux", nil, []string{"managed /etc/claude-code/managed-mcp.json json", user, project}},
		{"windows", map[string]string{"ProgramFiles": "/pf"}, []string{"managed /pf/ClaudeCode/managed-mcp.json json", user, project}},
		{"windows", nil, []string{"managed " + winDefault + " json", user, project}},
		{"linux", map[string]string{"CLAUDE_CONFIG_DIR": "/cfg"},
			[]string{"managed /etc/claude-code/managed-mcp.json json", "user /cfg/.claude.json json CLAUDE_CONFIG_DIR", project}},
	}
	for _, c := range cases {
		got := slashList(claudeCodeCandidates(fakeEnv(c.goos, c.vars)))
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s %v:\n got %q\nwant %q", c.goos, c.vars, got, c.want)
		}
	}
}

func TestClaudeCodeHomes(t *testing.T) {
	for vars, want := range map[string]string{"": "/h/.claude", "/cfg": "/cfg"} {
		got := claudeCodeHomes(fakeEnv("linux", map[string]string{"CLAUDE_CONFIG_DIR": vars}))
		if len(got) != 1 || filepath.ToSlash(got[0]) != want {
			t.Errorf("CLAUDE_CONFIG_DIR=%q: got %q, want [%s]", vars, got, want)
		}
	}
}

func TestClaudeCodeFixture(t *testing.T) {
	c := lookupClient(t, "claude-code")
	userOf := func(env Env) string { return filepath.ToSlash(filepath.Join(env.Home, ".claude.json")) }
	projOf := func(env Env) string { return filepath.ToSlash(filepath.Join(env.Cwd, ".mcp.json")) }

	env, _ := fixtureEnv(t)
	place(t, "testdata/claudecode/claude.json", filepath.Join(env.Home, ".claude.json"))
	place(t, "testdata/claudecode/mcp.json", filepath.Join(env.Cwd, ".mcp.json"))
	r, _ := discoverOne(env, c)
	if st := statuses(r); st[userOf(env)] != StatusFound || st[projOf(env)] != StatusFound || !r.Detected {
		t.Errorf("both placed: %+v", r)
	}

	env, _ = fixtureEnv(t)
	r, _ = discoverOne(env, c)
	if st := statuses(r); st[userOf(env)] != StatusMissing || st[projOf(env)] != StatusMissing {
		t.Errorf("empty: %+v", r)
	}
	if r.Detected && !managedPresent(r) {
		t.Errorf("empty: Detected without a managed file: %+v", r)
	}

	env, _ = fixtureEnv(t)
	place(t, "testdata/claudecode/mcp.json", filepath.Join(env.Cwd, ".mcp.json"))
	r, _ = discoverOne(env, c)
	if statuses(r)[projOf(env)] != StatusFound || (r.Detected && !managedPresent(r)) {
		t.Errorf("only shared .mcp.json: %+v", r)
	}
}

// managedPresent reports whether the machine has a real managed file, which tests must not depend on.
func managedPresent(r Result) bool {
	for _, f := range r.Files {
		if f.Scope == ScopeManaged && f.Status != StatusMissing {
			return true
		}
	}
	return false
}

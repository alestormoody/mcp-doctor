package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alestormoody/mcp-doctor/internal/clients"
)

// withEnv points discovery at real temp dirs (home path has a space and non-ASCII) and returns home and cwd.
func withEnv(t *testing.T) (home, cwd string) {
	t.Helper()
	root := t.TempDir()
	home, cwd = filepath.Join(root, "home dir ü"), filepath.Join(root, "proj")
	setEnv(t, root, home, cwd)
	return home, cwd
}

func setEnv(t *testing.T, root, home, cwd string) {
	t.Helper()
	saved := discoverEnv
	t.Cleanup(func() { discoverEnv = saved })
	env := osEnv()
	env.Home, env.Cwd = home, cwd
	env.Getenv = func(k string) string {
		if runtime.GOOS == "windows" && (k == "APPDATA" || k == "LOCALAPPDATA") {
			return filepath.Join(root, k)
		}
		return ""
	}
	discoverEnv = func() clients.Env { return env }
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// lines returns stdout with every line whitespace-collapsed.
func lines(out string) []string {
	var ls []string
	for _, l := range strings.Split(out, "\n") {
		ls = append(ls, collapse(l))
	}
	return ls
}

func hasLine(out string, parts ...string) bool {
	want := collapse(strings.Join(parts, " "))
	for _, l := range lines(out) {
		if l == want {
			return true
		}
	}
	return false
}

func mkfile(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClientsListsAllClients(t *testing.T) {
	home, cwd := withEnv(t)
	code, out, errw := run("clients")
	if code != 0 || errw != "" {
		t.Fatalf("code=%d err=%q", code, errw)
	}
	last := -1
	for _, name := range []string{"Claude Code", "Codex", "Claude Desktop", "Cursor", "VS Code"} {
		i := strings.Index(out, "\n"+name+" ")
		if strings.HasPrefix(out, name+" ") {
			i = 0
		}
		if i <= last {
			t.Fatalf("%s out of order (%d <= %d):\n%s", name, i, last, out)
		}
		last = i
	}
	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, "  ") {
			continue
		}
		switch strings.Fields(l)[0] {
		case "managed", "user", "project":
		default:
			t.Errorf("candidate line without scope: %q", l)
		}
	}
	if !hasLine(out, "Cursor not detected") ||
		!hasLine(out, "user", filepath.Join(home, ".cursor", "mcp.json"), "missing") ||
		!hasLine(out, "project", filepath.Join(cwd, ".cursor", "mcp.json"), "missing") {
		t.Errorf("cursor block wrong:\n%s", out)
	}
}

func TestClientsDetected(t *testing.T) {
	home, _ := withEnv(t)
	p := filepath.Join(home, ".cursor", "mcp.json")
	mkfile(t, p)
	_, out, _ := run("clients")
	if !hasLine(out, "Cursor detected") || !hasLine(out, "user", p, "found") {
		t.Errorf("out:\n%s", out)
	}
}

func TestClientsUnreadable(t *testing.T) {
	home, _ := withEnv(t)
	p := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(p, 0o750); err != nil {
		t.Fatal(err)
	}
	code, out, errw := run("clients")
	if code != 0 || errw != "" {
		t.Fatalf("code=%d err=%q", code, errw)
	}
	if !hasLine(out, "user", p, "unreadable", "not a regular file") {
		t.Errorf("no unreadable line:\n%s", out)
	}
	warn := "WARNING MCP_DISC_001 " + p + ": Config file cannot be read: not a regular file"
	i, j := strings.Index(out, warn), strings.Index(out, "VS Code")
	if i < 0 || i < j {
		t.Errorf("warning missing or before table:\n%s", out)
	}
}

func TestClientsLabels(t *testing.T) {
	_, cwd := withEnv(t)
	p := filepath.Join(cwd, ".mcp.json")
	mkfile(t, p)
	_, out, _ := run("clients")
	_, vs, ok := strings.Cut(out, "VS Code")
	if !ok {
		t.Fatalf("no VS Code block:\n%s", out)
	}
	if !hasLine(vs, "project", p, "found", "shared with Claude Code") {
		t.Errorf("out:\n%s", vs)
	}
}

func TestClientsEmptyHome(t *testing.T) {
	root := t.TempDir()
	setEnv(t, root, "", filepath.Join(root, "proj"))
	code, out, _ := run("clients")
	if code != 0 || !strings.Contains(out, "WARNING MCP_DISC_002: Home directory") {
		t.Errorf("code=%d out:\n%s", code, out)
	}
}

func TestClientsExtraArgs(t *testing.T) {
	if code, out, _ := run("clients", "extra"); code != 2 || out != "" {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestOSEnv(t *testing.T) {
	env := osEnv()
	if env.GOOS != runtime.GOOS || !filepath.IsAbs(env.Cwd) {
		t.Errorf("GOOS=%q Cwd=%q", env.GOOS, env.Cwd)
	}
	if env.Getenv == nil || env.Stat == nil || env.Lstat == nil || env.ReadDir == nil || env.Open == nil {
		t.Fatal("nil func in osEnv")
	}
	if _, err := env.Open(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("Open of a missing file succeeded")
	}
}

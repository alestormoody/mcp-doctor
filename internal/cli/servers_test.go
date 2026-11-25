package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// userConfig mirrors clients.userConfigDir for the env setEnv builds.
func userConfig(home string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support")
	case "windows":
		return filepath.Join(filepath.Dir(home), "APPDATA")
	default:
		return filepath.Join(home, ".config")
	}
}

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeAllClients places one valid file per client and returns their paths in client order.
func writeAllClients(t *testing.T, home, cwd string) (cc, codex, desktop, cursor, vscode string) {
	t.Helper()
	cc = filepath.Join(cwd, ".mcp.json")
	write(t, cc, "{\n  \"mcpServers\": {\n    \"fetch\": {\"command\": \"uvx\", \"args\": [\"mcp-server-fetch\"]}\n  }\n}\n")
	codex = filepath.Join(home, ".codex", "config.toml")
	write(t, codex, "[mcp_servers.docs]\nurl = \"https://h.example/mcp\"\n")
	desktop = filepath.Join(userConfig(home), "Claude", "claude_desktop_config.json")
	write(t, desktop, "{\"mcpServers\": {\"files\": {\"command\": \"npx\"}}}\n")
	cursor = filepath.Join(home, ".cursor", "mcp.json")
	write(t, cursor, "{\"mcpServers\": {\"remote\": {\"url\": \"https://c.example:8443\"}}}\n")
	vscode = filepath.Join(userConfig(home), "Code", "User", "mcp.json")
	write(t, vscode, "{\"servers\": {\"gh\": {\"type\": \"http\", \"url\": \"https://api.example/mcp?k=v\"}}}\n")
	return
}

func TestServersListsAll(t *testing.T) {
	home, cwd := withEnv(t)
	cc, codex, desktop, cursor, vscode := writeAllClients(t, home, cwd)
	code, out, errw := run("servers")
	if code != 0 || errw != "" {
		t.Fatalf("code=%d err=%q out:\n%s", code, errw, out)
	}
	if !strings.HasPrefix(out, "CLIENT ") || !hasLine(out, "CLIENT SCOPE NAME TRANSPORT TARGET NOTE SOURCE") {
		t.Errorf("no header:\n%s", out)
	}
	want := [][]string{
		{"Claude Code project fetch stdio (inferred) uvx -", cc + ":3:5"},
		{"Codex user docs http (inferred) https://h.example/… -", codex + ":1:"},
		{"Claude Desktop user files stdio (inferred) npx -", desktop + ":1:"},
		{"Cursor user remote http (inferred) https://c.example:8443 -", cursor + ":1:"},
		{"VS Code user gh http https://api.example/… -", vscode + ":1:"},
		{"VS Code project fetch stdio (inferred) uvx shared with Claude Code", cc + ":3:5"},
	}
	ls := lines(out)
	if len(ls) != len(want)+2 || ls[len(ls)-1] != "" { // header, rows, trailing ""
		t.Fatalf("want %d rows:\n%s", len(want), out)
	}
	for i, w := range want {
		row := ls[i+1]
		if !strings.HasPrefix(row, w[0]+" "+w[1]) {
			t.Errorf("row %d = %q, want prefix %q", i, row, w[0]+" "+w[1])
		}
	}
}

func TestServersNone(t *testing.T) {
	withEnv(t)
	code, out, errw := run("servers")
	if code != 0 || errw != "" || out != "No MCP servers found.\n" {
		t.Errorf("code=%d out=%q err=%q", code, out, errw)
	}
}

func TestServersFindings(t *testing.T) {
	home, _ := withEnv(t)
	p := filepath.Join(home, ".cursor", "mcp.json")
	write(t, p, "{\"mcpServers\": {\"x\": {\"command\": \"a\",}\n")
	code, out, _ := run("servers")
	if code != 1 || strings.Contains(out, "Cursor ") {
		t.Errorf("code=%d out:\n%s", code, out)
	}
	if !strings.HasPrefix(out, "No MCP servers found.\n\nERROR MCP_PARSE_001 "+p+":") ||
		!strings.Contains(out, ": Syntax error: ") {
		t.Errorf("no syntax finding after a blank line:\n%s", out)
	}
}

func TestServersNotes(t *testing.T) {
	home, cwd := withEnv(t)
	write(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers.off]\ncommand = \"x\"\nenabled = false\n")
	write(t, filepath.Join(home, ".claude.json"), "{\"mcpServers\": {\"dup\": {\"command\": \"u\"}}}\n")
	write(t, filepath.Join(cwd, ".mcp.json"), "{\"mcpServers\": {\"dup\": {\"command\": \"p\"}}}\n")
	code, out, _ := run("servers")
	if code != 0 {
		t.Fatalf("code=%d out:\n%s", code, out)
	}
	for _, want := range []string{
		"Codex user off stdio (inferred) x disabled ",
		"Claude Code user dup stdio (inferred) u shadowed ",
		"Claude Code project dup stdio (inferred) p - ",
	} {
		found := false
		for _, l := range lines(out) {
			found = found || strings.HasPrefix(l, want)
		}
		if !found {
			t.Errorf("no row %q:\n%s", want, out)
		}
	}
}

func TestServersExtraArgs(t *testing.T) {
	if code, out, _ := run("servers", "x"); code != 2 || out != "" {
		t.Errorf("code=%d out=%q", code, out)
	}
}

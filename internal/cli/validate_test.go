package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateClean(t *testing.T) {
	home, _ := withEnv(t)
	write(t, filepath.Join(home, ".cursor", "mcp.json"), "{\"mcpServers\": {\"a\": {\"command\": \"x\"}}}\n")
	write(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers.b]\ncommand = \"y\"\n")
	code, out, errw := run("validate")
	if code != 0 || errw != "" || out != "2 files checked, 2 servers, 0 errors, 0 warnings\n" {
		t.Errorf("code=%d out=%q err=%q", code, out, errw)
	}
}

func TestValidateNone(t *testing.T) {
	withEnv(t)
	code, out, _ := run("validate")
	if code != 0 || out != "0 files checked, 0 servers, 0 errors, 0 warnings\n" {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestValidateErrors(t *testing.T) {
	home, _ := withEnv(t)
	cursor := filepath.Join(home, ".cursor", "mcp.json")
	write(t, cursor, "{\"mcpServers\": {\n")
	cc := filepath.Join(home, ".claude.json")
	write(t, cc, "{\"mcpServers\": {\"r\": {\"url\": \"https://h.example\"}}}\n")
	desktop := filepath.Join(userConfig(home), "Claude", "claude_desktop_config.json")
	write(t, desktop, "{\n  // note\n  \"mcpServers\": {\"f\": {\"command\": \"npx\"}}\n}\n")
	code, out, _ := run("validate")
	if code != 1 {
		t.Errorf("code=%d", code)
	}
	for _, prefix := range []string{
		"ERROR MCP_PARSE_001 " + cursor + ":",
		"ERROR MCP_PARSE_008 " + cc + ":1:",
		"WARNING MCP_PARSE_006 " + desktop + ":2:",
	} {
		if !strings.Contains(out, "\n"+prefix) && !strings.HasPrefix(out, prefix) {
			t.Errorf("no line starting %q:\n%s", prefix, out)
		}
	}
	if !strings.HasSuffix(out, "\n3 files checked, 1 server, 2 errors, 1 warning\n") {
		t.Errorf("summary wrong:\n%s", out)
	}
}

func TestValidateWarningsOnly(t *testing.T) {
	home, _ := withEnv(t)
	write(t, filepath.Join(home, ".cursor", "mcp.json"), "{\"mcpServers\": {\"a\": {\"command\": \"x\",},},}\n")
	code, out, _ := run("validate")
	if code != 0 || !strings.Contains(out, "WARNING MCP_PARSE_006 ") ||
		!strings.HasSuffix(out, "\n1 file checked, 1 server, 0 errors, 1 warning\n") {
		t.Errorf("code=%d out:\n%s", code, out)
	}
}

func TestValidateExtraArgs(t *testing.T) {
	if code, out, _ := run("validate", "x"); code != 2 || out != "" {
		t.Errorf("code=%d out=%q", code, out)
	}
}

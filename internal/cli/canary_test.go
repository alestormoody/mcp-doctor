package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestCanary puts a unique marker in every value position and in syntax-error tokens of all five clients'
// files, then proves no marker reaches stdout or stderr of servers and validate.
func TestCanary(t *testing.T) {
	home, cwd := withEnv(t)
	write(t, filepath.Join(home, ".claude.json"), `{"mcpServers": {
  "cc1": {"command": "run", "args": ["--token", "CANARYARG3"], "env": {"K": "CANARYENV1", "D": "${X:-CANARYDEF8}"}},
  "cc2": {"type": "http", "url": "https://u:CANARYUSR4@h.example/CANARYPATH5?q=CANARYQRY6", "headers": {"Authorization": "CANARYHDR2"}}
}}`)
	write(t, filepath.Join(home, ".codex", "config.toml"), `[mcp_servers.cx1]
url = "https://h.example/CANARYPATH5?k=CANARYQRY6"
http_headers = { "X-Key" = "CANARYHDR2" }
[mcp_servers.cx1.oauth]
client_secret = "CANARYSEC7"
[mcp_servers.cx2]
command = "run"
args = ["CANARYARG3"]
env = { K = "CANARYENV1" }
`)
	write(t, filepath.Join(cwd, ".codex", "config.toml"), "[mcp_servers.bad]\ncommand = CANARYSYN9\n")
	write(t, filepath.Join(userConfig(home), "Claude", "claude_desktop_config.json"),
		`{"mcpServers": {"cd1": {"command": "run", "args": ["CANARYARG3"], "env": {"K": "CANARYENV1"}}}}`)
	write(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers": {
  "cu1": {"url": "https://CANARYUSR4@h.example/CANARYPATH5", "headers": {"A": "CANARYHDR2"},
          "auth": {"CLIENT_ID": "id", "CLIENT_SECRET": "CANARYSEC7"}}
}}`)
	write(t, filepath.Join(cwd, ".cursor", "mcp.json"), `{"mcpServers": CANARYSYN9}`)
	write(t, filepath.Join(userConfig(home), "Code", "User", "mcp.json"), `{"servers": {
  "vs1": {"type": "http", "url": "https://h.example?t=CANARYQRY6", "headers": {"A": "CANARYHDR2"}},
  "vs2": {"command": "run", "args": ["CANARYARG3"], "env": {"K": "CANARYENV1"}}
}}`)

	for _, cmd := range []string{"servers", "validate"} {
		code, out, errw := run(cmd)
		if code != 1 {
			t.Errorf("%s: code=%d, want 1 (two syntax errors)", cmd, code)
		}
		if strings.Contains(out, "CANARY") || strings.Contains(errw, "CANARY") {
			t.Errorf("%s leaked a value:\nstdout:\n%s\nstderr:\n%s", cmd, out, errw)
		}
		if strings.Count(out, "MCP_PARSE_001") != 2 {
			t.Errorf("%s: want two syntax findings:\n%s", cmd, out)
		}
		if cmd == "validate" && !strings.Contains(out, " 8 servers, ") {
			t.Errorf("validate did not count the servers:\n%s", out)
		}
	}
	_, out, _ := run("servers")
	for _, name := range []string{"cc1", "cc2", "cx1", "cx2", "cd1", "cu1", "vs1", "vs2"} {
		if !strings.Contains(out, " "+name+" ") {
			t.Errorf("server %s not listed, the check would be vacuous:\n%s", name, out)
		}
	}
}

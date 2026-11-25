// Claude Code adapter: candidate config locations.
// Sources: https://code.claude.com/docs/en/mcp, https://code.claude.com/docs/en/managed-mcp.md,
// https://code.claude.com/docs/en/claude-directory

package clients

import "path/filepath"

func claudeCodeCandidates(env Env) []Candidate {
	var managed string
	switch env.GOOS {
	case "darwin":
		managed = "/Library/Application Support/ClaudeCode"
	case "windows":
		managed = env.Getenv("ProgramFiles")
		if managed == "" {
			managed = `C:\Program Files`
		}
		managed = filepath.Join(managed, "ClaudeCode")
	default:
		managed = "/etc/claude-code"
	}
	user := Candidate{Scope: ScopeUser, Path: filepath.Join(env.Home, ".claude.json"), Format: FormatJSON}
	if d := env.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		user.Path, user.Label = filepath.Join(d, ".claude.json"), "CLAUDE_CONFIG_DIR"
	}
	return []Candidate{
		{Scope: ScopeManaged, Path: filepath.Join(managed, "managed-mcp.json"), Format: FormatJSON},
		user,
		{Scope: ScopeProject, Path: filepath.Join(env.Cwd, ".mcp.json"), Format: FormatJSON, Shared: true},
	}
}

func claudeCodeHomes(env Env) []string {
	if d := env.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return []string{d}
	}
	return []string{filepath.Join(env.Home, ".claude")}
}

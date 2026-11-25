// Claude Desktop adapter: candidate config locations.
// Source: https://modelcontextprotocol.io/docs/develop/connect-local-servers

package clients

import (
	"path/filepath"

	"github.com/alestormoody/mcp-doctor/internal/diag"
)

const claudeDesktopFile = "claude_desktop_config.json"

func claudeDesktopCandidates(env Env) []Candidate {
	cs := []Candidate{{Scope: ScopeUser, Path: filepath.Join(userConfigDir(env), "Claude", claudeDesktopFile), Format: FormatJSON}}
	if env.GOOS == "windows" {
		// MSIX install paths are LOW confidence (https://github.com/anthropics/claude-code/issues/26073):
		// all are listed, none is assumed to be the live one.
		lad := localAppData(env)
		for _, p := range []string{
			filepath.Join(lad, "Packages", "Claude_pzs8sxrjxfjjc", "LocalCache", "Roaming", "Claude", claudeDesktopFile),
			filepath.Join(lad, "Claude-Data", "Claude_pzs8sxrjxfjjc", claudeDesktopFile),
		} {
			cs = append(cs, Candidate{Scope: ScopeUser, Path: p, Format: FormatJSON, Label: "MSIX"})
		}
	}
	return cs
}

func claudeDesktopHomes(env Env) []string {
	var hs []string
	for _, c := range claudeDesktopCandidates(env) {
		hs = append(hs, filepath.Dir(c.Path))
	}
	return hs
}

// parseClaudeDesktop maps the client's config files onto Servers. Placeholder: filled in by the client's parse plan.
func parseClaudeDesktop(Env, []Loaded) ([]Server, []diag.Finding) { return nil, nil }

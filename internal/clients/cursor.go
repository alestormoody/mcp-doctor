// Cursor adapter: candidate config locations.
// Source: https://cursor.com/docs/context/mcp

package clients

import (
	"path/filepath"

	"github.com/alestormoody/mcp-doctor/internal/diag"
)

func cursorCandidates(env Env) []Candidate {
	return []Candidate{
		{Scope: ScopeUser, Path: filepath.Join(env.Home, ".cursor", "mcp.json"), Format: FormatJSON},
		{Scope: ScopeProject, Path: filepath.Join(env.Cwd, ".cursor", "mcp.json"), Format: FormatJSON},
	}
}

func cursorHomes(env Env) []string { return []string{filepath.Join(env.Home, ".cursor")} }

// parseCursor maps the client's config files onto Servers. Placeholder: filled in by the client's parse plan.
func parseCursor(Env, []Loaded) ([]Server, []diag.Finding) { return nil, nil }

// VS Code adapter: candidate config locations.
// Sources: https://code.visualstudio.com/docs/agents/reference/mcp-configuration,
// https://code.visualstudio.com/docs/configure/profiles

package clients

import (
	"path/filepath"
	"strings"
)

var vscodeVariants = []struct{ dir, label string }{{"Code", ""}, {"Code - Insiders", "Insiders"}}

func vscodeUserDir(env Env, dir string) string { return filepath.Join(userConfigDir(env), dir, "User") }

// copilotHome is $COPILOT_HOME, else ~/.copilot; the label names the override when used.
func copilotHome(env Env) (dir, label string) {
	if d := env.Getenv("COPILOT_HOME"); d != "" {
		return d, "COPILOT_HOME"
	}
	return filepath.Join(env.Home, ".copilot"), ""
}

func vscodeCandidates(env Env) []Candidate {
	var cs []Candidate
	for _, v := range vscodeVariants {
		user := vscodeUserDir(env, v.dir)
		cs = append(cs, Candidate{Scope: ScopeUser, Path: filepath.Join(user, "mcp.json"), Format: FormatJSONC, Label: v.label})
		// The profiles/<id>/mcp.json layout is MEDIUM-LOW confidence; one level only, no recursion.
		profiles := filepath.Join(user, "profiles")
		entries, _ := env.ReadDir(profiles) // errors are reported by discoverOne
		for _, e := range entries {
			if e.IsDir() {
				cs = append(cs, Candidate{Scope: ScopeUser, Path: filepath.Join(profiles, e.Name(), "mcp.json"),
					Format: FormatJSONC, Label: strings.TrimSpace(v.label + " profile " + e.Name())})
			}
		}
	}
	cp, label := copilotHome(env)
	return append(cs,
		Candidate{Scope: ScopeUser, Path: filepath.Join(cp, "mcp-config.json"), Format: FormatJSON, Label: label},
		Candidate{Scope: ScopeProject, Path: filepath.Join(env.Cwd, ".vscode", "mcp.json"), Format: FormatJSONC},
		Candidate{Scope: ScopeProject, Path: filepath.Join(env.Cwd, ".mcp.json"), Format: FormatJSON,
			Label: "shared with Claude Code", Shared: true},
	)
}

func vscodeHomes(env Env) []string {
	var hs []string
	for _, v := range vscodeVariants {
		hs = append(hs, vscodeUserDir(env, v.dir))
	}
	cp, _ := copilotHome(env)
	return append(hs, cp)
}

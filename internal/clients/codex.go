// Codex adapter: candidate config locations.
// Sources: https://learn.chatgpt.com/docs/config-file/config-basic and .../config-advanced

package clients

import (
	"path/filepath"
	"strings"
)

// codexHome is $CODEX_HOME, else ~/.codex; label is "CODEX_HOME" when the variable set it.
func codexHome(env Env) (dir, label string) {
	if d := env.Getenv("CODEX_HOME"); d != "" {
		return d, "CODEX_HOME"
	}
	return filepath.Join(env.Home, ".codex"), ""
}

func codexCandidates(env Env) []Candidate {
	home, label := codexHome(env)
	var cs []Candidate
	if env.GOOS != "windows" { // the system file is not documented on Windows
		cs = append(cs, Candidate{Scope: ScopeManaged, Path: "/etc/codex/config.toml", Format: FormatTOML})
	}
	cs = append(cs, Candidate{Scope: ScopeUser, Path: filepath.Join(home, "config.toml"), Format: FormatTOML, Label: label})
	if env.Home != "" || label != "" {
		// Listing errors are reported by discoverOne's ReadDir wrapper; here they just mean no profiles.
		entries, _ := env.ReadDir(home)
		for _, e := range entries {
			if name := e.Name(); !e.IsDir() && strings.HasSuffix(name, ".config.toml") {
				cs = append(cs, Candidate{Scope: ScopeUser, Path: filepath.Join(home, name), Format: FormatTOML,
					Label: "profile " + strings.TrimSuffix(name, ".config.toml")})
			}
		}
	}
	if env.Cwd != "" {
		cs = append(cs, Candidate{Scope: ScopeProject, Path: filepath.Join(env.Cwd, ".codex", "config.toml"), Format: FormatTOML})
	}
	return cs
}

func codexHomes(env Env) []string {
	home, _ := codexHome(env)
	return []string{home}
}

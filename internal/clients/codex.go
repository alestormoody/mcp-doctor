// Codex adapter: candidate config locations.
// Sources: https://learn.chatgpt.com/docs/config-file/config-basic and .../config-advanced

package clients

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/alestormoody/mcp-doctor/internal/diag"
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
	// Codex only loads these for trusted projects; discovery lists them regardless (trust is checked in M02).
	for _, d := range codexProjectDirs(env) {
		cs = append(cs, Candidate{Scope: ScopeProject, Path: filepath.Join(d, ".codex", "config.toml"), Format: FormatTOML})
	}
	return cs
}

// codexProjectDirs is every dir from the nearest ancestor holding .git (file or dir) down to Cwd,
// or just Cwd when no ancestor has one. The walk stops at the filesystem root.
func codexProjectDirs(env Env) []string {
	if env.Cwd == "" {
		return nil
	}
	var up []string
	for d := env.Cwd; ; d = filepath.Dir(d) {
		up = append(up, d)
		if _, err := env.Stat(filepath.Join(d, ".git")); err == nil {
			slices.Reverse(up)
			return up
		}
		if filepath.Dir(d) == d {
			return []string{env.Cwd}
		}
	}
}

func codexHomes(env Env) []string {
	home, _ := codexHome(env)
	return []string{home}
}

// parseCodex maps the client's config files onto Servers. Placeholder: filled in by the client's parse plan.
func parseCodex(Env, []Loaded) ([]Server, []diag.Finding) { return nil, nil }

// codexRank is the client's scope precedence for markShadowed. Placeholder: filled in by the client's parse plan.
func codexRank(Source) (int, bool) { return 0, false }

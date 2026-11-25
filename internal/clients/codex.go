// Codex adapter: candidate config locations and config.toml parsing.
// Sources: https://learn.chatgpt.com/docs/config-file/config-basic and .../config-advanced

package clients

import (
	"bytes"
	"cmp"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alestormoody/mcp-doctor/internal/config"
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

// codexFields is the documented set of [mcp_servers.<name>] keys
// (https://learn.chatgpt.com/docs/config-file/config-reference.md).
var codexFields = []string{"command", "args", "env", "env_vars", "cwd", "url", "auth", "oauth", "bearer_token_env_var",
	"http_headers", "http_headers_helper", "env_http_headers", "enabled", "required", "startup_readiness",
	"startup_timeout_sec", "startup_timeout_ms", "tool_timeout_sec", "enabled_tools", "disabled_tools",
	"supports_parallel_tool_calls", "tool_input_schema_max_bytes", "omit_tools_from", "default_tools_approval_mode",
	"tools", "scopes", "oauth_resource", "environment_id"}

const codexUntrusted = "not trusted: Codex ignores this file"

// parseCodex parses every file once, then labels project files from the trust table of the user config.
func parseCodex(env Env, files []Loaded) ([]Server, []diag.Finding) {
	home, _ := codexHome(env)
	userPath := filepath.Join(home, "config.toml")
	var (
		servers []Server
		fds     []diag.Finding
		userDoc *config.TOMLDoc
	)
	for _, l := range files {
		ss, doc, ffds := codexFile(env, l.File, l.Src)
		if l.Path == userPath {
			userDoc = doc
		}
		servers = append(servers, ss...)
		fds = append(fds, ffds...)
	}
	for i, s := range servers {
		if s.Source.Scope == ScopeProject {
			servers[i].Source.Label = codexUntrusted
			if codexTrusted(env, userDoc, filepath.Dir(filepath.Dir(s.Source.Path))) {
				servers[i].Source.Label = "trusted project"
			}
		}
	}
	return servers, fds
}

// codexFile maps one config.toml; doc is nil when the file does not parse.
func codexFile(_ Env, f File, src []byte) ([]Server, *config.TOMLDoc, []diag.Finding) {
	const client = "codex"
	doc, ps := config.ParseTOML(src)
	var fds []diag.Finding
	for _, p := range ps {
		fds = append(fds, problemFinding(client, f, p))
	}
	if doc == nil {
		return nil, nil, fds
	}
	v, ok := doc.Values["mcp_servers"]
	if !ok {
		return nil, doc, fds
	}
	at := func(path ...string) string {
		l, c := doc.Pos(path...)
		return position(f.Path, l, c)
	}
	tbl, ok := v.(map[string]any)
	if !ok {
		return nil, doc, append(fds, parseFinding("MCP_PARSE_002", client, "", at("mcp_servers"),
			`Invalid structure: "mcp_servers" is not a table`))
	}
	names := make([]string, 0, len(tbl))
	for n := range tbl {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int {
		la, ca := doc.Pos("mcp_servers", a)
		lb, cb := doc.Pos("mcp_servers", b)
		return cmp.Or(cmp.Compare(la, lb), cmp.Compare(ca, cb), strings.Compare(a, b))
	})
	label := f.Label
	if strings.HasPrefix(label, "profile ") {
		label += " (only with --profile)"
	}
	var servers []Server
	for _, name := range names {
		line, col := doc.Pos("mcp_servers", name)
		s := Server{Name: name, Source: Source{Client: client, Path: f.Path, Scope: f.Scope, Format: f.Format,
			Label: label, Line: line, Col: col}}
		entry, ok := tbl[name].(map[string]any)
		if !ok {
			fds = append(fds, parseFinding("MCP_PARSE_002", client, name, s.Source.Position(),
				fmt.Sprintf("Invalid structure: server %q is not a table", name)))
			continue
		}
		bad := func(field, want string) {
			pos := at("mcp_servers", name, field)
			if l, _ := doc.Pos("mcp_servers", name, field); l == 0 {
				pos = s.Source.Position()
			}
			fds = append(fds, parseFinding("MCP_PARSE_002", client, name, pos,
				fmt.Sprintf("Invalid structure: %q must be %s", field, want)))
		}
		if !codexEntry(&s, entry, bad) {
			continue
		}
		t, inferred, code, msg := resolveTransport("", s.Command != "", s.URL.Reveal() != "", false, nil, "stdio", "http")
		if code != "" {
			fds = append(fds, parseFinding(code, client, name, s.Source.Position(), msg))
			continue
		}
		s.Transport, s.TransportInferred = t, inferred
		for k := range entry {
			if !slices.Contains(codexFields, k) {
				s.Unknown = append(s.Unknown, k)
			}
		}
		slices.Sort(s.Unknown)
		if len(s.Unknown) > 0 {
			fds = append(fds, unknownFinding(client, s, s.Unknown, ""))
		}
		if raw := doc.Bytes("mcp_servers", name); raw != nil {
			b := bytes.Clone(raw)
			s.raw = &b
		}
		servers = append(servers, s)
	}
	return servers, doc, fds
}

// codexEntry decodes the modelled fields of entry into s; on a wrong type it calls bad and returns false.
func codexEntry(s *Server, entry map[string]any, bad func(field, want string)) bool {
	const wantStr, wantStrs, wantMap = "a string", "an array of strings", "a table with string values"
	str := func(field string, dst *string) bool {
		v, ok := entry[field]
		if !ok {
			return true
		}
		if *dst, ok = v.(string); !ok {
			bad(field, wantStr)
		}
		return ok
	}
	strMap := func(field string) (map[string]string, bool) {
		v, ok := entry[field]
		if !ok {
			return nil, true
		}
		m, ok := v.(map[string]any)
		if !ok {
			bad(field, wantMap)
			return nil, false
		}
		out := make(map[string]string, len(m))
		for k, x := range m {
			if out[k], ok = x.(string); !ok {
				bad(field, wantMap)
				return nil, false
			}
		}
		return out, true
	}
	toSecrets := func(m map[string]string) map[string]diag.Secret {
		if m == nil {
			return nil
		}
		out := make(map[string]diag.Secret, len(m))
		for k, v := range m {
			out[k] = diag.NewSecret(v)
		}
		return out
	}
	var url, bearer string
	if !str("command", &s.Command) || !str("cwd", &s.Cwd) || !str("url", &url) || !str("bearer_token_env_var", &bearer) {
		return false
	}
	if v, ok := entry["args"]; ok {
		arr, ok := v.([]any)
		if !ok {
			bad("args", wantStrs)
			return false
		}
		for _, x := range arr {
			a, ok := x.(string)
			if !ok {
				bad("args", wantStrs)
				return false
			}
			s.Args = append(s.Args, diag.NewSecret(a))
		}
	}
	envMap, ok1 := strMap("env")
	headers, ok2 := strMap("http_headers")
	envHeaders, ok3 := strMap("env_http_headers")
	if !ok1 || !ok2 || !ok3 {
		return false
	}
	s.Env, s.Headers = toSecrets(envMap), toSecrets(headers)
	if url != "" {
		s.URL = diag.NewSecret(url)
	}
	if v, ok := entry["env_vars"]; ok {
		const want = "an array of names or {name, source} tables"
		arr, ok := v.([]any)
		if !ok {
			bad("env_vars", want)
			return false
		}
		for i, x := range arr {
			name, ok := x.(string)
			if t, isTable := x.(map[string]any); isTable {
				name, ok = t["name"].(string)
			}
			if !ok {
				bad("env_vars", want)
				return false
			}
			s.Refs = append(s.Refs, VarRef{Field: fmt.Sprintf("env_vars[%d]", i), Kind: "env", Name: name})
		}
	}
	if bearer != "" {
		s.Refs = append(s.Refs, VarRef{Field: "bearer_token_env_var", Kind: "env", Name: bearer})
	}
	for _, h := range slices.Sorted(maps.Keys(envHeaders)) {
		s.Refs = append(s.Refs, VarRef{Field: "env_http_headers." + h, Kind: "env", Name: envHeaders[h]})
	}
	if v, ok := entry["enabled"]; ok {
		b, ok := v.(bool)
		if !ok {
			bad("enabled", "true or false")
			return false
		}
		s.Disabled = !b
	}
	return true
}

// codexRank: closest project > farther project > user > system; profiles (opt-in) and untrusted projects
// (ignored by Codex) neither shadow nor are shadowed.
func codexRank(s Source) (int, bool) {
	switch {
	case strings.HasPrefix(s.Label, "profile ") || strings.Contains(s.Label, "not trusted"):
		return 0, false
	case s.Scope == ScopeProject:
		return 100 + strings.Count(filepath.ToSlash(filepath.Dir(filepath.Dir(s.Path))), "/"), true
	case s.Scope == ScopeUser:
		return 2, true
	case s.Scope == ScopeManaged:
		return 1, true
	}
	return 0, false
}

// codexTrusted reports whether the nearest ancestor-or-self of projectDir listed in the user config's
// [projects] table has trust_level = "trusted". It uses only the already-parsed user document.
func codexTrusted(env Env, userDoc *config.TOMLDoc, projectDir string) bool {
	if userDoc == nil || projectDir == "" {
		return false
	}
	projects, ok := userDoc.Values["projects"].(map[string]any)
	if !ok {
		return false
	}
	keys := slices.Sorted(maps.Keys(projects)) // deterministic when two keys name the same dir
	for d := projectDir; ; d = filepath.Dir(d) {
		for _, k := range keys {
			if samePath(env.GOOS, k, d) {
				p, _ := projects[k].(map[string]any)
				level, _ := p["trust_level"].(string)
				return level == "trusted"
			}
		}
		if filepath.Dir(d) == d {
			return false
		}
	}
}

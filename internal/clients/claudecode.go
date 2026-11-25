// Claude Code adapter: candidate config locations and parsing.
// Sources: https://code.claude.com/docs/en/mcp, https://code.claude.com/docs/en/managed-mcp.md,
// https://code.claude.com/docs/en/claude-directory

package clients

import (
	"maps"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/alestormoody/mcp-doctor/internal/config"
	"github.com/alestormoody/mcp-doctor/internal/diag"
)

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

const claudeCodeID = "claude-code"

// parseClaudeCode maps every Claude Code file onto Servers. The user file's projects[<cwd>].disabledMcpServers
// marks servers of that name Disabled in every file of this Load; the file is never read a second time.
func parseClaudeCode(env Env, files []Loaded) ([]Server, []diag.Finding) {
	var servers []Server
	var fds []diag.Finding
	disabled := map[string]bool{}
	for _, l := range files {
		ss, dis, f := claudeCodeFile(env, l.File, l.Src)
		servers, fds = append(servers, ss...), append(fds, f...)
		for _, n := range dis {
			disabled[n] = true
		}
	}
	for i := range servers {
		servers[i].Disabled = disabled[servers[i].Name]
	}
	return servers, fds
}

// claudeCodeFile parses one file: top-level mcpServers in f's scope and, for the user file only,
// projects[<cwd>] (local scope) and its disabled names. Other keys (history, ...) are never decoded.
func claudeCodeFile(env Env, f File, src []byte) (servers []Server, disabled []string, fds []diag.Finding) {
	doc, fds := openJSON(claudeCodeID, f, src)
	if doc == nil {
		return nil, nil, fds
	}
	root, isObject, rfds := objectFields(claudeCodeID, f, doc.Root())
	fds = append(fds, rfds...)
	if !isObject {
		r := doc.Root()
		return nil, nil, append(fds, parseFinding("MCP_PARSE_002", claudeCodeID, "", position(f.Path, r.Line, r.Col),
			"Invalid structure: the file is not a JSON object"))
	}
	servers, efds := claudeCodeServers(f, f.Scope, f.Label, root["mcpServers"])
	fds = append(fds, efds...)
	projects, ok := root["projects"]
	if f.Scope != ScopeUser || !ok || env.Cwd == "" {
		return servers, nil, fds
	}
	ms, isObject, ps := projects.Members()
	if !isObject {
		return servers, nil, append(fds, parseFinding("MCP_PARSE_002", claudeCodeID, "",
			position(f.Path, projects.Line, projects.Col), `Invalid structure: "projects" is not an object`))
	}
	for _, p := range ps { // only the cwd key matters: other projects are never reported
		if samePath(env.GOOS, p.Key, env.Cwd) {
			fds = append(fds, problemFinding(claudeCodeID, f, p))
		}
	}
	var proj config.Member
	for _, m := range ms {
		if samePath(env.GOOS, m.Name, env.Cwd) {
			proj = m // last match wins, as for a duplicate key
		}
	}
	if proj == (config.Member{}) {
		return servers, nil, fds
	}
	fields, isObject, pfds := objectFields(claudeCodeID, f, proj)
	fds = append(fds, pfds...)
	if !isObject {
		return servers, nil, append(fds, parseFinding("MCP_PARSE_002", claudeCodeID, "",
			position(f.Path, proj.Line, proj.Col), "Invalid structure: the project entry is not an object"))
	}
	local, lfds := claudeCodeServers(f, ScopeLocal, "project "+proj.Name, fields["mcpServers"])
	servers, fds = append(servers, local...), append(fds, lfds...)
	disabled, dfd := jsonEntry{Src: Source{Client: claudeCodeID, Path: f.Path}, Fields: fields}.strs("disabledMcpServers")
	if dfd != nil {
		fds = append(fds, *dfd)
	}
	return servers, disabled, fds
}

// claudeCodeServers maps one mcpServers object; entries with a type or transport error are dropped.
func claudeCodeServers(f File, scope Scope, label string, obj config.Member) ([]Server, []diag.Finding) {
	entries, fds := jsonEntries(claudeCodeID, f, scope, label, obj)
	var out []Server
	for _, e := range entries {
		var bad []diag.Finding
		str := func(field string) string {
			v, fd := e.str(field)
			if fd != nil {
				bad = append(bad, *fd)
			}
			return v
		}
		strMap := func(field string) map[string]string {
			v, fd := e.strMap(field)
			if fd != nil {
				bad = append(bad, *fd)
			}
			return v
		}
		typ, cmd, url := str("type"), str("command"), str("url")
		args, fd := e.strs("args")
		if fd != nil {
			bad = append(bad, *fd)
		}
		env, headers := strMap("env"), strMap("headers")
		if len(bad) > 0 {
			fds = append(fds, bad...)
			continue
		}
		t, inferred, code, msg := resolveTransport(typ, cmd != "", url != "", true,
			map[string]string{"streamable-http": "http"}, "stdio", "http", "sse", "ws", "sdk")
		if code != "" {
			fds = append(fds, parseFinding(code, claudeCodeID, e.Name, e.Src.Position(), msg))
			continue
		}
		s := Server{Name: e.Name, Source: e.Src, Transport: t, TransportInferred: inferred, Command: cmd, raw: &e.raw}
		s.Refs = dollarRefs("command", cmd)
		for i, a := range args {
			s.Args = append(s.Args, diag.NewSecret(a))
			s.Refs = append(s.Refs, dollarRefs("args["+strconv.Itoa(i)+"]", a)...)
		}
		s.Env, s.Refs = claudeCodeSecrets("env", env, s.Refs)
		if url != "" {
			s.URL = diag.NewSecret(url)
			s.Refs = append(s.Refs, dollarRefs("url", url)...)
		}
		s.Headers, s.Refs = claudeCodeSecrets("headers", headers, s.Refs)
		if s.Unknown = e.unknown("type", "command", "args", "env", "url", "headers", "headersHelper",
			"oauth", "timeout", "alwaysLoad"); s.Unknown != nil {
			fds = append(fds, unknownFinding(claudeCodeID, s, s.Unknown, ""))
		}
		out = append(out, s)
	}
	return out, fds
}

// claudeCodeSecrets wraps m's values and appends their refs in key order (nil m -> nil).
func claudeCodeSecrets(field string, m map[string]string, refs []VarRef) (map[string]diag.Secret, []VarRef) {
	if m == nil {
		return nil, refs
	}
	out := make(map[string]diag.Secret, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		out[k] = diag.NewSecret(m[k])
		refs = append(refs, dollarRefs(field+"."+k, m[k])...)
	}
	return out, refs
}

// claudeCodeRank is the documented precedence: managed > local > project > user.
func claudeCodeRank(s Source) (int, bool) {
	r, ok := map[Scope]int{ScopeManaged: 4, ScopeLocal: 3, ScopeProject: 2, ScopeUser: 1}[s.Scope]
	return r, ok
}

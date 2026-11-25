// Claude Desktop adapter: candidate config locations and mcpServers parsing.
// Source: https://modelcontextprotocol.io/docs/develop/connect-local-servers

package clients

import (
	"path/filepath"

	"github.com/alestormoody/mcp-doctor/internal/config"
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

// parseClaudeDesktop maps every Claude Desktop file onto Servers. Each Windows file keeps its own label and
// no file is assumed to be the live one; precedence is not documented, so nothing is shadowed.
func parseClaudeDesktop(_ Env, files []Loaded) ([]Server, []diag.Finding) {
	var servers []Server
	var fds []diag.Finding
	for _, l := range files {
		ss, fs := claudeDesktopServers(l.File, l.Src)
		servers, fds = append(servers, ss...), append(fds, fs...)
	}
	return servers, fds
}

// claudeDesktopServers parses one file. Documented fields: command, args, env. url and type are unknown
// (remote servers come from the Connectors UI) but still decide the inferred transport. ${...} stays literal.
func claudeDesktopServers(f File, src []byte) ([]Server, []diag.Finding) {
	const id = "claude-desktop"
	top, fds := cdTop(id, f, src)
	if top == nil {
		return nil, fds
	}
	entries, efds := jsonEntries(id, f, f.Scope, f.Label, top["mcpServers"])
	fds = append(fds, efds...)
	var servers []Server
	for _, e := range entries {
		cmd, fd1 := e.str("command")
		args, fd2 := e.strs("args")
		env, fd3 := e.strMap("env")
		if fd := cdFirst(fd1, fd2, fd3); fd != nil {
			fds = append(fds, *fd)
			continue
		}
		u, _ := e.str("url") // undocumented: a wrong type is not this client's structure error
		t, inferred, code, msg := resolveTransport("", cmd != "", u != "", false, nil, "stdio", "http")
		if code != "" {
			fds = append(fds, parseFinding(code, id, e.Name, e.Src.Position(), msg))
			continue
		}
		s := Server{Name: e.Name, Source: e.Src, Transport: t, TransportInferred: inferred, Command: cmd,
			Args: cdSecrets(args), Env: cdSecretMap(env), Unknown: e.unknown("command", "args", "env"), raw: &e.raw}
		if u != "" {
			s.URL = diag.NewSecret(u)
		}
		if len(s.Unknown) > 0 {
			fds = append(fds, unknownFinding(id, s, s.Unknown, ""))
		}
		servers = append(servers, s)
	}
	return servers, fds
}

// The helpers below are shared with cursor.go (same file shape: a plain JSON object with "mcpServers").

// cdTop opens a JSON-family file and returns its top-level members; nil when the file has a syntax error or
// its root is not an object (002). Unknown top-level keys are ignored: the file may hold other settings.
func cdTop(client string, f File, src []byte) (map[string]config.Member, []diag.Finding) {
	doc, fds := openJSON(client, f, src)
	if doc == nil {
		return nil, fds
	}
	root := doc.Root()
	top, isObject, ofds := objectFields(client, f, root)
	fds = append(fds, ofds...)
	if !isObject {
		return nil, append(fds, parseFinding("MCP_PARSE_002", client, "", position(f.Path, root.Line, root.Col),
			"Invalid structure: the file is not a JSON object"))
	}
	return top, fds
}

// cdFirst returns the first non-nil field finding (one structure error per entry is enough to drop it).
func cdFirst(fds ...*diag.Finding) *diag.Finding {
	for _, fd := range fds {
		if fd != nil {
			return fd
		}
	}
	return nil
}

func cdSecrets(ss []string) []diag.Secret {
	if ss == nil {
		return nil
	}
	out := make([]diag.Secret, len(ss))
	for i, s := range ss {
		out[i] = diag.NewSecret(s)
	}
	return out
}

func cdSecretMap(m map[string]string) map[string]diag.Secret {
	if m == nil {
		return nil
	}
	out := make(map[string]diag.Secret, len(m))
	for k, v := range m {
		out[k] = diag.NewSecret(v)
	}
	return out
}

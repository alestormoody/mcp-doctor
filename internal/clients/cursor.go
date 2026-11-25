// Cursor adapter: candidate config locations and mcpServers parsing.
// Source: https://cursor.com/docs/context/mcp

package clients

import (
	"maps"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/alestormoody/mcp-doctor/internal/diag"
)

func cursorCandidates(env Env) []Candidate {
	return []Candidate{
		{Scope: ScopeUser, Path: filepath.Join(env.Home, ".cursor", "mcp.json"), Format: FormatJSON},
		{Scope: ScopeProject, Path: filepath.Join(env.Cwd, ".cursor", "mcp.json"), Format: FormatJSON},
	}
}

func cursorHomes(env Env) []string { return []string{filepath.Join(env.Home, ".cursor")} }

// parseCursor maps every Cursor file onto Servers; precedence is not documented, so nothing is shadowed.
func parseCursor(_ Env, files []Loaded) ([]Server, []diag.Finding) {
	var servers []Server
	var fds []diag.Finding
	for _, l := range files {
		ss, fs := cursorFile(l.File, l.Src)
		servers, fds = append(servers, ss...), append(fds, fs...)
	}
	return servers, fds
}

// cursorAuth is the documented OAuth object; scopes are not secret and not needed, so they are not decoded.
type cursorAuth struct {
	ClientID     string `json:"CLIENT_ID"`
	ClientSecret string `json:"CLIENT_SECRET"`
}

// cursorFile parses one file. Documented fields: type, command, args, env, envFile (stdio) and url, headers,
// auth (remote). auth values are only scanned for variable references, never stored.
func cursorFile(f File, src []byte) ([]Server, []diag.Finding) {
	const id = "cursor"
	top, fds := cdTop(id, f, src)
	if top == nil {
		return nil, fds
	}
	entries, efds := jsonEntries(id, f, f.Scope, f.Label, top["mcpServers"])
	fds = append(fds, efds...)
	var servers []Server
	for _, e := range entries {
		typ, fd1 := e.str("type")
		cmd, fd2 := e.str("command")
		args, fd3 := e.strs("args")
		env, fd4 := e.strMap("env")
		envFile, fd5 := e.str("envFile")
		u, fd6 := e.str("url")
		headers, fd7 := e.strMap("headers")
		var auth cursorAuth
		_, fd8 := e.decode("auth", "an object with string CLIENT_ID and CLIENT_SECRET", &auth)
		if fd := cdFirst(fd1, fd2, fd3, fd4, fd5, fd6, fd7, fd8); fd != nil {
			fds = append(fds, *fd)
			continue
		}
		t, inferred, code, msg := resolveTransport(typ, cmd != "", u != "", false, nil, "stdio", "http", "sse")
		if code != "" {
			fds = append(fds, parseFinding(code, id, e.Name, e.Src.Position(), msg))
			continue
		}
		refs := colonRefs("command", cmd)
		for i, a := range args {
			refs = append(refs, colonRefs("args["+strconv.Itoa(i)+"]", a)...)
		}
		refs = append(refs, cursorMapRefs("env", env)...)
		refs = append(refs, colonRefs("url", u)...)
		refs = append(refs, cursorMapRefs("headers", headers)...)
		refs = append(refs, colonRefs("auth.CLIENT_ID", auth.ClientID)...)
		refs = append(refs, colonRefs("auth.CLIENT_SECRET", auth.ClientSecret)...)
		s := Server{Name: e.Name, Source: e.Src, Transport: t, TransportInferred: inferred, Command: cmd,
			Args: cdSecrets(args), Env: cdSecretMap(env), Headers: cdSecretMap(headers), EnvFile: envFile, Refs: refs,
			Unknown: e.unknown("type", "command", "args", "env", "envFile", "url", "headers", "auth"), raw: &e.raw}
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

// cursorMapRefs scans m's values in key order, as Field "<prefix>.<key>".
func cursorMapRefs(prefix string, m map[string]string) []VarRef {
	var refs []VarRef
	for _, k := range slices.Sorted(maps.Keys(m)) {
		refs = append(refs, colonRefs(prefix+"."+k, m[k])...)
	}
	return refs
}

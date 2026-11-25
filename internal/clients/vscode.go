// VS Code adapter: candidate config locations.
// Sources: https://code.visualstudio.com/docs/agents/reference/mcp-configuration,
// https://code.visualstudio.com/docs/configure/profiles

package clients

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/alestormoody/mcp-doctor/internal/diag"
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

// parseVSCode maps the client's config files onto Servers. Precedence is not documented: Shadowed is never set.
func parseVSCode(env Env, files []Loaded) ([]Server, []diag.Finding) {
	var servers []Server
	var fds []diag.Finding
	for _, l := range files {
		ss, ffds := vscodeFile(env, l.File, l.Src)
		servers = append(servers, ss...)
		fds = append(fds, ffds...)
	}
	return servers, fds
}

const vscodeUnsupported = "not supported in this file"

var (
	vscodeMCPFields      = []string{"type", "command", "args", "cwd", "env", "envFile", "dev", "sandboxEnabled", "sandbox", "url", "headers", "oauth"}
	vscodePortableFields = []string{"type", "command", "args", "env", "cwd", "url", "headers", "tools", "oauthClientId"}
	// The Copilot CLI writes "type": "local" for stdio servers in mcp-config.json:
	// https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers
	vscodePortableAliases = map[string]string{"local": "stdio"}
)

// vscodeFile parses one file. mcp.json (user, profile, Insiders, .vscode) uses "servers" and ${env:}/${input:}
// variables in every string; the portable files (~/.copilot/mcp-config.json, .mcp.json) use "mcpServers" and ${NAME}.
func vscodeFile(_ Env, f File, src []byte) ([]Server, []diag.Finding) {
	const client = "vscode"
	doc, fds := openJSON(client, f, src)
	if doc == nil {
		return nil, fds
	}
	root := doc.Root()
	top, isObject, tfds := objectFields(client, f, root)
	fds = append(fds, tfds...)
	if !isObject {
		return nil, append(fds, parseFinding("MCP_PARSE_002", client, "", position(f.Path, root.Line, root.Col),
			"Invalid structure: the file is not a JSON object"))
	}
	base := filepath.Base(f.Path)
	portable := base == "mcp-config.json" || base == ".mcp.json"

	key, known, aliases := "servers", vscodeMCPFields, map[string]string(nil)
	var unsupported []string
	inputs, checkInputs := map[string]bool{}, true
	if portable {
		key, known, aliases = "mcpServers", vscodePortableFields, vscodePortableAliases
		unsupported = []string{"envFile", "dev", "sandboxEnabled"}
		if base == ".mcp.json" {
			known = slices.DeleteFunc(slices.Clone(known), func(s string) bool { return s == "cwd" })
			unsupported = append(unsupported, "cwd")
		}
		if m, ok := top["inputs"]; ok {
			fds = append(fds, parseFinding("MCP_PARSE_005", client, "", position(f.Path, m.Line, m.Col),
				`Field "inputs" is `+vscodeUnsupported))
		}
	} else if m, ok := top["inputs"]; ok {
		var list []struct {
			ID string `json:"id"`
		}
		if p := m.Decode(&list); p != nil {
			checkInputs = false // no reliable id list: reporting every ${input:} as undefined would be noise
			fds = append(fds, parseFinding("MCP_PARSE_002", client, "", position(f.Path, p.Line, p.Col),
				`Invalid structure: "inputs" must be an array of objects with a string "id"`))
		}
		for _, in := range list {
			inputs[in.ID] = true
		}
	}

	entries, efds := jsonEntries(client, f, f.Scope, f.Label, top[key])
	fds = append(fds, efds...)
	var servers []Server
	for _, e := range entries {
		s, ok, sfds := vscodeServer(e, known, aliases, portable)
		fds = append(fds, sfds...)
		if !ok {
			continue
		}
		var unknown, unsup []string
		for _, n := range s.Unknown {
			if slices.Contains(unsupported, n) {
				unsup = append(unsup, n)
			} else {
				unknown = append(unknown, n)
			}
		}
		if len(unsup) > 0 {
			fds = append(fds, unknownFinding(client, s, unsup, vscodeUnsupported))
		}
		if len(unknown) > 0 {
			fds = append(fds, unknownFinding(client, s, unknown, ""))
		}
		for _, r := range s.Refs {
			if checkInputs && r.Kind == "input" && !inputs[r.Name] {
				fds = append(fds, parseFinding("MCP_PARSE_009", client, s.Name, s.Source.Position(),
					fmt.Sprintf("Undefined input reference %q", r.Name)))
			}
		}
		servers = append(servers, s)
	}
	return servers, fds
}

// vscodeServer decodes one entry; ok is false when a 002/003/008 drops it.
func vscodeServer(e jsonEntry, known []string, aliases map[string]string, portable bool) (Server, bool, []diag.Finding) {
	var fds []diag.Finding
	str := func(field string) string {
		if !slices.Contains(known, field) {
			return ""
		}
		v, fd := e.str(field)
		if fd != nil {
			fds = append(fds, *fd)
		}
		return v
	}
	strMap := func(field string) map[string]string {
		v, fd := e.strMap(field)
		if fd != nil {
			fds = append(fds, *fd)
		}
		return v
	}
	typ, cmd, url, cwd := str("type"), str("command"), str("url"), str("cwd")
	envFile := str("envFile")
	args, fd := e.strs("args")
	if fd != nil {
		fds = append(fds, *fd)
	}
	env, headers := strMap("env"), strMap("headers")
	if fds != nil {
		return Server{}, false, fds
	}
	t, inferred, code, msg := resolveTransport(typ, cmd != "", url != "", false, aliases, "stdio", "http", "sse")
	if code != "" {
		return Server{}, false, []diag.Finding{parseFinding(code, e.Src.Client, e.Name, e.Src.Position(), msg)}
	}
	s := Server{Name: e.Name, Source: e.Src, Transport: t, TransportInferred: inferred, Command: cmd,
		URL: diag.NewSecret(url), Cwd: cwd, EnvFile: envFile, Unknown: e.unknown(known...), raw: &e.raw}
	for _, a := range args {
		s.Args = append(s.Args, diag.NewSecret(a))
	}
	s.Env, s.Headers = vscodeSecrets(env), vscodeSecrets(headers)
	if !portable {
		for _, n := range slices.Sorted(maps.Keys(e.Fields)) {
			var v any
			_ = e.Fields[n].Decode(&v) // cannot fail: any accepts every valid JSON value
			vscodeStringLeaves(v, n, func(path, v string) { s.Refs = append(s.Refs, colonRefs(path, v)...) })
		}
		return s, true, nil
	}
	s.Refs = dollarRefs("command", cmd)
	for i, a := range args {
		s.Refs = append(s.Refs, dollarRefs("args["+strconv.Itoa(i)+"]", a)...)
	}
	for _, k := range slices.Sorted(maps.Keys(env)) {
		s.Refs = append(s.Refs, dollarRefs("env."+k, env[k])...)
	}
	s.Refs = append(s.Refs, dollarRefs("cwd", cwd)...)
	s.Refs = append(s.Refs, dollarRefs("url", url)...)
	for _, k := range slices.Sorted(maps.Keys(headers)) {
		s.Refs = append(s.Refs, dollarRefs("headers."+k, headers[k])...)
	}
	return s, true, nil
}

func vscodeSecrets(m map[string]string) map[string]diag.Secret {
	if m == nil {
		return nil
	}
	out := make(map[string]diag.Secret, len(m))
	for k, v := range m {
		out[k] = diag.NewSecret(v)
	}
	return out
}

// vscodeStringLeaves calls fn(path, s) for every string leaf of a decoded JSON value, object keys sorted;
// path is "a.b", "a[0]", "env.K".
func vscodeStringLeaves(v any, prefix string, fn func(path, s string)) {
	switch x := v.(type) {
	case string:
		fn(prefix, x)
	case []any:
		for i, el := range x {
			vscodeStringLeaves(el, prefix+"["+strconv.Itoa(i)+"]", fn)
		}
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(x)) {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			vscodeStringLeaves(x[k], p, fn)
		}
	}
}

// Client-independent parsing core: the normalized Server model, Load, scope precedence, URL redaction
// and the shared helpers every client's parse function uses.

package clients

import (
	"bytes"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/alestormoody/mcp-doctor/internal/config"
	"github.com/alestormoody/mcp-doctor/internal/diag"
)

// MaxFileSize is the largest config file Load parses (~/.claude.json can be tens of MB).
const MaxFileSize = 64 << 20

// Loaded is a found file and the bytes Load read from it. Clients that need data from another of their own
// files (Claude Code disabledMcpServers, Codex trust_level) take it from these bytes: no file is ever read
// twice for one client.
type Loaded struct {
	File
	Src []byte
}

// Source is where a server is defined.
type Source struct {
	Client string // client ID
	Path   string
	Scope  Scope
	Format Format
	Label  string
	Line   int // 1-based; 0 when unknown
	Col    int // 1-based byte column; 0 when unknown
}

// Position is "path:line:col", or the path alone when the line is unknown.
func (s Source) Position() string {
	if s.Line == 0 {
		return s.Path
	}
	return s.Path + ":" + strconv.Itoa(s.Line) + ":" + strconv.Itoa(s.Col)
}

// VarRef is a variable reference found in a string field, recorded raw and never expanded.
// Default values are not stored: they may be secrets.
type VarRef struct {
	Field      string // "command", "args[0]", "env.KEY", "headers.KEY", "url", "cwd", "auth.CLIENT_SECRET"
	Kind       string // env | input | workspace | userHome | other
	Name       string
	HasDefault bool
}

// Server is one MCP server entry, normalized across clients.
type Server struct {
	Name              string
	Source            Source
	Transport         string // stdio | http | sse | ws | sdk | ""
	TransportInferred bool
	Command           string
	Args              []diag.Secret
	Env               map[string]diag.Secret
	Headers           map[string]diag.Secret
	URL               diag.Secret
	Cwd, EnvFile      string
	Disabled          bool
	Refs              []VarRef
	Unknown           []string // sorted field names outside the client's documented set
	Shadowed          bool
	// raw is the entry's original bytes (JSON fragment / TOML section), never printed, kept for edits.
	// A pointer on purpose: fmt prints a nested pointer as an address, while it never calls methods on
	// unexported fields, so a Formatter on a byte type would not redact it.
	raw *[]byte
}

// Load runs Discover, reads every found file once per client that lists it, and hands each client all its
// read files at once. It never fails: problems are findings. Servers come in client order, then file order.
func Load(env Env) ([]Result, []Server, []diag.Finding) {
	results, findings := Discover(env)
	var servers []Server
	for _, r := range results {
		var loaded []Loaded
		for _, f := range r.Files {
			if f.Status != StatusFound {
				continue
			}
			src, err := env.ReadFile(f.Path)
			switch {
			case err != nil:
				findings = append(findings, discFinding("MCP_DISC_001", r.ID, f.Path,
					"Config file cannot be read: "+reason(err), permResolution))
			case len(src) > MaxFileSize:
				findings = append(findings, parseFinding("MCP_PARSE_007", r.ID, "", f.Path,
					"Config file is larger than 64 MiB and was not parsed"))
			default:
				loaded = append(loaded, Loaded{File: f, Src: src})
			}
		}
		if r.parse == nil || len(loaded) == 0 {
			continue
		}
		ss, fds := r.parse(env, loaded)
		if r.rank != nil {
			markShadowed(ss, r.rank)
		}
		servers = append(servers, ss...)
		findings = append(findings, fds...)
	}
	return results, servers, findings
}

// markShadowed sets Shadowed on every entry whose name a higher-ranked entry of the same client also defines.
// Entries with ok=false neither shadow nor are shadowed; tied top ranks shadow each other not.
func markShadowed(servers []Server, rank func(Source) (int, bool)) {
	top := map[string]int{}
	for _, s := range servers {
		if r, ok := rank(s.Source); ok {
			if t, seen := top[s.Name]; !seen || r > t {
				top[s.Name] = r
			}
		}
	}
	for i := range servers {
		if r, ok := rank(servers[i].Source); ok && r < top[servers[i].Name] {
			servers[i].Shadowed = true
		}
	}
}

// RedactURL shows only scheme://host[:port]; "/…" marks any path, query, fragment or userinfo (all may hold tokens).
func RedactURL(u string) string {
	p, err := url.Parse(u)
	if err != nil || p.Scheme == "" || p.Host == "" {
		return "(URL not shown)"
	}
	out := p.Scheme + "://" + p.Host
	if p.User != nil || (p.Path != "" && p.Path != "/") || p.RawQuery != "" || p.Fragment != "" || p.Opaque != "" {
		out += "/…"
	}
	return out
}

var parseResolutions = map[string]string{
	"MCP_PARSE_001": "Fix the syntax at the reported position; the client cannot read this file.",
	"MCP_PARSE_002": "Fix the structure at the reported position; see the client's documentation for the expected shape.",
	"MCP_PARSE_003": "Give the server a command (local server) or a url (remote server).",
	"MCP_PARSE_004": "Remove the duplicate key; keep the definition you want.",
	"MCP_PARSE_005": "Check the field name for a typo, or remove it; the client ignores fields it does not know.",
	"MCP_PARSE_006": "Remove comments and trailing commas; the client reads this file as plain JSON.",
	"MCP_PARSE_007": "The file is unusually large; check it is the right file. mcp-doctor skips files over 64 MiB.",
	"MCP_PARSE_008": "Make the transport type match the fields: a command for stdio, a url for http, sse or ws.",
	"MCP_PARSE_009": "Define the input in the file's \"inputs\" list, or fix the reference.",
	"MCP_PARSE_010": "Re-save the file as UTF-8 without a byte order mark.",
}

// parseFinding builds a parse finding with the code's default severity. msg must never carry a value from the file.
func parseFinding(code, client, server, source, msg string) diag.Finding {
	info, ok := diag.Lookup(code)
	if !ok {
		panic(fmt.Sprintf("parseFinding: unregistered code %q", code)) // programming error, caught by tests
	}
	return diag.Finding{Code: code, Category: info.Category, Severity: info.DefaultSeverity,
		Client: client, Server: server, Source: source, Message: msg, Resolution: parseResolutions[code]}
}

// position is "path:line:col", or the path alone when line is 0.
func position(path string, line, col int) string {
	return Source{Path: path, Line: line, Col: col}.Position()
}

// problemFinding maps a format-level problem of file f to its parse finding.
func problemFinding(client string, f File, p config.Problem) diag.Finding {
	code, msg := "MCP_PARSE_001", "Syntax error: "+p.Message
	switch p.Kind {
	case config.KindSyntax:
	case config.KindType:
		code, msg = "MCP_PARSE_002", "Invalid structure: "+p.Message
	case config.KindDuplicate:
		code, msg = "MCP_PARSE_004", fmt.Sprintf("Duplicate key %q (first at line %d:%d); the last one is used", p.Key, p.PrevLine, p.PrevCol)
	case config.KindNonStandard:
		code, msg = "MCP_PARSE_006", "Comments or trailing commas in a file the client reads as plain JSON"
	case config.KindBOM:
		code, msg = "MCP_PARSE_010", "File starts with a byte order mark"
	}
	return parseFinding(code, client, "", position(f.Path, p.Line, p.Col), msg)
}

// openJSON parses a JSON-family file; Format json is strict. doc is nil when the file has a syntax error.
func openJSON(client string, f File, src []byte) (*config.JSONDoc, []diag.Finding) {
	doc, ps := config.ParseJSON(src, f.Format == FormatJSON)
	fds := make([]diag.Finding, 0, len(ps))
	for _, p := range ps {
		fds = append(fds, problemFinding(client, f, p))
	}
	return doc, fds
}

// objectFields returns the members of m by name (last occurrence wins; repeats reported as 004).
// isObject is false, with no findings, when m is not an object.
func objectFields(client string, f File, m config.Member) (fields map[string]config.Member, isObject bool, fds []diag.Finding) {
	ms, isObject, ps := m.Members()
	if !isObject {
		return nil, false, nil
	}
	fields = make(map[string]config.Member, len(ms))
	for _, x := range ms {
		fields[x.Name] = x
	}
	for _, p := range ps {
		fds = append(fds, problemFinding(client, f, p))
	}
	return fields, true, fds
}

// jsonEntry is one server entry of a JSON servers object.
type jsonEntry struct {
	Name   string
	Src    Source                   // position of the server's name key
	Fields map[string]config.Member // last occurrence wins (duplicates already reported)
	raw    []byte                   // original bytes of the entry object; Server.raw = &e.raw
}

// jsonEntries lists the entries of the servers object obj in document order. A zero obj (key absent) yields
// nothing; a non-object obj or entry is a 002 and is dropped; duplicate keys at both levels are 004.
func jsonEntries(client string, f File, scope Scope, label string, obj config.Member) ([]jsonEntry, []diag.Finding) {
	if obj == (config.Member{}) {
		return nil, nil
	}
	ms, isObject, ps := obj.Members()
	if !isObject {
		return nil, []diag.Finding{parseFinding("MCP_PARSE_002", client, "", position(f.Path, obj.Line, obj.Col),
			fmt.Sprintf("Invalid structure: %q is not an object", obj.Name))}
	}
	var fds []diag.Finding
	for _, p := range ps {
		fds = append(fds, problemFinding(client, f, p))
	}
	entries := make([]jsonEntry, 0, len(ms))
	for _, m := range ms {
		src := Source{Client: client, Path: f.Path, Scope: scope, Format: f.Format, Label: label, Line: m.Line, Col: m.Col}
		fields, isObject, efds := objectFields(client, f, m)
		fds = append(fds, efds...)
		if !isObject {
			fd := parseFinding("MCP_PARSE_002", client, m.Name, src.Position(),
				fmt.Sprintf("Invalid structure: server %q is not an object", m.Name))
			fds = append(fds, fd)
			continue
		}
		entries = append(entries, jsonEntry{Name: m.Name, Src: src, Fields: fields, raw: bytes.Clone(m.Bytes())})
	}
	return entries, fds
}

// decode decodes field into v; absent -> false, nil; wrong type or a null anywhere in the value (encoding/json
// silently accepts null as a zero value) -> 002 at the value position, saying what was expected.
func (e jsonEntry) decode(field, want string, v any) (bool, *diag.Finding) {
	m, ok := e.Fields[field]
	if !ok {
		return false, nil
	}
	bad := func(line, col int) (bool, *diag.Finding) {
		fd := parseFinding("MCP_PARSE_002", e.Src.Client, e.Name, position(e.Src.Path, line, col),
			fmt.Sprintf("Invalid structure: %q must be %s", field, want))
		return false, &fd
	}
	if p := m.Decode(v); p != nil {
		return bad(p.Line, p.Col)
	}
	var generic any
	if m.Decode(&generic) == nil && hasNull(generic) {
		return bad(m.Line, m.Col)
	}
	return true, nil
}

// hasNull reports whether a value decoded into any contains a JSON null at any depth.
func hasNull(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case []any:
		return slices.ContainsFunc(x, hasNull)
	case map[string]any:
		for _, y := range x {
			if hasNull(y) {
				return true
			}
		}
	}
	return false
}

func (e jsonEntry) str(field string) (string, *diag.Finding) {
	var s string
	_, fd := e.decode(field, "a string", &s)
	return s, fd
}

func (e jsonEntry) strs(field string) ([]string, *diag.Finding) {
	var s []string
	if _, fd := e.decode(field, "an array of strings", &s); fd != nil {
		return nil, fd
	}
	return s, nil
}

func (e jsonEntry) strMap(field string) (map[string]string, *diag.Finding) {
	var m map[string]string
	if _, fd := e.decode(field, "an object with string values", &m); fd != nil {
		return nil, fd
	}
	return m, nil
}

// unknown returns the sorted names of Fields not in known.
func (e jsonEntry) unknown(known ...string) []string {
	var out []string
	for n := range e.Fields {
		if !slices.Contains(known, n) {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// unknownFinding is the one 005 per server listing its unknown field names.
func unknownFinding(client string, s Server, names []string, note string) diag.Finding {
	msg := "Unknown field(s): " + strings.Join(names, ", ")
	if note != "" {
		msg += " (" + note + ")"
	}
	return parseFinding("MCP_PARSE_005", client, s.Name, s.Source.Position(), msg)
}

// resolveTransport decides the transport from the stated type (as written, "" when absent) and the fields.
// code is "" when the entry is consistent; the written type is never echoed in msg.
func resolveTransport(stated string, hasCmd, hasURL, urlNeedsType bool, aliases map[string]string, allowed ...string) (t string, inferred bool, code, msg string) {
	if stated == "" {
		switch {
		case hasCmd:
			return "stdio", true, "", ""
		case hasURL && urlNeedsType:
			return "", false, "MCP_PARSE_008", "server has a url but no transport type"
		case hasURL:
			return "http", true, "", ""
		}
		return "", false, "MCP_PARSE_003", "Server has neither a command nor a url"
	}
	t = stated
	if a, ok := aliases[stated]; ok {
		t = a
	}
	if !slices.Contains(allowed, t) {
		return "", false, "MCP_PARSE_008", "transport type is not one of: " + strings.Join(allowed, ", ")
	}
	switch t {
	case "stdio":
		if !hasCmd {
			if hasURL {
				return t, false, "MCP_PARSE_008", "stdio server has a url but no command"
			}
			return t, false, "MCP_PARSE_003", "Server has neither a command nor a url"
		}
	case "http", "sse", "ws":
		if !hasURL {
			return t, false, "MCP_PARSE_008", t + " server has no url"
		}
	}
	return t, false, "", ""
}

// samePath compares a project path key from a config file with a real path. Pure string logic.
func samePath(goos, a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if goos == "windows" {
		norm := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, "\\", "/"), "/") }
		return strings.EqualFold(norm(a), norm(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

var (
	dollarRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-[^}]*)?\}`)
	colonRef  = regexp.MustCompile(`\$\{([^{}]+)\}`)
)

// dollarRefs finds ${NAME} and ${NAME:-default} (Kind env); a bare $NAME is literal.
func dollarRefs(field, s string) []VarRef {
	var out []VarRef
	for _, m := range dollarRef.FindAllStringSubmatch(s, -1) {
		out = append(out, VarRef{Field: field, Kind: "env", Name: m[1], HasDefault: m[2] != ""})
	}
	return out
}

// colonRefs finds VS Code / Cursor style ${env:N}, ${input:id}, ${workspaceFolder}, ${userHome} and any other ${...}.
func colonRefs(field, s string) []VarRef {
	var out []VarRef
	for _, m := range colonRef.FindAllStringSubmatch(s, -1) {
		r := VarRef{Field: field, Kind: "other", Name: m[1]}
		switch kind, name, _ := strings.Cut(m[1], ":"); {
		case kind == "env" && name != "":
			r.Kind, r.Name = "env", name
		case kind == "input" && name != "":
			r.Kind, r.Name = "input", name
		case m[1] == "workspaceFolder" || m[1] == "workspaceFolderBasename":
			r.Kind = "workspace"
		case m[1] == "userHome":
			r.Kind = "userHome"
		}
		out = append(out, r)
	}
	return out
}

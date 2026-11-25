// Client-independent parsing core: the normalized Server model, Load, scope precedence, URL redaction
// and the shared helpers every client's parse function uses.

package clients

import (
	"fmt"
	"net/url"
	"strconv"

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
	raw *[]byte //nolint:unused // filled by jsonEntries in the next commit
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

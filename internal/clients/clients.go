// Package clients finds the MCP configuration files of supported AI clients.
// It only stats and opens-then-closes files; it never reads, writes or lists beyond fixed locations.
package clients

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"

	"github.com/alestormoody/mcp-doctor/internal/diag"
)

// Scope is where a config file applies.
type Scope string

// The three scopes a candidate may have.
const (
	ScopeManaged Scope = "managed"
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// Format is the file syntax of a config file.
type Format string

// The config file formats clients use.
const (
	FormatJSON  Format = "json"
	FormatJSONC Format = "jsonc"
	FormatTOML  Format = "toml"
)

// Status is what discovery observed at a candidate path.
type Status string

// The three outcomes of classifying a candidate.
const (
	StatusFound      Status = "found"
	StatusMissing    Status = "missing"
	StatusUnreadable Status = "unreadable"
)

// Env is everything discovery may observe. The CLI builds the real one; tests inject fakes.
type Env struct {
	GOOS, Home, Cwd string // Home/Cwd may be "" when unresolvable
	Getenv          func(string) string
	Stat            func(string) (fs.FileInfo, error) // follows symlinks
	Lstat           func(string) (fs.FileInfo, error) // only used after Stat says not-exist (broken symlink)
	ReadDir         func(string) ([]fs.DirEntry, error)
	Open            func(string) (io.Closer, error) // opened and closed at once; never read
}

// Candidate is one location a client may read its MCP config from.
type Candidate struct {
	Scope  Scope
	Path   string
	Format Format
	Label  string // optional short qualifier, e.g. "Insiders", "MSIX", "CODEX_HOME"
	Shared bool   // read by more than one client: listed, but never by itself makes a client Detected
}

// File is a Candidate plus what discovery observed there.
type File struct {
	Candidate
	Status Status
	Reason string // short reason for unreadable; never file contents
}

// Client is one supported AI client.
type Client struct {
	ID, Name   string
	candidates func(Env) []Candidate // ordered: managed, user, project
	homes      func(Env) []string    // config home dirs whose existence means "detected"
}

// Result is the discovery outcome for one client.
type Result struct {
	Client
	Detected bool
	Files    []File
}

// all is the client table, in display order.
var all = []Client{
	{ID: "claude-code", Name: "Claude Code", candidates: claudeCodeCandidates, homes: claudeCodeHomes},
	{ID: "codex", Name: "Codex", candidates: codexCandidates, homes: codexHomes},
	{ID: "claude-desktop", Name: "Claude Desktop", candidates: claudeDesktopCandidates, homes: claudeDesktopHomes},
	{ID: "cursor", Name: "Cursor", candidates: cursorCandidates, homes: cursorHomes},
	{ID: "vscode", Name: "VS Code", candidates: vscodeCandidates, homes: vscodeHomes},
}

// userConfigDir mirrors os.UserConfigDir for env.GOOS: https://pkg.go.dev/os#UserConfigDir
func userConfigDir(env Env) string {
	switch env.GOOS {
	case "darwin":
		return filepath.Join(env.Home, "Library", "Application Support")
	case "windows":
		if d := env.Getenv("APPDATA"); d != "" {
			return d
		}
		return filepath.Join(env.Home, "AppData", "Roaming")
	default:
		if d := env.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(d) {
			return d
		}
		return filepath.Join(env.Home, ".config")
	}
}

// localAppData is %LOCALAPPDATA% on windows; other OSes have no equivalent.
func localAppData(env Env) string {
	if env.GOOS != "windows" {
		return ""
	}
	if d := env.Getenv("LOCALAPPDATA"); d != "" {
		return d
	}
	return filepath.Join(env.Home, "AppData", "Local")
}

// classify stats c.Path and, for a regular file, opens and closes it to prove read access. It never reads bytes.
func classify(env Env, c Candidate) File {
	unreadable := func(r string) File { return File{Candidate: c, Status: StatusUnreadable, Reason: r} }
	info, err := env.Stat(c.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if _, lerr := env.Lstat(c.Path); lerr == nil {
			return unreadable("broken symlink")
		}
		return File{Candidate: c, Status: StatusMissing}
	case err != nil:
		return unreadable(reason(err))
	case !info.Mode().IsRegular():
		return unreadable("not a regular file")
	}
	f, err := env.Open(c.Path)
	if err != nil {
		return unreadable(reason(err))
	}
	_ = f.Close()
	return File{Candidate: c, Status: StatusFound}
}

// reason is the short cause of err, without the path the user already sees.
func reason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

func discFinding(code, client, source, msg, resolution string) diag.Finding {
	return diag.Finding{Code: code, Category: "discovery", Severity: diag.SeverityWarning,
		Client: client, Source: source, Message: msg, Resolution: resolution}
}

const permResolution = "Check the file's permissions and ownership; mcp-doctor needs read access only."

// discoverOne classifies every candidate of c. Missing files are data, unreadable ones become MCP_DISC_001.
func discoverOne(env Env, c Client) (Result, []diag.Finding) {
	var findings []diag.Finding
	inner := env.ReadDir
	// Adapters ignore ReadDir errors: this wrapper reports them, and never lists a relative dir
	// (an empty Home must not make discovery list the process's current directory).
	env.ReadDir = func(dir string) ([]fs.DirEntry, error) {
		if !filepath.IsAbs(dir) {
			return nil, fs.ErrNotExist
		}
		ents, err := inner(dir)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			findings = append(findings, discFinding("MCP_DISC_001", c.ID, dir,
				"Config directory cannot be listed: "+reason(err), permResolution))
		}
		return ents, err
	}

	res := Result{Client: c}
	seen := map[string]bool{}
	for _, cand := range c.candidates(env) {
		key := filepath.Clean(cand.Path)
		if !filepath.IsAbs(cand.Path) || seen[key] {
			continue
		}
		seen[key] = true
		f := classify(env, cand)
		res.Files = append(res.Files, f)
		if f.Status == StatusUnreadable {
			findings = append(findings, discFinding("MCP_DISC_001", c.ID, f.Path,
				"Config file cannot be read: "+f.Reason, permResolution))
		}
		if f.Status != StatusMissing && !f.Shared {
			res.Detected = true
		}
	}
	for _, h := range c.homes(env) {
		if !filepath.IsAbs(h) {
			continue
		}
		if info, err := env.Stat(h); err == nil && info.IsDir() {
			res.Detected = true
		}
	}
	return res, findings
}

// Discover runs every client in table order. It never fails: problems are findings.
func Discover(env Env) ([]Result, []diag.Finding) {
	var findings []diag.Finding
	if env.Home == "" {
		findings = append(findings, discFinding("MCP_DISC_002", "", "",
			"Home directory cannot be resolved; user config locations were skipped",
			"Set HOME (USERPROFILE on Windows) and run again."))
	}
	if env.Cwd == "" {
		findings = append(findings, discFinding("MCP_DISC_002", "", "",
			"Current directory cannot be resolved; project config locations were skipped",
			"Run mcp-doctor from an existing directory."))
	}
	results := make([]Result, 0, len(all))
	for _, c := range all {
		r, fds := discoverOne(env, c)
		results = append(results, r)
		findings = append(findings, fds...)
	}
	return results, findings
}

// Package clients finds the MCP configuration files of supported AI clients.
// It only stats and opens-then-closes files; it never reads, writes or lists beyond fixed locations.
package clients

import (
	"io"
	"io/fs"
	"path/filepath"
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

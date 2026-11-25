package clients

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// --- shared test helpers (used by every *_test.go in this package) ---

// mapEnv returns a Getenv backed by m; a nil map yields "" for every key.
func mapEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

type fakeInfo struct {
	name  string
	isDir bool
}

func (f fakeInfo) Name() string { return f.name }
func (f fakeInfo) Size() int64  { return 0 }
func (f fakeInfo) Mode() fs.FileMode {
	if f.isDir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.isDir }
func (f fakeInfo) Sys() any           { return nil }

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// fakeEnv is an in-memory Env with Home "/h" and Cwd "/w/proj". paths are slash-style; a trailing "/"
// marks a directory and parent directories are implied. Lookups use filepath.ToSlash(filepath.Clean(p)).
func fakeEnv(goos string, vars map[string]string, paths ...string) Env {
	nodes := map[string]bool{} // slash path -> isDir
	for _, p := range paths {
		isDir := strings.HasSuffix(p, "/")
		p = path.Clean(p)
		nodes[p] = nodes[p] || isDir
		for d := path.Dir(p); d != p; p, d = d, path.Dir(d) {
			nodes[d] = true
		}
	}
	key := func(p string) string { return filepath.ToSlash(filepath.Clean(p)) }
	stat := func(p string) (fs.FileInfo, error) {
		k := key(p)
		isDir, ok := nodes[k]
		if !ok {
			return nil, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
		}
		return fakeInfo{name: path.Base(k), isDir: isDir}, nil
	}
	return Env{
		GOOS:   goos,
		Home:   "/h",
		Cwd:    "/w/proj",
		Getenv: mapEnv(vars),
		Stat:   stat,
		Lstat:  stat,
		ReadDir: func(p string) ([]fs.DirEntry, error) {
			k := key(p)
			isDir, ok := nodes[k]
			if !ok {
				return nil, &fs.PathError{Op: "readdir", Path: p, Err: fs.ErrNotExist}
			}
			if !isDir {
				return nil, &fs.PathError{Op: "readdir", Path: p, Err: errors.New("not a directory")}
			}
			var out []fs.DirEntry
			for n, d := range nodes {
				if n != k && path.Dir(n) == k {
					out = append(out, fs.FileInfoToDirEntry(fakeInfo{name: path.Base(n), isDir: d}))
				}
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
			return out, nil
		},
		Open: func(p string) (io.Closer, error) {
			if _, err := stat(p); err != nil {
				return nil, err
			}
			return nopCloser{}, nil
		},
	}
}

// fixtureEnv is a real-filesystem Env rooted under a directory whose name has a space and Unicode.
// It creates root/home (Home) and root/work/proj (Cwd). Tests set env.Getenv to add variables.
func fixtureEnv(t *testing.T) (env Env, root string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "Prova dir", "ユーザー Ünï")
	home, cwd := filepath.Join(root, "home"), filepath.Join(root, "work", "proj")
	for _, d := range []string{home, cwd} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	return Env{
		GOOS:    runtime.GOOS,
		Home:    home,
		Cwd:     cwd,
		Getenv:  mapEnv(nil),
		Stat:    os.Stat,
		Lstat:   os.Lstat,
		ReadDir: os.ReadDir,
		Open: func(p string) (io.Closer, error) {
			f, err := os.Open(p) // #nosec G304 -- test fixture paths under t.TempDir
			if err != nil {
				return nil, err // avoid a typed-nil io.Closer
			}
			return f, nil
		},
	}, root
}

// place copies src (a path relative to the package dir, e.g. "testdata/cursor/mcp.json") to dst, creating parents.
func place(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o600); err != nil { // #nosec G703 -- test fixture path under t.TempDir
		t.Fatal(err)
	}
}

// slashList renders candidates as "scope path format label[ (shared)]" with slash paths, for golden comparisons.
func slashList(cs []Candidate) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		s := strings.TrimSpace(fmt.Sprintf("%s %s %s %s", c.Scope, filepath.ToSlash(c.Path), c.Format, c.Label))
		if c.Shared {
			s += " (shared)"
		}
		out = append(out, s)
	}
	return out
}

// statuses maps each file's slash path to its status.
func statuses(r Result) map[string]Status {
	m := map[string]Status{}
	for _, f := range r.Files {
		m[filepath.ToSlash(f.Path)] = f.Status
	}
	return m
}

// lookupClient returns the table row with id.
func lookupClient(t *testing.T, id string) Client {
	t.Helper()
	for _, c := range all {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("client %q not in table", id)
	return Client{}
}

// --- core tests ---

func TestClientTable(t *testing.T) {
	want := []string{"claude-code", "codex", "claude-desktop", "cursor", "vscode"}
	if len(all) != len(want) {
		t.Fatalf("len(all) = %d, want %d", len(all), len(want))
	}
	seen := map[string]bool{}
	for i, c := range all {
		if c.ID != want[i] {
			t.Errorf("all[%d].ID = %q, want %q", i, c.ID, want[i])
		}
		if seen[c.ID] {
			t.Errorf("duplicate ID %q", c.ID)
		}
		seen[c.ID] = true
		if c.Name == "" || c.candidates == nil || c.homes == nil {
			t.Errorf("row %q is incomplete", c.ID)
		}
	}
}

func TestUserConfigDir(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "xdg")
	cases := []struct {
		goos string
		vars map[string]string
		want string
	}{
		{"darwin", nil, "/h/Library/Application Support"},
		{"windows", map[string]string{"APPDATA": "/appdata"}, "/appdata"},
		{"windows", nil, "/h/AppData/Roaming"},
		{"linux", map[string]string{"XDG_CONFIG_HOME": xdg}, filepath.ToSlash(xdg)},
		{"linux", map[string]string{"XDG_CONFIG_HOME": "rel"}, "/h/.config"},
		{"linux", nil, "/h/.config"},
	}
	for _, c := range cases {
		if got := filepath.ToSlash(userConfigDir(fakeEnv(c.goos, c.vars))); got != c.want {
			t.Errorf("%s %v: got %q, want %q", c.goos, c.vars, got, c.want)
		}
	}
}

func TestLocalAppData(t *testing.T) {
	if got := filepath.ToSlash(localAppData(fakeEnv("windows", map[string]string{"LOCALAPPDATA": "/lad"}))); got != "/lad" {
		t.Errorf("got %q, want /lad", got)
	}
	if got := filepath.ToSlash(localAppData(fakeEnv("windows", nil))); got != "/h/AppData/Local" {
		t.Errorf("got %q, want /h/AppData/Local", got)
	}
	if got := localAppData(fakeEnv("linux", nil)); got != "" {
		t.Errorf("linux: got %q, want empty", got)
	}
}

func TestFakeEnv(t *testing.T) {
	env := fakeEnv("linux", nil, "/h/.cursor/mcp.json", "/h/profiles/b/", "/h/profiles/a/")
	if fi, err := env.Stat("/h/.cursor/mcp.json"); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("file: %v %v", fi, err)
	}
	if fi, err := env.Stat("/h/profiles/a"); err != nil || !fi.IsDir() {
		t.Errorf("trailing-slash dir: %v %v", fi, err)
	}
	if fi, err := env.Stat("/h/.cursor"); err != nil || !fi.IsDir() {
		t.Errorf("implied parent: %v %v", fi, err)
	}
	if _, err := env.Stat("/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("unknown: %v", err)
	}
	ents, err := env.ReadDir("/h/profiles")
	if err != nil || len(ents) != 2 || ents[0].Name() != "a" || ents[1].Name() != "b" || !ents[0].IsDir() {
		t.Errorf("ReadDir: %v %v", ents, err)
	}
	if c, err := env.Open("/h/.cursor/mcp.json"); err != nil || c.Close() != nil {
		t.Errorf("Open: %v", err)
	}
}

func TestSlashListAndLookup(t *testing.T) {
	got := slashList([]Candidate{
		{Scope: ScopeUser, Path: filepath.Join("/h", ".cursor", "mcp.json"), Format: FormatJSON},
		{Scope: ScopeProject, Path: "/w/proj/.mcp.json", Format: FormatJSON, Label: "shared with Claude Code", Shared: true},
	})
	want := []string{"user /h/.cursor/mcp.json json", "project /w/proj/.mcp.json json shared with Claude Code (shared)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("slashList = %q, want %q", got, want)
	}
	if c := lookupClient(t, "cursor"); c.Name != "Cursor" {
		t.Errorf("lookupClient(cursor).Name = %q", c.Name)
	}
}

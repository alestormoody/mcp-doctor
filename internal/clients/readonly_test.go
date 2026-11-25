package clients

import (
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type snapEntry struct {
	size  int64
	mode  fs.FileMode
	mtime time.Time
}

// snapshot records every entry under root; nothing is excluded.
func snapshot(t *testing.T, root string) map[string]snapEntry {
	t.Helper()
	m := map[string]snapEntry{}
	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// os.Lstat, not d.Info(): on Windows the directory listing carries stale
		// directory mtimes (NTFS updates them lazily), which made the snapshot flaky.
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		m[rel] = snapEntry{info.Size(), info.Mode(), info.ModTime()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func sameSnapshot(t *testing.T, before, after map[string]snapEntry) {
	t.Helper()
	if len(before) != len(after) {
		t.Errorf("entry count %d -> %d", len(before), len(after))
	}
	for p, b := range before {
		a, ok := after[p]
		if !ok {
			t.Errorf("%s disappeared", p)
			continue
		}
		if a.size != b.size || a.mode != b.mode || !a.mtime.Equal(b.mtime) {
			t.Errorf("%s changed: %+v -> %+v", p, b, a)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			t.Errorf("%s appeared", p)
		}
	}
}

func TestReadOnlySnapshot(t *testing.T) {
	env, root := fullFixture(t)
	before := snapshot(t, root)
	Discover(env)
	Load(env)
	sameSnapshot(t, before, snapshot(t, root))

	// Corrupted JSON and TOML files are parsed (and rejected) without being touched either.
	env, root = fullFixture(t)
	writeFileBytes(t, filepath.Join(env.Home, ".cursor", "mcp.json"), `{"mcpServers": {`)
	writeFileBytes(t, filepath.Join(env.Home, ".codex", "config.toml"), "[mcp_servers.x\ncommand = \"a")
	before = snapshot(t, root)
	if _, _, findings := Load(env); strings.Count(parseCodes(findings), "MCP_PARSE_001") != 2 {
		t.Errorf("corrupted files not reported: %s", parseCodes(findings))
	}
	sameSnapshot(t, before, snapshot(t, root))

	// A config path that is a directory: unreadable, still nothing changes.
	env, root = fullFixture(t)
	dirAsFile := filepath.Join(env.Home, ".cursor", "mcp.json")
	if err := os.Remove(dirAsFile); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirAsFile, 0o750); err != nil {
		t.Fatal(err)
	}
	before = snapshot(t, root)
	if _, findings := Discover(env); len(findings) != 1 || findings[0].Code != "MCP_DISC_001" {
		t.Errorf("findings = %+v", findings)
	}
	Load(env)
	sameSnapshot(t, before, snapshot(t, root))
}

func TestKnownLocationsOnly(t *testing.T) {
	env, _ := fullFixture(t)
	var touched []string
	regular := map[string]bool{}
	var opened, listed []string
	rec := func(p string) { touched = append(touched, filepath.Clean(p)) }
	stat, lstat, readDir, open := env.Stat, env.Lstat, env.ReadDir, env.Open
	env.Stat = func(p string) (fs.FileInfo, error) {
		rec(p)
		info, err := stat(p)
		if err == nil && info.Mode().IsRegular() {
			regular[filepath.Clean(p)] = true
		}
		return info, err
	}
	env.Lstat = func(p string) (fs.FileInfo, error) { rec(p); return lstat(p) }
	env.ReadDir = func(p string) ([]fs.DirEntry, error) { rec(p); listed = append(listed, p); return readDir(p) }
	env.Open = func(p string) (io.Closer, error) { rec(p); opened = append(opened, p); return open(p) }
	reads := map[string]int{}
	readFile := env.ReadFile
	env.ReadFile = func(p string) ([]byte, error) { rec(p); reads[filepath.Clean(p)]++; return readFile(p) }

	results, _, _ := Load(env)

	allowed := map[string]bool{}
	files := map[string]bool{}
	for _, r := range results {
		for _, f := range r.Files {
			allowed[filepath.Clean(f.Path)] = true
			files[filepath.Clean(f.Path)] = true
		}
	}
	for _, c := range all {
		for _, h := range c.homes(env) {
			allowed[filepath.Clean(h)] = true
		}
	}
	codex, _ := codexHome(env)
	allowed[filepath.Clean(codex)] = true
	for _, v := range vscodeVariants {
		allowed[filepath.Join(vscodeUserDir(env, v.dir), "profiles")] = true
	}
	for d := env.Cwd; ; d = filepath.Dir(d) {
		allowed[filepath.Join(d, ".git")] = true
		if filepath.Dir(d) == d {
			break
		}
	}

	for _, p := range touched {
		if !allowed[p] {
			t.Errorf("touched undeclared path %s", p)
		}
	}
	for _, p := range opened {
		if !files[filepath.Clean(p)] || !regular[filepath.Clean(p)] {
			t.Errorf("opened %s, which is not a File path that Stat saw as a regular file", p)
		}
	}
	if len(listed) > 3 {
		t.Errorf("ReadDir called %d times, want <= 3: %v", len(listed), listed)
	}
	if len(opened) == 0 {
		t.Error("recording Env saw no Open calls; the wrapper is not wired")
	}

	// Every found file is read exactly once per client that lists it (twice for the shared .mcp.json);
	// missing or unreadable files are never read.
	wantReads := map[string]int{}
	for _, r := range results {
		for _, f := range r.Files {
			if f.Status == StatusFound {
				wantReads[filepath.Clean(f.Path)]++
			}
		}
	}
	for p, n := range reads {
		if n != wantReads[p] {
			t.Errorf("ReadFile(%s) called %d times, want %d", p, n, wantReads[p])
		}
	}
	for p, n := range wantReads {
		if reads[p] != n {
			t.Errorf("found file %s read %d times, want %d", p, reads[p], n)
		}
	}
	if shared := filepath.Join(env.Cwd, ".mcp.json"); reads[shared] != 2 {
		t.Errorf("shared .mcp.json read %d times, want 2", reads[shared])
	}
}

// TestNoWriteAPI statically guards the production files. The text scan also matches comments and
// strings in non-test files, on purpose: do not mention those names in production comments.
func TestNoWriteAPI(t *testing.T) {
	forbiddenImports := []string{"os", "os/exec", "runtime", "syscall", "io/ioutil"}
	forbiddenNames := []string{"os.WriteFile", "os.Create", "os.Mkdir", "os.Remove", "os.Rename", "os.Chmod",
		"os.Chown", "os.OpenFile", "os.Symlink", "os.Chtimes", "os.Truncate", "exec."}
	checked := map[string]int{}
	var paths []string
	for _, dir := range []string{".", filepath.Join("..", "config")} {
		ps, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		checked[dir] = 0
		paths = append(paths, ps...)
	}
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		checked[filepath.Dir(p)]++
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			for _, bad := range forbiddenImports {
				if path == bad {
					t.Errorf("%s imports %q", p, path)
				}
			}
		}
		src, err := os.ReadFile(p) // #nosec G304 -- package source file
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range forbiddenNames {
			if strings.Contains(string(src), bad) {
				t.Errorf("%s contains %q", p, bad)
			}
		}
	}
	for dir, n := range checked {
		if n == 0 {
			t.Errorf("no production files found in %s", dir)
		}
	}
}

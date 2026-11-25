package clients

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexCandidates(t *testing.T) {
	user := "user /h/.codex/config.toml toml"
	proj := "project /w/proj/.codex/config.toml toml"
	managed := "managed /etc/codex/config.toml toml"
	cases := []struct {
		name, goos string
		vars       map[string]string
		paths      []string
		want       []string
	}{
		{"linux", "linux", nil, nil, []string{managed, user, proj}},
		{"darwin", "darwin", nil, nil, []string{managed, user, proj}},
		{"windows", "windows", nil, nil, []string{user, proj}},
		{"CODEX_HOME", "linux", map[string]string{"CODEX_HOME": "/ch"}, nil,
			[]string{managed, "user /ch/config.toml toml CODEX_HOME", proj}},
		{"profiles", "linux", nil, []string{"/h/.codex/config.toml", "/h/.codex/work.config.toml", "/h/.codex/a.config.toml",
			"/h/.codex/notes.toml", "/h/.codex/dir.config.toml/"},
			[]string{managed, user, "user /h/.codex/a.config.toml toml profile a",
				"user /h/.codex/work.config.toml toml profile work", proj}},
	}
	for _, c := range cases {
		got := slashList(codexCandidates(fakeEnv(c.goos, c.vars, c.paths...)))
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestCodexHomes(t *testing.T) {
	for _, c := range []struct {
		vars map[string]string
		want string
	}{{nil, "/h/.codex"}, {map[string]string{"CODEX_HOME": "/ch"}, "/ch"}} {
		got := codexHomes(fakeEnv("linux", c.vars))
		if len(got) != 1 || filepath.ToSlash(got[0]) != c.want {
			t.Errorf("%v: got %q, want [%s]", c.vars, got, c.want)
		}
	}
}

func TestCodexProjectWalk(t *testing.T) {
	cases := []struct {
		name, cwd string
		paths     []string
		want      []string
	}{
		{"git dir at root", "/w/proj/a/b", []string{"/w/proj/.git/"},
			[]string{"/w/proj", "/w/proj/a", "/w/proj/a/b"}},
		{"git file (worktree)", "/w/proj/a/b", []string{"/w/proj/.git"},
			[]string{"/w/proj", "/w/proj/a", "/w/proj/a/b"}},
		{"git at cwd", "/w/proj", []string{"/w/proj/.git/"}, []string{"/w/proj"}},
		{"no git", "/w/proj/a", nil, []string{"/w/proj/a"}},
		{"nested repos", "/w/proj/a", []string{"/w/.git/", "/w/proj/.git/"}, []string{"/w/proj", "/w/proj/a"}},
	}
	for _, c := range cases {
		env := fakeEnv("linux", nil, c.paths...)
		env.Cwd = c.cwd
		var want []string
		for _, d := range c.want {
			want = append(want, "project "+d+"/.codex/config.toml toml")
		}
		var got []string
		for _, s := range slashList(codexCandidates(env)) {
			if strings.HasPrefix(s, "project ") {
				got = append(got, s)
			}
		}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, want)
		}
	}

	env := fakeEnv("linux", nil)
	env.Cwd = "/w/proj/a"
	var probes []string
	stat := env.Stat
	env.Stat = func(p string) (fs.FileInfo, error) { probes = append(probes, filepath.ToSlash(p)); return stat(p) }
	codexProjectDirs(env)
	if len(probes) != 4 || probes[len(probes)-1] != "/.git" {
		t.Errorf("walk probes = %q, want 4 ending in /.git", probes)
	}

	probes = nil
	env.Cwd = ""
	if dirs := codexProjectDirs(env); dirs != nil || len(probes) != 0 {
		t.Errorf("empty Cwd: dirs %q, probes %q", dirs, probes)
	}
	for _, s := range slashList(codexCandidates(env)) {
		if strings.HasPrefix(s, "project ") {
			t.Errorf("empty Cwd: project candidate %q", s)
		}
	}
	if len(probes) != 0 {
		t.Errorf("empty Cwd: Stat probes %q", probes)
	}
}

func TestCodexFixture(t *testing.T) {
	env, _ := fixtureEnv(t)
	cwd := filepath.Join(env.Cwd, "sub")
	for _, d := range []string{filepath.Join(env.Cwd, ".git"), cwd} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	codexDir := filepath.Join(env.Home, ".codex")
	user, work := filepath.Join(codexDir, "config.toml"), filepath.Join(codexDir, "work.config.toml")
	proj, sub := filepath.Join(env.Cwd, ".codex", "config.toml"), filepath.Join(cwd, ".codex", "config.toml")
	place(t, "testdata/codex/config.toml", user)
	place(t, "testdata/codex/work.config.toml", work)
	place(t, "testdata/codex/project.toml", proj)
	place(t, "testdata/codex/project.toml", sub)
	env.Cwd = cwd

	codex := lookupClient(t, "codex")
	r, findings := discoverOne(env, codex)
	if len(findings) != 0 || !r.Detected {
		t.Fatalf("findings %+v, detected %v", findings, r.Detected)
	}
	st := statuses(r)
	for _, p := range []string{user, work, proj, sub} {
		if st[filepath.ToSlash(p)] != StatusFound {
			t.Errorf("%s: status %q", p, st[filepath.ToSlash(p)])
		}
	}
	for _, f := range r.Files {
		if f.Path == work && f.Label != "profile work" {
			t.Errorf("profile label = %q", f.Label)
		}
	}

	env.ReadDir = func(p string) ([]fs.DirEntry, error) {
		return nil, &fs.PathError{Op: "readdir", Path: p, Err: fs.ErrPermission}
	}
	_, findings = discoverOne(env, codex)
	if len(findings) != 1 || findings[0].Code != "MCP_DISC_001" || findings[0].Source != codexDir {
		t.Errorf("ReadDir permission: findings = %+v", findings)
	}

	env2, _ := fixtureEnv(t)
	env2.Cwd = env2.Home
	home := filepath.Join(env2.Home, ".codex", "config.toml")
	place(t, "testdata/codex/config.toml", home)
	r, _ = discoverOne(env2, codex)
	var n int
	for _, f := range r.Files {
		if f.Path == home {
			n++
			if f.Scope != ScopeUser {
				t.Errorf("Cwd == Home: scope %q, want user", f.Scope)
			}
		}
	}
	if n != 1 {
		t.Errorf("Cwd == Home: %d files for %s, want 1", n, home)
	}
}

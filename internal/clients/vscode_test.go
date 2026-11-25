package clients

import (
	"io"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVSCodeCandidates(t *testing.T) {
	const u = "/h/Library/Application Support"
	dirs := []string{u + "/Code/User/profiles/abc/", u + "/Code/User/profiles/7f1/", u + "/Code/User/profiles/x.json",
		u + "/Code - Insiders/User/profiles/p1/"}
	tail := []string{
		"project /w/proj/.vscode/mcp.json jsonc",
		"project /w/proj/.mcp.json json shared with Claude Code (shared)",
	}
	cases := []struct {
		goos  string
		vars  map[string]string
		paths []string
		want  []string
	}{
		{"darwin", nil, dirs, append([]string{
			"user " + u + "/Code/User/mcp.json jsonc",
			"user " + u + "/Code/User/profiles/7f1/mcp.json jsonc profile 7f1",
			"user " + u + "/Code/User/profiles/abc/mcp.json jsonc profile abc",
			"user " + u + "/Code - Insiders/User/mcp.json jsonc Insiders",
			"user " + u + "/Code - Insiders/User/profiles/p1/mcp.json jsonc Insiders profile p1",
			"user /h/.copilot/mcp-config.json json",
		}, tail...)},
		{"darwin", map[string]string{"COPILOT_HOME": "/cp"}, nil, append([]string{
			"user " + u + "/Code/User/mcp.json jsonc",
			"user " + u + "/Code - Insiders/User/mcp.json jsonc Insiders",
			"user /cp/mcp-config.json json COPILOT_HOME",
		}, tail...)},
		{"linux", nil, nil, append([]string{
			"user /h/.config/Code/User/mcp.json jsonc",
			"user /h/.config/Code - Insiders/User/mcp.json jsonc Insiders",
			"user /h/.copilot/mcp-config.json json",
		}, tail...)},
		{"windows", map[string]string{"APPDATA": "/appdata"}, nil, append([]string{
			"user /appdata/Code/User/mcp.json jsonc",
			"user /appdata/Code - Insiders/User/mcp.json jsonc Insiders",
			"user /h/.copilot/mcp-config.json json",
		}, tail...)},
	}
	for _, c := range cases {
		got := slashList(vscodeCandidates(fakeEnv(c.goos, c.vars, c.paths...)))
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s %v:\n got %q\nwant %q", c.goos, c.vars, got, c.want)
		}
	}
}

func TestVSCodeHomes(t *testing.T) {
	const u = "/h/Library/Application Support"
	for vars, want := range map[string][]string{
		"":    {u + "/Code/User", u + "/Code - Insiders/User", "/h/.copilot"},
		"/cp": {u + "/Code/User", u + "/Code - Insiders/User", "/cp"},
	} {
		var got []string
		for _, h := range vscodeHomes(fakeEnv("darwin", map[string]string{"COPILOT_HOME": vars})) {
			got = append(got, filepath.ToSlash(h))
		}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("COPILOT_HOME=%q: got %q, want %q", vars, got, want)
		}
	}
}

// vscodeEnv is a fixtureEnv whose Windows variables point inside root, never at the real profile.
func vscodeEnv(t *testing.T) Env {
	t.Helper()
	env, root := fixtureEnv(t)
	if runtime.GOOS == "windows" {
		env.Getenv = mapEnv(map[string]string{
			"APPDATA":      filepath.Join(root, "appdata"),
			"LOCALAPPDATA": filepath.Join(root, "localappdata"),
		})
	}
	return env
}

func TestVSCodeFixture(t *testing.T) {
	c := lookupClient(t, "vscode")

	env := vscodeEnv(t)
	u := userConfigDir(env)
	want := []string{
		filepath.Join(u, "Code", "User", "mcp.json"),
		filepath.Join(u, "Code - Insiders", "User", "profiles", "p1", "mcp.json"),
		filepath.Join(env.Home, ".copilot", "mcp-config.json"),
		filepath.Join(env.Cwd, ".vscode", "mcp.json"),
	}
	place(t, "testdata/vscode/mcp.json", want[0])
	place(t, "testdata/vscode/mcp.json", want[1])
	place(t, "testdata/vscode/mcp-config.json", want[2])
	place(t, "testdata/vscode/mcp.json", want[3])
	r, findings := discoverOne(env, c)
	if !r.Detected || len(findings) != 0 {
		t.Errorf("placed: Detected=%v findings=%+v", r.Detected, findings)
	}
	st := statuses(r)
	for _, p := range want {
		if st[filepath.ToSlash(p)] != StatusFound {
			t.Errorf("%s: %s", p, st[filepath.ToSlash(p)])
		}
	}

	env = vscodeEnv(t)
	shared := filepath.Join(env.Cwd, ".mcp.json")
	place(t, "testdata/vscode/mcp-config.json", shared)
	r, _ = discoverOne(env, c)
	if r.Detected || statuses(r)[filepath.ToSlash(shared)] != StatusFound {
		t.Errorf("only shared .mcp.json: %+v", r)
	}
}

func TestVSCodeEmptyHomeNoRelativePaths(t *testing.T) {
	env, _ := fixtureEnv(t)
	env.Home = ""
	env.Getenv = mapEnv(nil)
	var touched []string
	rec := func(p string) { touched = append(touched, p) }
	stat, lstat, readDir, open := env.Stat, env.Lstat, env.ReadDir, env.Open
	env.Stat = func(p string) (fs.FileInfo, error) { rec(p); return stat(p) }
	env.Lstat = func(p string) (fs.FileInfo, error) { rec(p); return lstat(p) }
	env.ReadDir = func(p string) ([]fs.DirEntry, error) { rec(p); return readDir(p) }
	env.Open = func(p string) (io.Closer, error) { rec(p); return open(p) }

	for _, id := range []string{"vscode", "claude-desktop"} {
		r, _ := discoverOne(env, lookupClient(t, id))
		if id == "vscode" {
			st := statuses(r)
			for _, p := range []string{filepath.Join(env.Cwd, ".vscode", "mcp.json"), filepath.Join(env.Cwd, ".mcp.json")} {
				if _, ok := st[filepath.ToSlash(p)]; !ok {
					t.Errorf("project candidate %s not listed", p)
				}
			}
		}
	}
	if len(touched) == 0 {
		t.Fatal("nothing touched")
	}
	for _, p := range touched {
		if !filepath.IsAbs(p) {
			t.Errorf("relative path touched: %q", p)
		}
	}
}

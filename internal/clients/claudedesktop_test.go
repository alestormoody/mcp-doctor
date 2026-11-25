package clients

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClaudeDesktopCandidates(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "xdg")
	const f = "claude_desktop_config.json"
	cases := []struct {
		goos string
		vars map[string]string
		want []string
	}{
		{"darwin", nil, []string{"user /h/Library/Application Support/Claude/" + f + " json"}},
		{"linux", nil, []string{"user /h/.config/Claude/" + f + " json"}},
		{"linux", map[string]string{"XDG_CONFIG_HOME": xdg}, []string{"user " + filepath.ToSlash(xdg) + "/Claude/" + f + " json"}},
		{"windows", map[string]string{"APPDATA": "/appdata", "LOCALAPPDATA": "/lad"}, []string{
			"user /appdata/Claude/" + f + " json",
			"user /lad/Packages/Claude_pzs8sxrjxfjjc/LocalCache/Roaming/Claude/" + f + " json MSIX",
			"user /lad/Claude-Data/Claude_pzs8sxrjxfjjc/" + f + " json MSIX",
		}},
		{"windows", nil, []string{
			"user /h/AppData/Roaming/Claude/" + f + " json",
			"user /h/AppData/Local/Packages/Claude_pzs8sxrjxfjjc/LocalCache/Roaming/Claude/" + f + " json MSIX",
			"user /h/AppData/Local/Claude-Data/Claude_pzs8sxrjxfjjc/" + f + " json MSIX",
		}},
	}
	for _, c := range cases {
		got := slashList(claudeDesktopCandidates(fakeEnv(c.goos, c.vars)))
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s %v:\n got %q\nwant %q", c.goos, c.vars, got, c.want)
		}
	}
}

func TestClaudeDesktopHomes(t *testing.T) {
	cases := []struct {
		goos string
		want []string
	}{
		{"darwin", []string{"/h/Library/Application Support/Claude"}},
		{"linux", []string{"/h/.config/Claude"}},
		{"windows", []string{
			"/appdata/Claude",
			"/lad/Packages/Claude_pzs8sxrjxfjjc/LocalCache/Roaming/Claude",
			"/lad/Claude-Data/Claude_pzs8sxrjxfjjc",
		}},
	}
	for _, c := range cases {
		var got []string
		for _, h := range claudeDesktopHomes(fakeEnv(c.goos, map[string]string{"APPDATA": "/appdata", "LOCALAPPDATA": "/lad"})) {
			got = append(got, filepath.ToSlash(h))
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s: got %q, want %q", c.goos, got, c.want)
		}
	}
}

// claudeDesktopEnv is a fixtureEnv whose Windows variables point inside root, never at the real profile.
func claudeDesktopEnv(t *testing.T) (Env, string) {
	t.Helper()
	env, root := fixtureEnv(t)
	if runtime.GOOS == "windows" {
		env.Getenv = mapEnv(map[string]string{
			"APPDATA":      filepath.Join(root, "appdata"),
			"LOCALAPPDATA": filepath.Join(root, "localappdata"),
		})
	}
	return env, root
}

func TestClaudeDesktopFixture(t *testing.T) {
	const src = "testdata/claudedesktop/claude_desktop_config.json"
	c := lookupClient(t, "claude-desktop")

	env, _ := claudeDesktopEnv(t)
	r, _ := discoverOne(env, c)
	if r.Detected {
		t.Error("empty: Detected true")
	}
	for p, s := range statuses(r) {
		if s != StatusMissing {
			t.Errorf("empty: %s is %s", p, s)
		}
	}

	placedEnv, root := claudeDesktopEnv(t)
	envs := []Env{placedEnv}
	if runtime.GOOS == "linux" { // XDG_CONFIG_HOME moves the file
		xdgEnv := placedEnv
		xdgEnv.Getenv = mapEnv(map[string]string{"XDG_CONFIG_HOME": filepath.Join(root, "xdg")})
		envs = append(envs, xdgEnv)
	}
	for _, env := range envs {
		claudeDesktopPlaced(t, env, c, src)
	}
}

func claudeDesktopPlaced(t *testing.T, env Env, c Client, src string) {
	t.Helper()
	cands := claudeDesktopCandidates(env)
	for _, cand := range cands { // on non-windows there is only the user file
		place(t, src, cand.Path)
	}
	r, _ := discoverOne(env, c)
	if !r.Detected || len(r.Files) != len(cands) {
		t.Fatalf("placed: %+v", r)
	}
	for _, cand := range cands {
		if s := statuses(r)[filepath.ToSlash(cand.Path)]; s != StatusFound {
			t.Errorf("%s: %s", cand.Path, s)
		}
	}
	if want := filepath.Join(userConfigDir(env), "Claude", "claude_desktop_config.json"); cands[0].Path != want {
		t.Errorf("first candidate %q, want %q", cands[0].Path, want)
	}
}

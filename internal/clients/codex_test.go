package clients

import (
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

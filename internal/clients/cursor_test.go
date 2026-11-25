package clients

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCursorCandidates(t *testing.T) {
	want := []string{"user /h/.cursor/mcp.json json", "project /w/proj/.cursor/mcp.json json"}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		if got := slashList(cursorCandidates(fakeEnv(goos, nil))); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s:\n got %q\nwant %q", goos, got, want)
		}
	}
}

func TestCursorHomes(t *testing.T) {
	if got := cursorHomes(fakeEnv("linux", nil)); len(got) != 1 || filepath.ToSlash(got[0]) != "/h/.cursor" {
		t.Errorf("got %q", got)
	}
}

func TestCursorFixture(t *testing.T) {
	c := lookupClient(t, "cursor")
	userOf := func(env Env) string { return filepath.Join(env.Home, ".cursor", "mcp.json") }
	projOf := func(env Env) string { return filepath.Join(env.Cwd, ".cursor", "mcp.json") }

	env, _ := fixtureEnv(t)
	place(t, "testdata/cursor/mcp.json", userOf(env))
	place(t, "testdata/cursor/mcp.json", projOf(env))
	r, _ := discoverOne(env, c)
	if st := statuses(r); st[filepath.ToSlash(userOf(env))] != StatusFound || st[filepath.ToSlash(projOf(env))] != StatusFound || !r.Detected {
		t.Errorf("both placed: %+v", r)
	}

	env, _ = fixtureEnv(t)
	r, _ = discoverOne(env, c)
	if st := statuses(r); st[filepath.ToSlash(userOf(env))] != StatusMissing || st[filepath.ToSlash(projOf(env))] != StatusMissing || r.Detected {
		t.Errorf("empty: %+v", r)
	}

	if err := os.MkdirAll(filepath.Join(env.Home, ".cursor"), 0o750); err != nil {
		t.Fatal(err)
	}
	if r, _ = discoverOne(env, c); !r.Detected {
		t.Errorf("home dir only: %+v", r)
	}

	env, _ = fixtureEnv(t)
	env.Cwd = env.Home
	place(t, "testdata/cursor/mcp.json", userOf(env))
	r, _ = discoverOne(env, c)
	if len(r.Files) != 1 || r.Files[0].Scope != ScopeUser || r.Files[0].Status != StatusFound {
		t.Errorf("Cwd == Home: %+v", r.Files)
	}
}

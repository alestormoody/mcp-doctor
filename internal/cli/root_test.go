package cli

import (
	"bytes"
	"strings"
	"testing"
)

func run(args ...string) (int, string, string) {
	var out, errw bytes.Buffer
	code := Execute(args, &out, &errw)
	return code, out.String(), errw.String()
}

func TestVersion(t *testing.T) {
	code, out, errw := run("version")
	want := "MCP Doctor dev\ncommit: none\nbuilt: unknown\nhttps://github.com/alestormoody/mcp-doctor\n"
	if code != 0 || out != want || errw != "" {
		t.Errorf("got code=%d out=%q err=%q", code, out, errw)
	}
}

func TestVersionInjected(t *testing.T) {
	saved := version
	version = "1.2.3"
	t.Cleanup(func() { version = saved })
	_, out, _ := run("version")
	if first := strings.SplitN(out, "\n", 2)[0]; first != "MCP Doctor 1.2.3" {
		t.Errorf("first line = %q", first)
	}
}

func TestUnknownCommand(t *testing.T) {
	// completion is cobra's default command; only version may be user-facing.
	for _, name := range []string{"nope", "completion"} {
		code, out, errw := run(name)
		if code != 2 || out != "" || !strings.Contains(errw, "unknown command") {
			t.Errorf("%s: code=%d out=%q err=%q", name, code, out, errw)
		}
	}
}

func TestVersionExtraArgs(t *testing.T) {
	if code, out, _ := run("version", "extra"); code != 2 || out != "" {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestRootHelp(t *testing.T) {
	code, out, _ := run()
	if code != 0 || !strings.Contains(out, "Usage:") {
		t.Errorf("code=%d out=%q", code, out)
	}
	for _, cmd := range []string{"clients", "servers", "validate", "version"} {
		if !strings.Contains(out, "\n  "+cmd+" ") {
			t.Errorf("help does not list %s:\n%s", cmd, out)
		}
	}
}

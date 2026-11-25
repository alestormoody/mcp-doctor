package clients

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/alestormoody/mcp-doctor/internal/diag"
)

const cdDir = "testdata/claudedesktop/parse/"

// cdParse parses one fixture as the user-scope Claude Desktop file at /p.
func cdParse(t *testing.T, name string) ([]Server, []diag.Finding, []byte) {
	t.Helper()
	src, err := os.ReadFile(cdDir + name) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatal(err)
	}
	ss, fds := claudeDesktopServers(jsonFile("/p", FormatJSON, ScopeUser), src)
	parseCheckFindings(t, fds)
	for _, s := range ss {
		if s.raw == nil || !bytes.Contains(src, *s.raw) || s.Shadowed {
			t.Errorf("%s: raw not a file slice or shadowed", s.Name)
		}
		if strings.Contains(fmt.Sprintf("%+v %v %#v", s, s, s), "CANARY") {
			t.Errorf("%s: formatted Server leaks a value", s.Name)
		}
	}
	return ss, fds, src
}

func TestClaudeDesktopParseValid(t *testing.T) {
	ss, fds, src := cdParse(t, "valid.json")
	if len(fds) != 0 || len(ss) != 1 {
		t.Fatalf("servers %d, findings %+v", len(ss), fds)
	}
	s := ss[0]
	wantSrc := Source{Client: "claude-desktop", Path: "/p", Scope: ScopeUser, Format: FormatJSON, Line: 4, Col: 5}
	if s.Name != "filesystem" || s.Source != wantSrc || s.Transport != "stdio" || !s.TransportInferred || s.Command != "npx" {
		t.Errorf("server = %+v", s)
	}
	if got := secrets(s.Args); !reflect.DeepEqual(got, []string{"-y", "@modelcontextprotocol/server-filesystem", "${APPDATA}"}) {
		t.Errorf("args = %q", got)
	}
	if got := secretMap(s.Env); !reflect.DeepEqual(got, map[string]string{"KEY": "CANARY-e"}) {
		t.Errorf("env = %v", got)
	}
	if s.Refs != nil || s.Unknown != nil || s.Headers != nil || s.URL.Reveal() != "" {
		t.Errorf("refs %v unknown %v headers %v", s.Refs, s.Unknown, s.Headers)
	}
	start := bytes.Index(src, []byte(`{
      "command"`))
	if want := src[start : bytes.Index(src, []byte("}\n  }"))+1]; !bytes.Equal(*s.raw, want) {
		t.Errorf("raw = %q, want %q", *s.raw, want)
	}
}

func TestClaudeDesktopParseRemote(t *testing.T) {
	ss, fds, _ := cdParse(t, "remote.json")
	if len(ss) != 1 || parseCodes(fds) != "MCP_PARSE_005" {
		t.Fatalf("servers %d, findings %+v", len(ss), fds)
	}
	s := ss[0]
	if s.Transport != "http" || !s.TransportInferred || s.Command != "" || s.URL.Reveal() != "https://h.example/mcp/CANARY-p" ||
		!reflect.DeepEqual(s.Unknown, []string{"type", "url"}) {
		t.Errorf("server = %+v url %q", s, s.URL.Reveal())
	}
	if fds[0].Server != "remote" || fds[0].Source != "/p:3:5" || fds[0].Message != "Unknown field(s): type, url" {
		t.Errorf("005 = %+v", fds[0])
	}
}

func TestClaudeDesktopParseComments(t *testing.T) {
	ss, fds, _ := cdParse(t, "comments.json")
	if parseCodes(fds) != "MCP_PARSE_006" || fds[0].Source != "/p:2:3" {
		t.Errorf("findings %+v", fds)
	}
	if len(ss) != 1 || ss[0].Name != "filesystem" || strings.Join(secrets(ss[0].Args), " ") != "-y pkg" {
		t.Errorf("servers %+v", ss)
	}
}

func TestClaudeDesktopParseIncomplete(t *testing.T) {
	ss, fds, _ := cdParse(t, "incomplete.json")
	if len(ss) != 0 || parseCodes(fds) != "MCP_PARSE_003,MCP_PARSE_002" {
		t.Fatalf("servers %+v, findings %+v", ss, fds)
	}
	if fds[0].Server != "empty" || fds[0].Source != "/p:3:5" {
		t.Errorf("003 = %+v", fds[0])
	}
	if fds[1].Server != "badargs" || fds[1].Source != "/p:4:43" {
		t.Errorf("002 = %+v", fds[1])
	}
}

func TestClaudeDesktopParseCorrupted(t *testing.T) {
	ss, fds, _ := cdParse(t, "corrupted.json")
	if len(ss) != 0 || parseCodes(fds) != "MCP_PARSE_001" || !regexp.MustCompile(`^/p:\d+:\d+$`).MatchString(fds[0].Source) {
		t.Errorf("servers %+v, findings %+v", ss, fds)
	}
}

func TestClaudeDesktopParseNotObject(t *testing.T) {
	for _, src := range []string{`[]`, `{"mcpServers": []}`, `{"mcpServers": {"x": "CANARY"}}`} {
		ss, fds := claudeDesktopServers(jsonFile("/p", FormatJSON, ScopeUser), []byte(src))
		parseCheckFindings(t, fds)
		if len(ss) != 0 || parseCodes(fds) != "MCP_PARSE_002" {
			t.Errorf("%s: servers %+v findings %+v", src, ss, fds)
		}
	}
	if ss, fds := claudeDesktopServers(jsonFile("/p", FormatJSON, ScopeUser), []byte(`{"other": 1}`)); len(ss) != 0 || len(fds) != 0 {
		t.Errorf("no mcpServers: %+v %+v", ss, fds)
	}
}

// TestClaudeDesktopLoadMSIX runs Load with Windows paths inside the temp root on every OS: the same server in
// the roaming file and both MSIX files is listed three times, each with its file's label, none shadowed.
func TestClaudeDesktopLoadMSIX(t *testing.T) {
	env, root := fixtureEnv(t)
	env.GOOS = "windows"
	env.Getenv = mapEnv(map[string]string{"APPDATA": filepath.Join(root, "appdata"), "LOCALAPPDATA": filepath.Join(root, "localappdata")})
	cands := claudeDesktopCandidates(env)
	if len(cands) != 3 {
		t.Fatalf("candidates %+v", cands)
	}
	for _, c := range cands {
		place(t, cdDir+"valid.json", c.Path)
	}
	_, servers, fds := Load(env)
	parseCheckFindings(t, fds)
	var got []string
	for _, s := range servers {
		if s.Source.Client != "claude-desktop" {
			continue
		}
		if s.Name != "filesystem" || s.Shadowed {
			t.Errorf("server %+v", s)
		}
		got = append(got, s.Source.Path+"|"+s.Source.Label)
	}
	want := []string{cands[0].Path + "|", cands[1].Path + "|MSIX", cands[2].Path + "|MSIX"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// cdFuzzCheck asserts the parser invariants shared by the Claude Desktop and Cursor fuzz targets.
func cdFuzzCheck(t *testing.T, client string, input []byte, ss []Server, fds []diag.Finding) {
	t.Helper()
	for _, f := range fds {
		if err := f.Validate(); err != nil || f.Client != client {
			t.Fatalf("finding %+v: %v", f, err)
		}
	}
	for _, s := range ss {
		if s.Shadowed || s.Source.Client != client || s.raw == nil || !bytes.Contains(stripBOM(input), *s.raw) {
			t.Fatalf("server %q: shadowed, wrong client or raw not in input", s.Name)
		}
	}
}

func FuzzParseClaudeDesktop(f *testing.F) {
	for _, b := range readFixtures(f, cdDir) {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		ss, fds := claudeDesktopServers(jsonFile("/p", FormatJSON, ScopeUser), input)
		cdFuzzCheck(t, "claude-desktop", input, ss, fds)
	})
}

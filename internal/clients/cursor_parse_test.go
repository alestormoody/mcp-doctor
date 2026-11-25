package clients

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/alestormoody/mcp-doctor/internal/diag"
)

const cursorDir = "testdata/cursor/parse/"

// cursorParse parses one fixture as the user-scope Cursor file at /p.
func cursorParse(t *testing.T, name string) ([]Server, []diag.Finding) {
	t.Helper()
	src, err := os.ReadFile(cursorDir + name) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatal(err)
	}
	ss, fds := cursorFile(jsonFile("/p", FormatJSON, ScopeUser), src)
	parseCheckFindings(t, fds)
	for _, s := range ss {
		if s.raw == nil || !bytes.Contains(src, *s.raw) || !bytes.HasPrefix(*s.raw, []byte("{")) || s.Shadowed {
			t.Errorf("%s: raw not an entry slice or shadowed", s.Name)
		}
		if strings.Contains(fmt.Sprintf("%+v %v %#v", s, s, s), "CANARY") {
			t.Errorf("%s: formatted Server leaks a value", s.Name)
		}
	}
	return ss, fds
}

func TestCursorParseValid(t *testing.T) {
	ss, fds := cursorParse(t, "valid.json")
	if len(fds) != 0 || len(ss) != 2 {
		t.Fatalf("servers %d, findings %+v", len(ss), fds)
	}
	src := func(line int) Source {
		return Source{Client: "cursor", Path: "/p", Scope: ScopeUser, Format: FormatJSON, Line: line, Col: 5}
	}
	l, r := ss[0], ss[1]
	if l.Name != "local" || l.Source != src(3) || l.Transport != "stdio" || l.TransportInferred || l.Command != "node" ||
		l.EnvFile != ".env" || l.Headers != nil || l.URL.Reveal() != "" || l.Unknown != nil {
		t.Errorf("local = %+v", l)
	}
	if got := secrets(l.Args); !reflect.DeepEqual(got, []string{"${workspaceFolder}/srv.js"}) {
		t.Errorf("local args = %q", got)
	}
	if got := secretMap(l.Env); !reflect.DeepEqual(got, map[string]string{"K": "${env:API_KEY}"}) {
		t.Errorf("local env = %v", got)
	}
	wantL := []VarRef{{Field: "args[0]", Kind: "workspace", Name: "workspaceFolder"}, {Field: "env.K", Kind: "env", Name: "API_KEY"}}
	if !reflect.DeepEqual(l.Refs, wantL) {
		t.Errorf("local refs = %+v", l.Refs)
	}
	if r.Name != "remote" || r.Source != src(10) || r.Transport != "http" || !r.TransportInferred || r.Command != "" ||
		r.Args != nil || r.Env != nil || r.URL.Reveal() != "https://h.example/mcp?key=CANARY-q" || r.Unknown != nil {
		t.Errorf("remote = %+v", r)
	}
	if got := secretMap(r.Headers); !reflect.DeepEqual(got, map[string]string{"Authorization": "Bearer CANARY-h"}) {
		t.Errorf("remote headers = %v", got)
	}
	if want := []VarRef{{Field: "auth.CLIENT_ID", Kind: "env", Name: "CID"}}; !reflect.DeepEqual(r.Refs, want) {
		t.Errorf("remote refs = %+v", r.Refs)
	}
}

func TestCursorRefsAllFields(t *testing.T) {
	src := `{"mcpServers": {"s": {"command": "${userHome}/bin", "args": ["a", "${pathSeparator}${/}"],
	  "env": {"B": "${env:B}", "A": "${workspaceFolderBasename}"}, "url": "https://${env:HOST}",
	  "headers": {"X": "${env:TOK}"}, "auth": {"CLIENT_ID": "x", "CLIENT_SECRET": "${env:SEC}"}}}}`
	ss, fds := cursorFile(jsonFile("/p", FormatJSON, ScopeProject), []byte(src))
	if len(fds) != 0 || len(ss) != 1 {
		t.Fatalf("servers %+v findings %+v", ss, fds)
	}
	var got []string
	for _, r := range ss[0].Refs {
		got = append(got, r.Field+" "+r.Kind+" "+r.Name)
	}
	want := []string{"command userHome userHome", "args[1] other pathSeparator", "args[1] other /",
		"env.A workspace workspaceFolderBasename", "env.B env B", "url env HOST", "headers.X env TOK", "auth.CLIENT_SECRET env SEC"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("refs\n got %q\nwant %q", got, want)
	}
}

func TestCursorParseIncomplete(t *testing.T) {
	ss, fds := cursorParse(t, "incomplete.json")
	if parseCodes(fds) != "MCP_PARSE_008,MCP_PARSE_005,MCP_PARSE_002" {
		t.Fatalf("findings %+v", fds)
	}
	if fds[0].Server != "nocmd" || fds[0].Source != "/p:3:5" || fds[0].Message != "stdio server has a url but no command" {
		t.Errorf("008 = %+v", fds[0])
	}
	if fds[1].Server != "typo" || fds[1].Message != "Unknown field(s): comand" {
		t.Errorf("005 = %+v", fds[1])
	}
	if fds[2].Server != "badauth" || fds[2].Source != "/p:5:57" {
		t.Errorf("002 = %+v", fds[2])
	}
	if len(ss) != 1 || ss[0].Name != "typo" || !reflect.DeepEqual(ss[0].Unknown, []string{"comand"}) {
		t.Errorf("servers %+v", ss)
	}
}

func TestCursorParseCorrupted(t *testing.T) {
	ss, fds := cursorParse(t, "corrupted.json")
	if len(ss) != 0 || parseCodes(fds) != "MCP_PARSE_001" || !regexp.MustCompile(`^/p:\d+:\d+$`).MatchString(fds[0].Source) {
		t.Errorf("servers %+v, findings %+v", ss, fds)
	}
}

func TestCursorParseComments(t *testing.T) {
	ss, fds := cursorParse(t, "comments.json")
	if parseCodes(fds) != "MCP_PARSE_006" || fds[0].Source != "/p:3:5" {
		t.Errorf("findings %+v", fds)
	}
	if len(ss) != 1 || ss[0].Name != "local" || ss[0].Command != "node" {
		t.Errorf("servers %+v", ss)
	}
}

func TestCursorParseFiles(t *testing.T) {
	valid, err := os.ReadFile(cursorDir + "valid.json")
	if err != nil {
		t.Fatal(err)
	}
	ss, fds := parseCursor(Env{}, []Loaded{
		{File: jsonFile("/u", FormatJSON, ScopeUser), Src: valid},
		{File: jsonFile("/w", FormatJSON, ScopeProject), Src: valid},
	})
	var got []string
	for _, s := range ss {
		got = append(got, fmt.Sprint(s.Source.Path, s.Source.Scope, s.Name, s.Shadowed))
	}
	if len(fds) != 0 || strings.Join(got, "|") != "/uuserlocalfalse|/uuserremotefalse|/wprojectlocalfalse|/wprojectremotefalse" {
		t.Errorf("got %q findings %+v", got, fds)
	}
}

func FuzzParseCursor(f *testing.F) {
	for _, b := range readFixtures(f, cursorDir) {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		ss, fds := cursorFile(jsonFile("/p", FormatJSON, ScopeProject), input)
		cdFuzzCheck(t, "cursor", input, ss, fds)
	})
}

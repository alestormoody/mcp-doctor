package clients

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alestormoody/mcp-doctor/internal/diag"
)

// vscodeParse runs vscodeFile on testdata/vscode/parse/<fixture> as if it lived at path.
func vscodeParse(t *testing.T, fixture, path string, format Format) ([]Server, []diag.Finding) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "vscode", "parse", fixture)) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatal(err)
	}
	ss, fds := vscodeFile(Env{}, jsonFile(path, format, ScopeUser), src)
	parseCheckFindings(t, fds)
	for _, f := range fds {
		if f.Client != "vscode" {
			t.Errorf("finding client %q", f.Client)
		}
	}
	return ss, fds
}

func vscodeByName(ss []Server) map[string]Server {
	m := map[string]Server{}
	for _, s := range ss {
		m[s.Name] = s
	}
	return m
}

func vscodeRefs(rs []VarRef) string {
	var out []string
	for _, r := range rs {
		out = append(out, fmt.Sprintf("%s %s %s %t", r.Field, r.Kind, r.Name, r.HasDefault))
	}
	return strings.Join(out, "|")
}

func vscodeOnly(fds []diag.Finding, code string) []diag.Finding {
	var out []diag.Finding
	for _, f := range fds {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

func TestVSCodeParseMCPJSON(t *testing.T) {
	ss, fds := vscodeParse(t, "mcp.jsonc", "/u/Code/User/mcp.json", FormatJSONC)
	if len(ss) != 3 {
		t.Fatalf("servers = %d, want 3 (mcpServers ignored)", len(ss))
	}
	if got := []string{ss[0].Name, ss[1].Name, ss[2].Name}; strings.Join(got, ",") != "local-tool,remote,inferred" {
		t.Errorf("order = %v", got)
	}
	m := vscodeByName(ss)
	lt := m["local-tool"]
	if lt.Source.Client != "vscode" || lt.Source.Scope != ScopeUser || lt.Source.Format != FormatJSONC ||
		lt.Source.Position() != "/u/Code/User/mcp.json:7:5" {
		t.Errorf("source = %+v", lt.Source)
	}
	if lt.Transport != "stdio" || lt.TransportInferred || lt.Command != "node" || lt.Cwd != "${userHome}/work" ||
		lt.EnvFile != "${workspaceFolder}/.env" || lt.Shadowed || lt.Unknown != nil {
		t.Errorf("local-tool = %+v", lt)
	}
	if got := strings.Join(secrets(lt.Args), ","); got != "${workspaceFolder}/x,${env:HOME_DIR},${input:missing}" {
		t.Errorf("args = %q", got)
	}
	if secretMap(lt.Env)["K"] != "${input:api-key}" {
		t.Errorf("env = %v", secretMap(lt.Env))
	}
	wantRefs := "args[0] workspace workspaceFolder false|args[1] env HOME_DIR false|args[2] input missing false|" +
		"cwd userHome userHome false|env.K input api-key false|envFile workspace workspaceFolder false"
	if got := vscodeRefs(lt.Refs); got != wantRefs {
		t.Errorf("refs:\n got %s\nwant %s", got, wantRefs)
	}
	r := m["remote"]
	if r.Transport != "http" || r.TransportInferred || r.URL.Reveal() != "https://h.example/mcp/CANARY-p" ||
		secretMap(r.Headers)["Authorization"] != "Bearer CANARY-h" || r.Refs != nil {
		t.Errorf("remote = %+v", r)
	}
	if in := m["inferred"]; in.Transport != "stdio" || !in.TransportInferred || in.Command != "uvx" {
		t.Errorf("inferred = %+v", in)
	}
	if got := parseCodes(fds); got != "MCP_PARSE_009" {
		t.Fatalf("codes = %q", got)
	}
	if f := fds[0]; f.Message != `Undefined input reference "missing"` || f.Server != "local-tool" ||
		f.Source != "/u/Code/User/mcp.json:7:5" {
		t.Errorf("009 = %+v", f)
	}
}

func TestVSCodeRefsAllLeaves(t *testing.T) {
	src := `{"servers": {"s": {"command": "x",
  "sandbox": {"network": {"allow": ["${input:nope1}"]}},
  "oauth": {"clientId": "${input:nope2}"},
  "extra": {"deep": "${env:X}", "n": 3, "b": true, "z": null}}}}`
	ss, fds := vscodeFile(Env{}, jsonFile("/w/.vscode/mcp.json", FormatJSONC, ScopeProject), []byte(src))
	parseCheckFindings(t, fds)
	if len(ss) != 1 {
		t.Fatalf("servers = %d", len(ss))
	}
	want := "extra.deep env X false|oauth.clientId input nope2 false|sandbox.network.allow[0] input nope1 false"
	if got := vscodeRefs(ss[0].Refs); got != want {
		t.Errorf("refs:\n got %s\nwant %s", got, want)
	}
	if got := strings.Join(ss[0].Unknown, ","); got != "extra" {
		t.Errorf("unknown = %q", got)
	}
	if got := len(vscodeOnly(fds, "MCP_PARSE_009")); got != 2 {
		t.Errorf("009 count = %d (%s)", got, parseCodes(fds))
	}
}

func TestVSCodeInputsWrongType(t *testing.T) {
	src := `{"inputs": {"id": "a"}, "servers": {"s": {"command": "${input:a}"}}}`
	ss, fds := vscodeFile(Env{}, jsonFile("/u/mcp.json", FormatJSONC, ScopeUser), []byte(src))
	parseCheckFindings(t, fds)
	if got := parseCodes(fds); got != "MCP_PARSE_002" || len(ss) != 1 {
		t.Errorf("codes = %q, servers %d", got, len(ss))
	}
}

func TestVSCodeParsePortable(t *testing.T) {
	ss, fds := vscodeParse(t, "mcp-config.json", "/h/.copilot/mcp-config.json", FormatJSON)
	if len(ss) != 1 {
		t.Fatalf("servers = %d", len(ss))
	}
	s := ss[0]
	if s.Name != "copilot-local" || s.Transport != "stdio" || s.TransportInferred || s.Command != "npx" ||
		s.Cwd != "/srv/tool" || s.EnvFile != "" || strings.Join(s.Unknown, ",") != "envFile" {
		t.Errorf("server = %+v", s)
	}
	if got := vscodeRefs(s.Refs); got != "env.T env TOKEN false" {
		t.Errorf("refs = %s", got)
	}
	if got := parseCodes(fds); got != "MCP_PARSE_005" ||
		!strings.Contains(fds[0].Message, "not supported in this file") || !strings.Contains(fds[0].Message, "envFile") {
		t.Errorf("findings = %+v", fds)
	}

	ss, fds = vscodeParse(t, "dot-mcp.json", "/w/proj/.mcp.json", FormatJSON)
	if len(ss) != 1 || ss[0].Name != "shared" {
		t.Fatalf("servers = %+v", ss)
	}
	s = ss[0]
	if s.Cwd != "" || s.Transport != "stdio" || !s.TransportInferred || s.Command != "${BIN:-uvx}" {
		t.Errorf("server = %+v", s)
	}
	if got := vscodeRefs(s.Refs); got != "command env BIN true" {
		t.Errorf("refs = %s ($LITERAL must stay literal)", got)
	}
	if got := parseCodes(fds); got != "MCP_PARSE_005,MCP_PARSE_005" {
		t.Fatalf("codes = %q", got)
	}
	for _, f := range fds {
		if !strings.Contains(f.Message, "not supported in this file") {
			t.Errorf("005 without note: %+v", f)
		}
	}
	if fds[0].Source != "/w/proj/.mcp.json:2:3" || !strings.Contains(fds[0].Message, "inputs") || fds[0].Server != "" {
		t.Errorf("inputs 005 = %+v", fds[0])
	}
	if !strings.Contains(fds[1].Message, "cwd") || fds[1].Server != "shared" {
		t.Errorf("cwd 005 = %+v", fds[1])
	}

	_, fds = vscodeFile(Env{}, jsonFile("/w/proj/.mcp.json", FormatJSON, ScopeProject),
		[]byte("{\n // c\n \"mcpServers\": {}\n}"))
	if got := parseCodes(fds); got != "MCP_PARSE_006" {
		t.Errorf("comment in portable file: %q", got)
	}
}

func TestVSCodeParseIncomplete(t *testing.T) {
	ss, fds := vscodeParse(t, "incomplete.jsonc", "/u/mcp.json", FormatJSONC)
	m := vscodeByName(ss)
	if len(ss) != 2 || m["typo"].Command != "node" || m["ok"].Transport != "http" || !m["ok"].TransportInferred {
		t.Errorf("servers = %+v", ss)
	}
	if got := parseCodes(fds); got != "MCP_PARSE_008,MCP_PARSE_002,MCP_PARSE_005" {
		t.Fatalf("codes = %q", got)
	}
	if fds[0].Server != "no-url" || fds[1].Server != "bad-cmd" || fds[1].Source != "/u/mcp.json:4:29" {
		t.Errorf("positions = %+v", fds[:2])
	}
	if fds[2].Server != "typo" || !strings.Contains(fds[2].Message, "comand") || strings.Join(m["typo"].Unknown, ",") != "comand" {
		t.Errorf("005 = %+v", fds[2])
	}
}

func TestVSCodeParseCorrupted(t *testing.T) {
	ss, fds := vscodeParse(t, "corrupted.jsonc", "/u/mcp.json", FormatJSONC)
	if len(ss) != 0 || parseCodes(fds) != "MCP_PARSE_001" || !strings.HasPrefix(fds[0].Source, "/u/mcp.json:") {
		t.Errorf("servers %d, findings %+v", len(ss), fds)
	}
	ss, fds = vscodeFile(Env{}, jsonFile("/u/mcp.json", FormatJSONC, ScopeUser), []byte(`[]`))
	if len(ss) != 0 || parseCodes(fds) != "MCP_PARSE_002" {
		t.Errorf("non-object root: %d %+v", len(ss), fds)
	}
}

func TestVSCodeLoad(t *testing.T) {
	env, _ := fixtureEnv(t)
	dst := filepath.Join(userConfigDir(env), "Code", "User", "mcp.json")
	place(t, "testdata/vscode/parse/mcp.jsonc", dst)
	_, servers, _ := Load(env)
	var got []string
	for _, s := range servers {
		if s.Source.Client != "vscode" {
			continue
		}
		if s.Source.Scope != ScopeUser || s.Source.Path != dst || !filepath.IsAbs(s.Source.Path) {
			t.Errorf("source = %+v", s.Source)
		}
		got = append(got, s.Name)
	}
	if strings.Join(got, ",") != "local-tool,remote,inferred" {
		t.Errorf("vscode servers = %v", got)
	}
}

func TestVSCodeRawPreserved(t *testing.T) {
	src, err := os.ReadFile("testdata/vscode/parse/mcp.jsonc")
	if err != nil {
		t.Fatal(err)
	}
	ss, _ := vscodeFile(Env{}, jsonFile("/u/mcp.json", FormatJSONC, ScopeUser), src)
	for _, s := range ss {
		if s.raw == nil || !bytes.HasPrefix(*s.raw, []byte("{")) || !bytes.Contains(src, *s.raw) {
			t.Errorf("%s: raw not a slice of the file", s.Name)
		}
	}
	if r := vscodeByName(ss)["remote"]; r.raw == nil || !bytes.Contains(*r.raw, []byte("// inline comment")) {
		t.Error("remote raw lost its comment")
	}
}

func FuzzParseVSCode(f *testing.F) {
	for _, dir := range []string{"testdata/vscode/parse", "testdata/vscode"} {
		for _, b := range readFixtures(f, dir) {
			f.Add(b)
		}
	}
	files := []File{
		jsonFile("/u/mcp.json", FormatJSONC, ScopeUser),
		jsonFile("/h/.copilot/mcp-config.json", FormatJSON, ScopeUser),
		jsonFile("/w/.mcp.json", FormatJSON, ScopeProject),
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		body := stripBOM(input)
		for _, file := range files {
			ss, fds := vscodeFile(Env{}, file, input)
			for _, fd := range fds {
				if err := fd.Validate(); err != nil || fd.Client != "vscode" {
					t.Fatalf("finding %+v: %v", fd, err)
				}
			}
			for _, s := range ss {
				if s.Shadowed || s.raw == nil || !bytes.Contains(body, *s.raw) {
					t.Fatalf("server %q: shadowed %t or raw not in input", s.Name, s.Shadowed)
				}
			}
		}
	})
}

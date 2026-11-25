package config

// Edit spike (docs/decisions/0001-format-preserving-edits.md): proves the format-preserving edit approach
// for M07 on the pinned library versions. No production edit API lives in this package.

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/tailscale/hujson"
)

const spikeDir = "testdata/spike"

// crlfVariant converts LF line endings to CRLF.
func crlfVariant(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n")) }

// bomVariant prepends a UTF-8 byte order mark.
func bomVariant(b []byte) []byte { return append([]byte("\xEF\xBB\xBF"), b...) }

// variants returns b as written ("lf"), with CRLF endings and with a BOM.
func variants(b []byte) map[string][]byte {
	return map[string][]byte{"lf": b, "crlf": crlfVariant(b), "bom": bomVariant(b)}
}

// readCorpus reads every regular file in dir, keyed by base name.
func readCorpus(t testing.TB, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // G304: test reads its own fixture directory
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = b
	}
	if len(files) == 0 {
		t.Fatalf("no fixtures in %s", dir)
	}
	return files
}

// spikeFiles returns every fixture variant whose name has one of exts, keyed "file/variant".
func spikeFiles(t *testing.T, exts ...string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for name, b := range readCorpus(t, spikeDir) {
		if !slices.Contains(exts, filepath.Ext(name)) {
			continue
		}
		for v, vb := range variants(b) {
			out[name+"/"+v] = vb
		}
	}
	return out
}

func TestEditSpikeJSONNoOp(t *testing.T) {
	for name, src := range spikeFiles(t, ".json", ".jsonc") {
		body, hasBOM, nl := prepare(src)
		v, err := hujson.Parse(body)
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		if got := unprepare(v.Pack(), hasBOM, nl); !bytes.Equal(got, src) {
			t.Errorf("%s: no-op pack is not byte-identical", name)
		}
	}
}

func TestEditSpikeJSONReplace(t *testing.T) {
	src := readCorpus(t, spikeDir)["comments.jsonc"]
	for name, in := range map[string][]byte{"lf": src, "crlf": crlfVariant(src)} {
		v, err := hujson.Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		target := v.Find("/mcpServers/a/command")
		start, end := target.StartOffset, target.EndOffset
		edited := spikeCopy(t, in) // not Clone: see TestEditSpikeJSONLimits
		if err := edited.Patch([]byte(`[{"op":"replace","path":"/mcpServers/a/command","value":"y"}]`)); err != nil {
			t.Fatal(err)
		}
		out := edited.Pack()
		tail := len(in) - end
		if !bytes.Equal(out[:start], in[:start]) || !bytes.Equal(out[len(out)-tail:], in[end:]) {
			t.Errorf("%s: bytes outside [%d,%d) changed", name, start, end)
		}
		if got := string(out[start : len(out)-tail]); got != `"y"` {
			t.Errorf("%s: replaced value = %q", name, got)
		}
		if !bytes.Contains(out, []byte("/* first server */")) || (name == "crlf" && bytes.Count(out, []byte("\r\n")) != bytes.Count(in, []byte("\r\n"))) {
			t.Errorf("%s: comments or CRLF lost", name)
		}
	}
}

// spikeCopy parses b again: an independent AST, the working copy M07 patches instead of a Clone.
func spikeCopy(t *testing.T, b []byte) hujson.Value {
	t.Helper()
	v, err := hujson.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestEditSpikeJSONLimits records the hujson limits M07 must work around.
func TestEditSpikeJSONLimits(t *testing.T) {
	// Clone turns the empty-but-present AfterExtra that marks a trailing comma into nil, dropping the comma.
	jsonc, _, _ := prepare(readCorpus(t, spikeDir)["comments.jsonc"])
	if c := spikeCopy(t, jsonc).Clone(); bytes.Equal(c.Pack(), jsonc) {
		t.Error("Clone now keeps trailing commas; the decision record can switch back to Clone")
	}

	body, _, _ := prepare(readCorpus(t, spikeDir)["plain.json"])
	v := spikeCopy(t, body)

	added := spikeCopy(t, body)
	if err := added.Patch([]byte(`[{"op":"add","path":"/mcpServers/c","value":{"command":"z"}}]`)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(added.Pack(), []byte(`"c":{"command":"z"}`)) {
		t.Errorf("add no longer inserts compact JSON; update the decision record:\n%s", added.Pack())
	}

	failed := spikeCopy(t, body)
	err := failed.Patch([]byte(`[{"op":"replace","path":"/mcpServers/a/command","value":"y"},{"op":"remove","path":"/nope"}]`))
	if err == nil {
		t.Fatal("patch with a missing path should fail")
	}
	if !bytes.Equal(v.Pack(), body) {
		t.Error("failed patch on a copy changed the original")
	}
	if bytes.Equal(failed.Pack(), body) {
		t.Error("failed patch no longer leaves a partial mutation; update the decision record")
	}
}

// tomlServer is the server name present in each TOML spike fixture.
var tomlServer = map[string]string{"table.toml": "a", "inline.toml": "a", "dotted.toml": "a", "quoted.toml": "my.server"}

// tomlSpan returns the byte span of mcp_servers.<name>: from its header to the next header (or EOF), or
// from the first to the last key-value under that path for inline and dotted shapes.
func tomlSpan(t *testing.T, keys []tomlKey, name string, size int) (start, end int) {
	t.Helper()
	want := []string{"mcp_servers", name}
	start, end = -1, -1
	for i, k := range keys {
		if k.Header && slices.Equal(k.Path, want) {
			end = size
			for _, n := range keys[i+1:] {
				if n.Header {
					end = n.Start
					break
				}
			}
			return k.Start, end
		}
		if !k.Header && len(k.Path) >= 2 && slices.Equal(k.Path[:2], want) {
			if start < 0 {
				start = k.Start
			}
			end = max(end, k.End)
		}
	}
	if start < 0 {
		t.Fatalf("no span for mcp_servers.%s", name)
	}
	return start, end
}

func TestEditSpikeTOMLNoOp(t *testing.T) {
	for name, src := range spikeFiles(t, ".toml") {
		server := tomlServer[strings.Split(name, "/")[0]]
		body, hasBOM, nl := prepare(src)
		keys, err := tomlKeys(body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		start, end := tomlSpan(t, keys, server, len(body))
		span := body[start:end]
		splice := func(repl []byte) []byte {
			return slices.Concat(body[:start], repl, body[end:])
		}

		if got := unprepare(splice(span), hasBOM, nl); !bytes.Equal(got, src) {
			t.Errorf("%s: no-op splice is not byte-identical", name)
		}

		repl := bytes.Replace(span, []byte(`"x"`), []byte(`"y"`), 1)
		if bytes.Equal(repl, span) {
			t.Fatalf("%s: span %q has no command to change", name, span)
		}
		out := splice(repl)
		if !bytes.Equal(out[:start], body[:start]) || !bytes.Equal(out[start+len(repl):], body[end:]) {
			t.Errorf("%s: bytes outside the span changed", name)
		}
		if hasBOM && !bytes.HasPrefix(unprepare(out, hasBOM, nl), []byte("\xEF\xBB\xBF")) {
			t.Errorf("%s: BOM not restored", name)
		}
		var doc map[string]any
		if err := toml.Unmarshal(out, &doc); err != nil {
			t.Fatalf("%s: edited file does not parse: %v", name, err)
		}
		servers, _ := doc["mcp_servers"].(map[string]any)
		entry, _ := servers[server].(map[string]any)
		if entry["command"] != "y" {
			t.Errorf("%s: command = %v, want y", name, entry["command"])
		}
	}
}

func TestTOMLKeys(t *testing.T) {
	corpus := readCorpus(t, spikeDir)
	paths := func(t *testing.T, src []byte) ([]tomlKey, [][]string) {
		t.Helper()
		keys, err := tomlKeys(src)
		if err != nil {
			t.Fatal(err)
		}
		var ps [][]string
		for _, k := range keys {
			ps = append(ps, k.Path)
		}
		return keys, ps
	}
	has := func(ps [][]string, want ...string) bool {
		return slices.ContainsFunc(ps, func(p []string) bool { return slices.Equal(p, want) })
	}

	t.Run("table", func(t *testing.T) {
		src := corpus["table.toml"]
		keys, ps := paths(t, src)
		want := [][]string{
			{"model"},
			{"mcp_servers", "a"}, {"mcp_servers", "a", "command"}, {"mcp_servers", "a", "args"},
			{"mcp_servers", "b"}, {"mcp_servers", "b", "url"},
			{"mcp_servers", "a", "env"}, {"mcp_servers", "a", "env", "KEY"},
		}
		if !slices.EqualFunc(ps, want, slices.Equal) {
			t.Fatalf("paths = %v, want %v", ps, want)
		}
		if !keys[1].Header || keys[2].Header || src[keys[1].Start] != '[' || string(src[keys[1].Start:keys[1].End]) != "[mcp_servers.a]\n" {
			t.Errorf("header key = %+v", keys[1])
		}
		if string(src[keys[2].Offset:keys[2].Offset+7]) != "command" || string(src[keys[2].Start:keys[2].End]) != `command = "x"` {
			t.Errorf("key-value = %+v", keys[2])
		}
	})
	t.Run("inline", func(t *testing.T) {
		_, ps := paths(t, corpus["inline.toml"])
		if !has(ps, "mcp_servers", "a") || !has(ps, "mcp_servers", "a", "command") {
			t.Errorf("paths = %v", ps)
		}
	})
	t.Run("quoted name is one part", func(t *testing.T) {
		_, ps := paths(t, corpus["quoted.toml"])
		if !has(ps, "mcp_servers", "my.server") {
			t.Errorf("paths = %v", ps)
		}
	})
	t.Run("comment-only line", func(t *testing.T) {
		_, ps := paths(t, []byte("# only a comment\n# another\n"))
		if len(ps) != 0 {
			t.Errorf("paths = %v", ps)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		if _, err := tomlKeys([]byte("[mcp_servers.a\ncommand = \n")); err == nil {
			t.Error("want an error")
		}
	})
}

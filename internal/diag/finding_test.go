package diag

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestRegistryValid(t *testing.T) {
	if err := checkRegistry(registry); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRegistry(t *testing.T) {
	ok := CodeInfo{Code: "MCP_CONFIG_001", Category: "config", Title: "t", DefaultSeverity: SeverityError}
	with := func(f func(*CodeInfo)) CodeInfo { c := ok; f(&c); return c }
	cases := []struct {
		name    string
		entries []CodeInfo
		wantErr bool
	}{
		{"valid", []CodeInfo{ok, with(func(c *CodeInfo) { c.Code = "MCP_CONFIG_002" })}, false},
		{"empty", nil, false},
		{"duplicate", []CodeInfo{ok, ok}, true},
		{"lowercase", []CodeInfo{with(func(c *CodeInfo) { c.Code = "mcp_config_001" })}, true},
		{"short number", []CodeInfo{with(func(c *CodeInfo) { c.Code = "MCP_CONFIG_01" })}, true},
		{"no prefix", []CodeInfo{with(func(c *CodeInfo) { c.Code = "CONFIG_001" })}, true},
		{"bad severity", []CodeInfo{with(func(c *CodeInfo) { c.DefaultSeverity = "fatal" })}, true},
		{"empty category", []CodeInfo{with(func(c *CodeInfo) { c.Category = "" })}, true},
		{"empty title", []CodeInfo{with(func(c *CodeInfo) { c.Title = "" })}, true},
	}
	for _, c := range cases {
		if err := checkRegistry(c.entries); (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}

// TestRegistryGolden keeps the registry append-only: codes.txt records every code ever shipped.
func TestRegistryGolden(t *testing.T) {
	f, err := os.Open("testdata/codes.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	recorded := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		recorded[line] = true
		if _, ok := Lookup(line); !ok {
			t.Errorf("code %s is in testdata/codes.txt but not in the registry (codes are append-only)", line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	for _, c := range registry {
		if !recorded[c.Code] {
			t.Errorf("code %s is in the registry but not recorded in testdata/codes.txt", c.Code)
		}
	}
}

func TestSeverityValid(t *testing.T) {
	for _, s := range []Severity{SeverityPass, SeverityInfo, SeverityWarning, SeverityError, SeverityCritical, SeveritySkipped} {
		if !s.Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []Severity{"", "fatal"} {
		if s.Valid() {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestFindingValidate(t *testing.T) {
	saved := registry
	registry = []CodeInfo{{Code: "MCP_CONFIG_001", Category: "config", Title: "t", DefaultSeverity: SeverityError}}
	t.Cleanup(func() { registry = saved })

	ok := Finding{Code: "MCP_CONFIG_001", Category: "config", Severity: SeverityError, Message: "m"}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid finding: %v", err)
	}
	bad := map[string]Finding{
		"unknown code":      {Code: "MCP_CONFIG_999", Category: "config", Severity: SeverityError},
		"category mismatch": {Code: "MCP_CONFIG_001", Category: "runtime", Severity: SeverityError},
		"invalid severity":  {Code: "MCP_CONFIG_001", Category: "config", Severity: "fatal"},
	}
	for name, f := range bad {
		if err := f.Validate(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

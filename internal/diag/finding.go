package diag

import (
	"errors"
	"fmt"
	"regexp"
)

// Severity is the seriousness of a Finding.
type Severity string

// The six severities a Finding may carry.
const (
	SeverityPass     Severity = "pass"
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
	SeveritySkipped  Severity = "skipped"
)

// Valid reports whether s is one of the six defined severities.
func (s Severity) Valid() bool {
	switch s {
	case SeverityPass, SeverityInfo, SeverityWarning, SeverityError, SeverityCritical, SeveritySkipped:
		return true
	}
	return false
}

// CodeInfo describes one registered finding code.
type CodeInfo struct {
	Code            string
	Category        string
	Title           string
	DefaultSeverity Severity
}

// Finding is one diagnostic result.
type Finding struct {
	Code       string   `json:"code"`
	Category   string   `json:"category"`
	Severity   Severity `json:"severity"`
	Message    string   `json:"message"`
	Client     string   `json:"client,omitempty"`
	Server     string   `json:"server,omitempty"`
	Source     string   `json:"source,omitempty"` // file[:line:col]; may become a struct before the JSON schema is frozen
	Resolution string   `json:"resolution,omitempty"`
	Fixable    bool     `json:"fixable"`
}

// registry is the single source of truth for finding codes. Append-only: never remove, renumber or reuse a code; record each new code in testdata/codes.txt.
var registry = []CodeInfo{
	{Code: "MCP_DISC_001", Category: "discovery", Title: "Config file cannot be read", DefaultSeverity: SeverityWarning},
	{Code: "MCP_DISC_002", Category: "discovery", Title: "Config location cannot be resolved", DefaultSeverity: SeverityWarning},
	{Code: "MCP_PARSE_001", Category: "parse", Title: "Config file has a syntax error", DefaultSeverity: SeverityError},
	{Code: "MCP_PARSE_002", Category: "parse", Title: "Config file has an invalid structure", DefaultSeverity: SeverityError},
	{Code: "MCP_PARSE_003", Category: "parse", Title: "Server has neither a command nor a URL", DefaultSeverity: SeverityError},
	{Code: "MCP_PARSE_004", Category: "parse", Title: "Duplicate key", DefaultSeverity: SeverityWarning},
	{Code: "MCP_PARSE_005", Category: "parse", Title: "Unknown or unsupported field in a server entry", DefaultSeverity: SeverityInfo},
	{Code: "MCP_PARSE_006", Category: "parse", Title: "Comments or trailing commas in a plain JSON file", DefaultSeverity: SeverityWarning},
	{Code: "MCP_PARSE_007", Category: "parse", Title: "Config file too large to parse", DefaultSeverity: SeverityWarning},
	{Code: "MCP_PARSE_008", Category: "parse", Title: "Transport type conflicts with the fields", DefaultSeverity: SeverityError},
	{Code: "MCP_PARSE_009", Category: "parse", Title: "Undefined input reference", DefaultSeverity: SeverityWarning},
	{Code: "MCP_PARSE_010", Category: "parse", Title: "File starts with a byte order mark", DefaultSeverity: SeverityInfo},
}

var codePattern = regexp.MustCompile(`^MCP_[A-Z]+_[0-9]{3}$`)

// Lookup returns the registry entry for code.
func Lookup(code string) (CodeInfo, bool) {
	for _, c := range registry {
		if c.Code == code {
			return c, true
		}
	}
	return CodeInfo{}, false
}

// checkRegistry rejects duplicate or malformed codes and incomplete entries.
func checkRegistry(entries []CodeInfo) error {
	seen := map[string]bool{}
	var errs []error
	for _, c := range entries {
		if seen[c.Code] {
			errs = append(errs, fmt.Errorf("duplicate code %q", c.Code))
		}
		seen[c.Code] = true
		if !codePattern.MatchString(c.Code) {
			errs = append(errs, fmt.Errorf("code %q does not match %s", c.Code, codePattern))
		}
		if !c.DefaultSeverity.Valid() {
			errs = append(errs, fmt.Errorf("code %q has invalid default severity %q", c.Code, c.DefaultSeverity))
		}
		if c.Category == "" || c.Title == "" {
			errs = append(errs, fmt.Errorf("code %q needs a category and a title", c.Code))
		}
	}
	return errors.Join(errs...)
}

// Validate checks f against the registry.
func (f Finding) Validate() error {
	info, ok := Lookup(f.Code)
	if !ok {
		return fmt.Errorf("unknown finding code %q", f.Code)
	}
	if f.Category != info.Category {
		return fmt.Errorf("finding code %q has category %q, registry says %q", f.Code, f.Category, info.Category)
	}
	if !f.Severity.Valid() {
		return fmt.Errorf("finding code %q has invalid severity %q", f.Code, f.Severity)
	}
	return nil
}

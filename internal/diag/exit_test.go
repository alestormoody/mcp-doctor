package diag

import (
	"os"
	"strings"
	"testing"
)

func TestExitCode(t *testing.T) {
	f := func(s Severity) Finding { return Finding{Severity: s} }
	cases := []struct {
		name     string
		findings []Finding
		want     int
	}{
		{"nil", nil, ExitOK},
		{"no problems", []Finding{f(SeverityPass), f(SeverityInfo), f(SeverityWarning), f(SeveritySkipped)}, ExitOK},
		{"error", []Finding{f(SeverityPass), f(SeverityError)}, ExitProblems},
		{"critical", []Finding{f(SeverityCritical), f(SeverityInfo)}, ExitProblems},
	}
	for _, c := range cases {
		if got := ExitCode(c.findings); got != c.want {
			t.Errorf("%s: ExitCode = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestExitCodesDocumented(t *testing.T) {
	b, err := os.ReadFile("../../docs/exit-codes.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []string{"| 0 |", "| 1 |", "| 2 |"} {
		if !strings.Contains(string(b), row) {
			t.Errorf("docs/exit-codes.md has no row %q", row)
		}
	}
}

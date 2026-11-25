package diag

// Process exit codes, documented in docs/exit-codes.md.
const (
	ExitOK       = 0 // no problems found
	ExitProblems = 1 // at least one ERROR or CRITICAL finding
	ExitError    = 2 // tool or usage error
)

// ExitCode returns ExitProblems if any finding is ERROR or CRITICAL, else ExitOK.
func ExitCode(findings []Finding) int {
	for _, f := range findings {
		if f.Severity == SeverityError || f.Severity == SeverityCritical {
			return ExitProblems
		}
	}
	return ExitOK
}

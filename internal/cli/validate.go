package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/alestormoody/mcp-doctor/internal/clients"
	"github.com/alestormoody/mcp-doctor/internal/diag"
)

func newValidateCmd(exit *int) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check every client's MCP config files for syntax and structure problems",
		Long: "Check the MCP config files of every supported AI client for syntax and structure problems,\n" +
			"and report each one with its file position. Values from the files are never shown.\n\n" +
			"Read-only: files are read, never changed, and no server is started.\n" +
			"More checks arrive in later releases.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			results, servers, findings := clients.Load(discoverEnv())
			*exit = diag.ExitCode(findings)
			return renderValidate(cmd.OutOrStdout(), results, servers, findings)
		},
	}
}

func renderValidate(out io.Writer, results []clients.Result, servers []clients.Server, findings []diag.Finding) error {
	if err := writeFindings(out, findings); err != nil {
		return err
	}
	files := map[string]bool{} // .mcp.json is listed by two clients but is one file
	for _, r := range results {
		for _, f := range r.Files {
			if f.Status == clients.StatusFound {
				files[f.Path] = true
			}
		}
	}
	var errs, warns int
	for _, f := range findings {
		switch f.Severity {
		case diag.SeverityError, diag.SeverityCritical:
			errs++
		case diag.SeverityWarning:
			warns++
		case diag.SeverityPass, diag.SeverityInfo, diag.SeveritySkipped:
		}
	}
	_, err := fmt.Fprintf(out, "%s checked, %s, %s, %s\n", plural(len(files), "file"), plural(len(servers), "server"),
		plural(errs, "error"), plural(warns, "warning"))
	return err
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

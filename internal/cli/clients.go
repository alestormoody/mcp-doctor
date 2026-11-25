package cli

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/alestormoody/mcp-doctor/internal/clients"
	"github.com/alestormoody/mcp-doctor/internal/diag"
)

// discoverEnv is the test seam: tests replace it to point discovery at temp dirs.
var discoverEnv = osEnv

// osEnv is the only place the real filesystem and environment are wired into discovery.
// An unresolvable home or working directory becomes "" and is reported as MCP_DISC_002.
func osEnv() clients.Env {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	return clients.Env{
		GOOS: runtime.GOOS, Home: home, Cwd: cwd,
		Getenv: os.Getenv, Stat: os.Stat, Lstat: os.Lstat, ReadDir: os.ReadDir,
		Open: func(p string) (io.Closer, error) {
			f, err := os.Open(p) //nolint:gosec // G304: path comes from the fixed candidate list
			if err != nil {
				return nil, err // avoid a typed-nil io.Closer
			}
			return f, nil
		},
		ReadFile: func(p string) ([]byte, error) {
			f, err := os.Open(p) //nolint:gosec // G304: path comes from the fixed candidate list
			if err != nil {
				return nil, err
			}
			defer func() { _ = f.Close() }()
			// One byte over the cap lets Load tell "too large" from "exactly at the cap".
			return io.ReadAll(io.LimitReader(f, clients.MaxFileSize+1))
		},
	}
}

func newClientsCmd(exit *int) *cobra.Command {
	return &cobra.Command{
		Use:   "clients",
		Short: "List supported AI clients and where their MCP config files are",
		Long: "List supported AI clients and every location where each may keep an MCP config file,\n" +
			"with whether the file is found, missing or unreadable.\n\n" +
			"Read-only: files are checked for existence and read access, never parsed or changed.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			results, findings := clients.Discover(discoverEnv())
			*exit = diag.ExitCode(findings)
			return renderClients(cmd.OutOrStdout(), results, findings)
		},
	}
}

func renderClients(out io.Writer, results []clients.Result, findings []diag.Finding) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for i, r := range results {
		if i > 0 {
			_, _ = fmt.Fprintln(tw)
		}
		state := "not detected"
		if r.Detected {
			state = "detected"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", r.Name, state)
		for _, f := range r.Files {
			line := fmt.Sprintf("  %s\t%s\t%s", f.Scope, f.Path, f.Status)
			if note := strings.TrimSpace(f.Label + " " + f.Reason); note != "" {
				line += "\t" + note
			}
			_, _ = fmt.Fprintln(tw, line)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return renderFindings(out, findings)
}

// renderFindings prints a blank line, then one line per finding; nothing when there are none.
// renderFindings prints findings after a table, separated by a blank line.
func renderFindings(out io.Writer, findings []diag.Finding) error {
	if len(findings) > 0 {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	return writeFindings(out, findings)
}

// writeFindings prints one line per finding.
func writeFindings(out io.Writer, findings []diag.Finding) error {
	for _, f := range findings {
		sev := strings.ToUpper(string(f.Severity))
		var err error
		if f.Source == "" {
			_, err = fmt.Fprintf(out, "%s %s: %s\n", sev, f.Code, f.Message)
		} else {
			_, err = fmt.Fprintf(out, "%s %s %s: %s\n", sev, f.Code, f.Source, f.Message)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Package cli wires the mcp-doctor command line.
package cli

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/alestormoody/mcp-doctor/internal/diag"
)

// Set at build time via -ldflags -X github.com/alestormoody/mcp-doctor/internal/cli.version=... (and .commit, .date).
var version, commit, date = "dev", "none", "unknown"

// Execute runs the command line with args and returns the process exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	// Logs go to stderr only; stdout is reserved for results.
	slog.SetDefault(slog.New(slog.NewTextHandler(stderr, nil)))

	root := &cobra.Command{
		Use:           "mcp-doctor",
		Short:         "Diagnose MCP server configurations across AI clients",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	exit := diag.ExitOK
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "MCP Doctor %s\ncommit: %s\nbuilt: %s\nhttps://github.com/alestormoody/mcp-doctor\n", version, commit, date)
			return err
		},
	})
	root.AddCommand(newClientsCmd(&exit))

	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\nRun 'mcp-doctor --help' for usage.\n", err)
		return diag.ExitError
	}
	return exit
}

package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/alestormoody/mcp-doctor/internal/clients"
	"github.com/alestormoody/mcp-doctor/internal/diag"
)

func newServersCmd(exit *int) *cobra.Command {
	return &cobra.Command{
		Use:   "servers",
		Short: "List MCP servers from every client's config files",
		Long: "List every MCP server defined in the config files of the supported AI clients, with its client,\n" +
			"scope, transport, command or URL, and the file position where it is defined.\n\n" +
			"Read-only: files are read, never changed, and no server is started. Values of env, headers\n" +
			"and args are never shown; URLs are shown only as scheme://host[:port].",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			results, servers, findings := clients.Load(discoverEnv())
			*exit = diag.ExitCode(findings)
			return renderServers(cmd.OutOrStdout(), results, servers, findings)
		},
	}
}

func renderServers(out io.Writer, results []clients.Result, servers []clients.Server, findings []diag.Finding) error {
	if len(servers) == 0 {
		if _, err := fmt.Fprintln(out, "No MCP servers found."); err != nil {
			return err
		}
		return renderFindings(out, findings)
	}
	names := map[string]string{}
	for _, r := range results {
		names[r.ID] = r.Name
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CLIENT\tSCOPE\tNAME\tTRANSPORT\tTARGET\tNOTE\tSOURCE")
	for _, s := range servers {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", names[s.Source.Client], s.Source.Scope, s.Name,
			transport(s), target(s), note(s), s.Source.Position())
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return renderFindings(out, findings)
}

func transport(s clients.Server) string {
	switch {
	case s.Transport == "":
		return "-"
	case s.TransportInferred:
		return s.Transport + " (inferred)"
	}
	return s.Transport
}

// target is the command for local servers and the redacted URL for remote ones; never args or env.
func target(s clients.Server) string {
	t := s.Command
	switch s.Transport {
	case "http", "sse", "ws":
		t = ""
		if u := s.URL.Reveal(); u != "" {
			t = clients.RedactURL(u)
		}
	}
	if t == "" {
		return "-"
	}
	return t
}

func note(s clients.Server) string {
	var parts []string
	if s.Disabled {
		parts = append(parts, "disabled")
	}
	if s.Shadowed {
		parts = append(parts, "shadowed")
	}
	if s.Source.Label != "" {
		parts = append(parts, s.Source.Label)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "; ")
}

// Command mcp-doctor diagnoses MCP server configurations across AI clients.
package main

import (
	"os"

	"github.com/alestormoody/mcp-doctor/internal/cli"
)

func main() { os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr)) }

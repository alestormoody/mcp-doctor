# Security

To report a vulnerability, see [SECURITY.md](../SECURITY.md).

## What the tool touches

MCP Doctor will read the MCP configuration files of AI clients. Those files often contain
secrets such as tokens and API keys. Today the tool reads nothing: only
`mcp-doctor version` exists.

## Controls in place

- `diag.Secret` redacts sensitive values in every output format; `Reveal()` is the only
  accessor.
- Logs go to stderr only.
- GitHub Actions are pinned by full commit SHA.
- govulncheck runs on every pull request.
- gitleaks scans the full git history on every pull request.
- Dependabot updates Go modules and GitHub Actions weekly.
- Workflows run with least-privilege permissions.
- The `main` branch is protected.

## Planned controls

Planned, not implemented yet:

- `scan` never starts MCP servers.
- `test` asks for consent before starting a server or connecting to an endpoint.
- Backups are written with `0600` permissions.

# Security

To report a vulnerability, see [SECURITY.md](../SECURITY.md).

## What the tool touches

MCP Doctor reads the MCP configuration files of AI clients. Those files often contain
secrets such as tokens and API keys.

- Config files are read (at most 64 MiB each; larger files are skipped with a finding) and
  never written.
- Values that can hold credentials (arguments, environment values, header values, URLs) are
  kept as `diag.Secret` from the moment they are decoded and are never printed.
- URLs are shown as `scheme://host[:port]` only; any path, query, fragment or user info is
  replaced by `/…`.
- Parser error texts are never passed through, because they can quote file content: findings
  carry the position and a fixed message.
- Variable references such as `${API_KEY}` are recorded by name and never expanded.
- No MCP server is started.

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

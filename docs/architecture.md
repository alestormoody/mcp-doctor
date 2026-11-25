# Architecture

MCP Doctor is one Go module that builds one binary, `mcp-doctor`.

## Packages today

| Package | Purpose |
| ------- | ------- |
| `cmd/mcp-doctor` | Entry point. Calls `cli.Execute` and exits with its code. |
| `internal/cli` | Cobra commands. Only `version` exists. |
| `internal/diag` | Diagnostic core: `Secret`, `Finding`, `Severity`, the finding-code registry and exit codes. |

## Planned layering

Planned, not implemented yet. Packages are added in this order, each one only in the
milestone that fills it with code:

```text
cli        (commands, output)
  repair   (consent-gated fixes)
  backup   (atomic writes, backups, restore)
  probe    (consent-gated protocol tests)
  checks   (static checks that produce findings)
  clients  (per-client discovery and config locations)
  config   (JSON, JSONC and TOML parsing)
  model    (normalized server model with provenance)
diag       (findings, codes, secrets, exit codes; used by every layer)
```

Dependencies point one way: a package may import packages below it, never above it.
There are no `utils` or `common` packages, and no empty packages.

## Data flow

Planned: discovery finds each client's configuration files, the parser turns them into a
normalized model that remembers where every value came from, checks turn the model into
findings, and the CLI renders the findings and picks the exit code. Repair and backup act
only on explicit consent.

## Output streams

Results go to stdout. Logs (`log/slog`) and errors go to stderr. This keeps stdout safe to
pipe and parse.

## Secrets

Any value from a configuration that may be sensitive is wrapped in `diag.Secret` as soon as
it is read. `Secret` redacts itself in `fmt` verbs, `String`, `GoString`, JSON, text
marshalling and `slog`. `Reveal()` is the only accessor that returns the real value, so every
use of a secret is explicit and easy to review.

## Findings and codes

A `Finding` carries a code, category, severity, message, client, server, source,
resolution and whether it is fixable. Severities are `PASS`, `INFO`, `WARNING`, `ERROR`,
`CRITICAL` and `SKIPPED`.

Codes have the form `MCP_<AREA>_<NNN>` and live in one registry. The registry is
append-only: a code is never reused, renumbered or removed. Tests fail on duplicate,
malformed or removed codes.

Exit codes are documented in [exit-codes.md](exit-codes.md).

## Dependency ledger

| Module | Version | Why |
| ------ | ------- | --- |
| `github.com/spf13/cobra` | v1.10.2 | CLI framework: commands, flags and help. |

The standard library comes first. A new row needs a justification in the pull request that
adds it.

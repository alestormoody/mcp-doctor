# Architecture

MCP Doctor is one Go module that builds one binary, `mcp-doctor`.

## Packages today

| Package | Purpose |
| ------- | ------- |
| `cmd/mcp-doctor` | Entry point. Calls `cli.Execute` and exits with its code. |
| `internal/cli` | Cobra commands: `version`, `clients`, `servers` and `validate`. |
| `internal/clients` | Read-only discovery of each client's config file locations and mapping of each client's schema onto the normalized `Server` model with provenance (one file per client). Reads files only through an injected, size-bounded `ReadFile`; never writes. |
| `internal/config` | Format layer: JSON and JSONC via hujson, TOML via go-toml/v2, with byte positions turned into line and column. Parser error texts are replaced by fixed messages. No client knowledge and no filesystem access: it parses bytes it is given. |
| `internal/diag` | Diagnostic core: `Secret`, `Finding`, `Severity`, the finding-code registry and exit codes. |

## Planned layering

Only `cli`, `clients`, `config` and `diag` exist today. Packages are added in this order, each one only in the
milestone that fills it with code:

```text
cli        (commands, output)
  repair   (consent-gated fixes)
  backup   (atomic writes, backups, restore)
  probe    (consent-gated protocol tests)
  checks   (static checks that produce findings)
  clients  (per-client discovery, schema mapping and the normalized server model) (present)
  config   (JSON, JSONC and TOML parsing) (present)
diag       (findings, codes, secrets, exit codes; used by every layer)
```

Dependencies point one way: a package may import packages below it, never above it.
There are no `utils` or `common` packages, and no empty packages.

## Data flow

Today: `clients.Discover` finds each client's configuration files; `clients.Load` reads every
found file once per client that lists it, parses it with `internal/config` and maps it onto
`Server` values that remember client, file, scope, line and column. `servers` prints the
servers, `validate` prints the findings; both pick the exit code from the findings.

Planned: checks turn the model into more findings. Repair and backup act only on explicit
consent; how edits will keep untouched bytes identical is recorded in
[decision 0001](decisions/0001-format-preserving-edits.md).

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
| `github.com/tailscale/hujson` | v0.0.0-20260727124030-b80ff77dac4f (untagged, pinned commit b80ff77dac4f) | BSD-3-Clause. JSONC (comments, trailing commas) with byte offsets and a byte-identical round trip; the standard library cannot parse comments. |
| `github.com/pelletier/go-toml/v2` | v2.4.3 | MIT. TOML with positioned errors; its `unstable` parser gives key positions and is used from one file only (`internal/config/toml.go`). The standard library has no TOML. |

The standard library comes first. A new row needs a justification in the pull request that
adds it.

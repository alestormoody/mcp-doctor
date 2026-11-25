# Testing

## Local gate

```sh
make check
```

`make check` runs `vet`, `build`, `test`, `race`, `lint` and `vuln`. See
[CONTRIBUTING.md](../CONTRIBUTING.md) for the individual targets.

## CI gates

Every pull request runs:

| Gate | Where |
| ---- | ----- |
| `go build ./...` | all jobs |
| `go vet ./...` | all jobs |
| `go test ./...` | all jobs |
| `go test -race ./...` | Linux, macOS and Windows |
| golangci-lint v2.15.0 | CI |
| govulncheck v1.10.0 | CI |
| gitleaks v8.30.1 | once, over the full git history |
| GoReleaser snapshot build | pull requests only, nothing is published |

The test matrix is the latest stable Go on `ubuntu-latest`, `macos-latest` and
`windows-latest`, plus Go 1.26.x (the minimum supported version) on `ubuntu-latest`.
The aggregate check `ci-ok` passes only when every gate passes.

All GitHub Actions are pinned by full commit SHA and updated weekly by Dependabot.

## Race detector on Windows

`-race` needs cgo. On Windows CI sets `CGO_ENABLED=1` and uses the MinGW `gcc` that is
preinstalled on the runner. To run `make race` locally on Windows you need a `gcc` in
`PATH`.

## Test style

- Standard library `testing` only.
- Table-driven tests where there is more than one case.
- Temporary files go in `t.TempDir()`.
- Build paths with `path/filepath`, never with hard-coded separators.
- Test fixtures use LF line endings (enforced by `.gitattributes`).

## Secret canary rule

Every output path must be proven free of secrets: a test puts a known canary value into a
`diag.Secret`, sends it through the output, and fails if the canary appears.
`internal/diag/secret_test.go` does this for `fmt`, JSON, text and `slog`.

The rule extends to config files: fixtures put canary strings in every value position
(environment, headers, arguments, URL user info, path and query, OAuth secrets, variable
defaults) and in syntax-error positions, and the tests fail if any canary appears in a
finding or in the output of `servers` and `validate`.

## Fuzzing

Every parser has a fuzz target. They run as normal tests over their seed corpus in CI; run
one for longer locally with:

```sh
go test ./internal/config -run='^$' -fuzz='^FuzzParseJSON$' -fuzztime=60s
```

| Package | Targets |
| ------- | ------- |
| `internal/config` | `FuzzParseJSON`, `FuzzParseTOML` |
| `internal/clients` | `FuzzParseClaudeCode`, `FuzzParseCodex`, `FuzzParseClaudeDesktop`, `FuzzParseCursor`, `FuzzParseVSCode` |

Replace the package and the target name for the others. A crasher found locally goes into
`testdata/fuzz/<Target>/` as a permanent seed.

## Read-only proof

`internal/clients/readonly_test.go` snapshots the whole fixture tree before and after
discovery and parsing (also with corrupted files) and fails on any change; records every
filesystem call and fails on a path outside the documented locations or a file read more
than once per client; and statically rejects write APIs in `internal/clients` and
`internal/config`.

## Coverage targets

| Component | Target coverage |
| --------- | --------------- |
| Policy engine | 95% |
| Security-critical logic | 95% |
| Configuration parsers | 90% |
| Diagnostic engine | 90% |
| Repair engine | 90% |
| Client adapters | 80% |
| Utilities | 85% |
| CLI | 70% |
| Whole repository | 80% |

Coverage is not proof of correctness or security. Critical components need behavioural,
negative and bypass tests whatever percentage they reach.

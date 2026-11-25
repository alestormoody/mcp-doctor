# Contributing to MCP Doctor

Thank you for your interest in MCP Doctor. The project is pre-alpha: read
[docs/roadmap.md](docs/roadmap.md) to see which milestone is in progress.

## Prerequisites

- Go 1.26 or later
- golangci-lint v2.15.0
- GNU Make
- On Windows, a `gcc` in `PATH` (for example MinGW) to run the race detector locally

## Local checks

Run the full local gate before opening a pull request:

```sh
make check
```

`make check` runs all of these targets, which you can also run one by one:

| Target | What it runs |
| ------ | ------------ |
| `make build` | `go build ./...` |
| `make test` | `go test ./...` |
| `make race` | `go test -race ./...` |
| `make vet` | `go vet ./...` |
| `make lint` | `golangci-lint run` |
| `make vuln` | `govulncheck ./...` (v1.10.0, via `go run`) |

See [docs/testing.md](docs/testing.md) for the CI gates and test style.

## Workflow

1. Open an issue first for anything outside the current milestone.
2. Branch from `main`.
3. Open one pull request per change.
4. The aggregate CI check `ci-ok` must be green.
5. Fill in the checklist of the pull request template.

## Commit messages

Commits follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/),
in English. Allowed types:

`feat`, `fix`, `docs`, `test`, `refactor`, `build`, `ci`, `chore`

Use a scope when it helps, for example:

```text
feat(cli): add the clients command
fix(diag): keep Secret redacted in slog groups
docs: explain exit codes
```

## Project rules

- Standard library first. Every new dependency is justified in the pull request and
  recorded in the dependency ledger in [docs/architecture.md](docs/architecture.md).
- No empty packages: a package is created only when there is code to put in it.
- Values read from configuration that may be sensitive use `diag.Secret`.
- Finding codes are append-only: never reuse, renumber or delete a code.
- Results go to stdout; logs and errors go to stderr.

## Agent rules

These rules apply to AI coding agents and to humans alike. Contributors must not:

1. Change the architecture without approval.
2. Introduce unnecessary dependencies.
3. Disable tests or security checks.
4. Lower the required coverage to make CI pass.
5. Change acceptance criteria.
6. Implement features outside the agreed scope.
7. Perform refactoring that was not requested.
8. Hide errors with generic error handling.
9. Declare a milestone complete without evidence.
10. Merge security-critical changes automatically.
11. Publish releases without authorization.
12. Change the license or the security policy.
13. Claim that a protection works without verifying that it is enforced.

In addition: do not add AI attribution trailers or "generated with" lines to commits or
pull requests.

### Mandatory completion report

At the end of every task, an agent must produce a report with this structure:

```markdown
## Task Completion Report

**Milestone:**
**Task ID:**
**Status:** Completed / Partial / Blocked

### 1. Implementation Summary

Describe the implemented changes.

### 2. Files Changed

List the files created, modified or removed.

### 3. Architectural Decisions

Document relevant decisions and explain their rationale.

### 4. Security and Privacy

Describe security implications, controls and remaining risks.

### 5. Accessibility

Describe accessibility considerations and verification performed.

### 6. Tests Executed

List commands, test results and coverage.

### 7. Definition of Done

Report each acceptance criterion as passed, failed or not applicable, including supporting evidence.

### 8. Known Limitations

Document unresolved issues and unsupported scenarios.

### 9. Manual Verification Required

List any remaining manual verification activities.

### 10. Out-of-Scope Findings

Report relevant findings without implementing unrelated changes.

### 11. Recommended Next Step

Identify the next approved task or milestone.
```

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).

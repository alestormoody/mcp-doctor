# Security policy

## Supported versions

MCP Doctor is pre-release. Until v0.1.0 only the latest commit on `main` is supported.

## Reporting a vulnerability

Report vulnerabilities privately through GitHub private vulnerability reporting:
https://github.com/alestormoody/mcp-doctor/security/advisories/new

Never open a public issue for a vulnerability.

Please include:

- the output of `mcp-doctor version`;
- your operating system;
- the AI client(s) involved, if any;
- the steps to reproduce.

Redact secrets (tokens, API keys, environment values) before sending anything.

We aim to acknowledge reports within 7 days. This is a best-effort target, not a guarantee.

## Scope

MCP Doctor reads AI client configuration files, and those files often contain secrets.
Any leak of a secret value in any output (stdout, stderr, logs, files written by the tool)
is in scope, as are the usual classes of vulnerability in the code and the build and
release pipeline.

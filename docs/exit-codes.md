# Exit codes

`mcp-doctor` exits with one of these codes:

| Code | Meaning |
| ---- | ------- |
| 0 | No problems found: only PASS, INFO, WARNING or SKIPPED findings. |
| 1 | Problems diagnosed: at least one ERROR or CRITICAL finding. |
| 2 | Tool or usage error: bad flags, unknown command or internal failure. |

Codes 1 and 2 are never conflated: 1 means the tool worked and found problems,
2 means the tool itself could not do its job.

Results go to stdout; logs and errors go to stderr.

New exit codes may only be added in a later release, with a CHANGELOG entry.
Existing codes keep their meaning.

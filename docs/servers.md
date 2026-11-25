# Servers and validate

`mcp-doctor servers` and `mcp-doctor validate` read the config files that
[`mcp-doctor clients`](clients.md) finds, parse them (JSON, JSONC or TOML, per client) and
report what they contain. Both are read-only: files are read, never changed, and no MCP
server is started. Neither takes arguments or flags.

## `mcp-doctor servers`

Lists every MCP server defined in the config files, one row per server and file:

```text
CLIENT       SCOPE    NAME   TRANSPORT        TARGET                    NOTE      SOURCE
Claude Code  project  fetch  stdio            uvx                       -         /path/to/project/.mcp.json:3:5
Codex        user     docs   http (inferred)  https://h.example/…       disabled  /path/to/home/.codex/config.toml:1:14
```

| Column | Content |
| ------ | ------- |
| CLIENT | The client that reads the file. |
| SCOPE | `managed`, `user`, `project` or `local` (Claude Code servers stored per project in `~/.claude.json`). |
| NAME | The server name as written in the file. |
| TRANSPORT | `stdio`, `http`, `sse`, `ws` or `sdk`. `(inferred)` means the file did not state the type and it was inferred: a `command` means `stdio`, a `url` means `http`. `-` when unknown. |
| TARGET | For local servers, the command as written. For remote servers, the URL as `scheme://host[:port]`, followed by `/…` when the URL has a path, query, fragment or user info. `-` when empty. |
| NOTE | `disabled`, `shadowed` and the file's label, joined with `; `; `-` when there is none. |
| SOURCE | `path:line:col` of the server entry. |

NOTE values:

- `disabled`: Codex `enabled = false`, or the name is in Claude Code's
  `disabledMcpServers` for the current project.
- `shadowed`: the same client defines a server with the same name in a scope it ranks
  higher, so this entry is not used. Only Claude Code and Codex document an order; see
  [precedence](clients.md#precedence).
- Labels: for example `CLAUDE_CONFIG_DIR`, `MSIX`, `profile <name>` (Codex profile, used only
  with `--profile`), `trusted project` and `not trusted: Codex ignores this file` (Codex
  project files), `Insiders` or `profile <id>` (VS Code).

Never shown: argument values, environment values, header values, URL paths, queries and user
info, and any other value from the file. Variable references such as `${API_KEY}` are not
expanded. Environment and header names are not listed yet.

When no server is found, `servers` prints `No MCP servers found.` instead of the table.
Findings, if any, follow after a blank line, one per line:
`SEVERITY CODE path:line:col: message`.

## `mcp-doctor validate`

Prints the findings of discovery and parsing, in the same format, then one summary line:

```text
2 files checked, 3 servers, 1 error, 0 warnings
```

Files are counted once even when two clients read them (`.mcp.json`). On a machine without
config files the line is `0 files checked, 0 servers, 0 errors, 0 warnings`. Errors count
ERROR and CRITICAL findings.

Today `validate` checks syntax and structure only. Later releases add more checks, such as
missing commands, runtimes and environment variables.

## Exit codes

Both commands exit with 1 when there is at least one ERROR finding and 0 otherwise
(warnings and info do not fail). Extra arguments exit with 2. See [exit codes](exit-codes.md).

## Findings

| Code | Severity | Title | When it appears and how to fix it |
| ---- | -------- | ----- | --------------------------------- |
| `MCP_PARSE_001` | ERROR | Config file has a syntax error | The file is not valid JSON, JSONC or TOML (for example a missing comma, an unterminated string, a duplicate TOML key or an empty JSON file). No server of that file is listed. Fix the file at the reported position. |
| `MCP_PARSE_002` | ERROR | Config file has an invalid structure | A value has the wrong type: the file or the servers key is not an object, an entry is not an object, or a field such as `args` is not an array of strings. Only that entry is dropped (the whole file when the top level is wrong). Fix the type at the reported position. |
| `MCP_PARSE_003` | ERROR | Server has neither a command nor a URL | An entry has no `command` and no `url`, so the client cannot start or reach it. Add one, or remove the entry. The entry is dropped. |
| `MCP_PARSE_004` | WARNING | Duplicate key | The same key appears twice in a JSON object; the message names the key and the first position. The last one is used, as clients do. Remove the copy you do not want. |
| `MCP_PARSE_005` | INFO | Unknown or unsupported field in a server entry | An entry has a field this client does not document (often a typo such as `comand`), or a field the file kind does not support (VS Code `inputs`, `envFile`, `dev`, `sandboxEnabled` in `.mcp.json` or `~/.copilot/mcp-config.json`). Check the name; the entry is still listed. |
| `MCP_PARSE_006` | WARNING | Comments or trailing commas in a plain JSON file | The client reads the file as plain JSON, but it contains comments or trailing commas. The servers are listed, but the client may reject the file. Remove the comments and trailing commas. |
| `MCP_PARSE_007` | WARNING | Config file too large to parse | The file is larger than 64 MiB and was skipped. Check why the file is so large; for `~/.claude.json` the client keeps history there. |
| `MCP_PARSE_008` | ERROR | Transport type conflicts with the fields | The transport cannot work: a Claude Code entry has a `url` but no `type` (Claude Code then reads it as stdio), a `stdio` entry has only a `url`, an `http`/`sse`/`ws` entry has no `url`, or the `type` value is not one the client accepts. Set the right `type` or the missing field. The entry is dropped. |
| `MCP_PARSE_009` | WARNING | Undefined input reference | A VS Code `mcp.json` entry uses `${input:id}` but `inputs` has no entry with that `id`. Add the input or fix the id. |
| `MCP_PARSE_010` | INFO | File starts with a byte order mark | The file begins with a UTF-8 byte order mark. Some programs reject it; re-save the file as UTF-8 without BOM. |

Discovery findings (`MCP_DISC_001`, `MCP_DISC_002`) are described in
[clients](clients.md#findings).

## Positions and messages

- Positions are 1-based line and column; the column counts bytes, not characters. After a
  byte order mark, the first character is column 1.
- Messages are fixed texts. They never quote the file, because file content can hold
  secrets. Key names may appear in structure findings.
- A file with a syntax error (`MCP_PARSE_001`) lists no servers. Structure errors
  (`MCP_PARSE_002`, `003`, `008`) drop only the affected entry; other servers in the file are
  still listed.
- `.mcp.json` is read by Claude Code and by VS Code, each with its own rules. It is checked
  once for each, so its findings can appear twice, once per client.

# Supported clients and config paths

`mcp-doctor clients` looks for MCP configuration files of five AI clients. It only checks
fixed, documented locations: it stats each path and opens-then-closes regular files to prove
they can be read. It never reads file contents, never writes anything and never starts a
program. [`servers` and `validate`](servers.md) read the files found here and parse them with
the per-client rules below; they never write either.

Project files are looked up in the current directory. Codex is the exception: it looks in
every directory from the nearest ancestor that holds `.git` down to the current directory.

Notation: `~` is the home directory (`$HOME` on macOS and Linux, `%USERPROFILE%` on Windows).
"Config dir" is `~/Library/Application Support` on macOS, `%APPDATA%` on Windows
(default `~\AppData\Roaming`) and `$XDG_CONFIG_HOME` on Linux when it is set to an absolute
path, otherwise `~/.config`. `%LOCALAPPDATA%` defaults to `~\AppData\Local`.

## Claude Code

| Scope | macOS | Linux | Windows | Format |
| ----- | ----- | ----- | ------- | ------ |
| managed | `/Library/Application Support/ClaudeCode/managed-mcp.json` | `/etc/claude-code/managed-mcp.json` | `%ProgramFiles%\ClaudeCode\managed-mcp.json` (default `C:\Program Files`) | JSON |
| user | `~/.claude.json`, or `$CLAUDE_CONFIG_DIR/.claude.json` when set | same | same | JSON |
| project | `.mcp.json` (shared with VS Code) | same | same | JSON |

Config directory (detection only): `~/.claude`, or `$CLAUDE_CONFIG_DIR` when set.

Sources: [MCP](https://code.claude.com/docs/en/mcp),
[managed MCP](https://code.claude.com/docs/en/managed-mcp.md),
[Claude directory](https://code.claude.com/docs/en/claude-directory).

### Format and fields

All three files use a top-level `mcpServers` object. In `~/.claude.json` the servers of the
current project are read from `projects["<current directory>"].mcpServers` (scope `local`),
and `projects["<current directory>"].disabledMcpServers` marks servers as disabled; other
projects and every other key of that file are ignored.

Fields: `type` (`stdio`, `http`, `streamable-http` as an alias of `http`, `sse`, `ws`, `sdk`),
`command`, `args`, `env`, `url`, `headers`, `headersHelper`, `oauth`, `timeout`, `alwaysLoad`.
An entry without `type` is stdio; an entry with a `url` but no `type` is a configuration
error (`MCP_PARSE_008`), because Claude Code reads it as stdio.

### Variables

`${VAR}` and `${VAR:-default}` in `command`, `args`, `env`, `url` and `headers`. They are
recorded by name, never expanded; default values are not kept.

### Precedence

managed > local > project > user, by server name; the whole entry of the winning scope is
used. Lower entries are marked `shadowed`.

Source: [MCP](https://code.claude.com/docs/en/mcp). Verified on 2026-11-26.

## Codex

| Scope | macOS | Linux | Windows | Format |
| ----- | ----- | ----- | ------- | ------ |
| managed | `/etc/codex/config.toml` | same | not documented, not checked | TOML |
| user | `~/.codex/config.toml`, or `$CODEX_HOME/config.toml` when set | same | same | TOML |
| user (profiles) | every `<name>.config.toml` in the Codex home | same | same | TOML |
| project | `.codex/config.toml` in every directory from the `.git` root down to the current directory | same | same | TOML |

Config directory (detection only): the Codex home (`~/.codex` or `$CODEX_HOME`).
The Codex home is the only directory Codex discovery lists, to find profiles.

Sources: [config basics](https://learn.chatgpt.com/docs/config-file/config-basic),
[advanced config](https://learn.chatgpt.com/docs/config-file/config-advanced).

### Format and fields

TOML tables `[mcp_servers.<name>]`; the inline form under `[mcp_servers]` and dotted keys
(`mcp_servers.<name>.command = ...`) are read the same way. There is no `type` field: a
`command` means stdio, a `url` means streamable HTTP.

Fields: `command`, `args`, `env`, `env_vars`, `cwd`, `url`, `auth`, `oauth`,
`bearer_token_env_var`, `http_headers`, `http_headers_helper`, `env_http_headers`, `enabled`,
`required`, `startup_readiness`, `startup_timeout_sec`, `startup_timeout_ms`,
`tool_timeout_sec`, `enabled_tools`, `disabled_tools`, `supports_parallel_tool_calls`,
`tool_input_schema_max_bytes`, `omit_tools_from`, `default_tools_approval_mode`, `tools`,
`scopes`, `oauth_resource`, `environment_id`. `enabled = false` marks the server disabled.

### Variables

No interpolation is documented. Environment variables named by `env_vars`,
`bearer_token_env_var` and the values of `env_http_headers` are recorded as references.

### Precedence

Closest project file > ... > project file at the `.git` root > user > system, by server
name. Profile files apply only with `--profile`, so they never shadow anything. Project files
are labelled `trusted project` when the user config marks the project (or a parent) trusted
in `projects."<path>".trust_level`, otherwise `not trusted: Codex ignores this file`; untrusted
files shadow nothing. How Codex merges `mcp_servers` tables across layers is not documented:
`mcp-doctor` treats the whole entry as replaced.

Sources: [MCP](https://learn.chatgpt.com/docs/extend/mcp?surface=cli),
[config reference](https://learn.chatgpt.com/docs/config-file/config-reference.md),
[config basics](https://learn.chatgpt.com/docs/config-file/config-basic). Verified on 2026-11-26.

Confidence: Codex only loads project files for projects it trusts. `mcp-doctor` lists them
regardless, because trust is stored in Codex's own config.

## Claude Desktop

| Scope | macOS | Linux | Windows | Format |
| ----- | ----- | ----- | ------- | ------ |
| user | `~/Library/Application Support/Claude/claude_desktop_config.json` | `<config dir>/Claude/claude_desktop_config.json` | `%APPDATA%\Claude\claude_desktop_config.json` | JSON |
| user (MSIX) | - | - | `%LOCALAPPDATA%\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Roaming\Claude\claude_desktop_config.json` | JSON |
| user (MSIX) | - | - | `%LOCALAPPDATA%\Claude-Data\Claude_pzs8sxrjxfjjc\claude_desktop_config.json` | JSON |

Config directory (detection only): the folder of each file above.

Sources: [Connect to local MCP servers](https://modelcontextprotocol.io/docs/develop/connect-local-servers),
[Windows MSIX reports](https://github.com/anthropics/claude-code/issues/26073).

Confidence: the two MSIX paths come from community reports, not from official documentation.
Every one of them that exists is listed; none is assumed to be the one Claude Desktop uses.

### Format and fields

Top-level `mcpServers` with `command`, `args` and `env`. Remote servers are added in the app
(Connectors), not in this file; a `url` or `type` found here is reported as an unknown field
(`MCP_PARSE_005`) and only used to infer the transport.

### Variables

None documented: `${...}` is plain text in this file.

### Precedence

Not documented; nothing is marked `shadowed`.

Source: [Connect to local MCP servers](https://modelcontextprotocol.io/docs/develop/connect-local-servers).
Verified on 2026-11-26.

## Cursor

| Scope | macOS | Linux | Windows | Format |
| ----- | ----- | ----- | ------- | ------ |
| user | `~/.cursor/mcp.json` | same | same | JSON |
| project | `.cursor/mcp.json` | same | same | JSON |

Config directory (detection only): `~/.cursor`.

Source: [Cursor MCP](https://cursor.com/docs/context/mcp).

### Format and fields

Top-level `mcpServers`. Local servers: `type` (`stdio`), `command`, `args`, `env`, `envFile`
(path only; the file is never read). Remote servers: `url`, `headers`, `auth`
(`CLIENT_ID`, `CLIENT_SECRET`, `scopes`; values never kept). A missing `type` is inferred.
The file is plain JSON: comments or trailing commas give `MCP_PARSE_006`.

### Variables

`${env:NAME}`, `${userHome}`, `${workspaceFolder}`, `${workspaceFolderBasename}`,
`${pathSeparator}` and `${/}` in `command`, `args`, `env`, `url`, `headers` and `auth`.

### Precedence

Not documented; nothing is marked `shadowed`.

Source: [Cursor MCP](https://cursor.com/docs/context/mcp). Verified on 2026-11-26.

## VS Code

| Scope | macOS | Linux | Windows | Format |
| ----- | ----- | ----- | ------- | ------ |
| user | `~/Library/Application Support/Code/User/mcp.json` | `<config dir>/Code/User/mcp.json` | `%APPDATA%\Code\User\mcp.json` | JSONC |
| user (Insiders) | same with `Code - Insiders` | same | same | JSONC |
| user (profiles) | `<User dir>/profiles/<id>/mcp.json`, for stable and Insiders | same | same | JSONC |
| user (Copilot) | `~/.copilot/mcp-config.json`, or `$COPILOT_HOME/mcp-config.json` when set | same | same | JSON |
| project | `.vscode/mcp.json` | same | same | JSONC |
| project | `.mcp.json` (shared with Claude Code) | same | same | JSON |

Config directories (detection only): the `User` directory of stable and Insiders, and the
Copilot home (`~/.copilot` or `$COPILOT_HOME`). The two `User/profiles` directories are the
only ones VS Code discovery lists, one level deep.

Sources: [MCP configuration reference](https://code.visualstudio.com/docs/agents/reference/mcp-configuration),
[profiles](https://code.visualstudio.com/docs/configure/profiles),
[settings](https://code.visualstudio.com/docs/configure/settings).

Confidence: the `User/profiles/<id>/mcp.json` layout is not in the official documentation.

### Format and fields

`mcp.json` (user, profiles, `.vscode/mcp.json`; JSON with comments): top-level `servers`, plus
`inputs` and `sandbox`. Fields: `type` (`stdio`, `http`, `sse`), `command`, `args`, `cwd`,
`env`, `envFile`, `dev`, `sandboxEnabled`, `sandbox`, `url`, `headers`, `oauth`. A missing
`type` is inferred.

`~/.copilot/mcp-config.json` and `.mcp.json` (plain JSON): top-level `mcpServers`. Fields:
`type` (`stdio`, `local` as an alias of `stdio`, `http`, `sse`), `command`, `args`, `env`,
`cwd` (`~/.copilot/mcp-config.json` only), `url`, `headers`, `tools`, `oauthClientId`.
`inputs`, `envFile`, `dev` and `sandboxEnabled` are not supported in these files
(`MCP_PARSE_005`).

### Variables

`mcp.json`: `${env:NAME}`, `${input:id}`, `${workspaceFolder}`, `${userHome}` and the other
predefined `${...}` variables, in every string field. A `${input:id}` without a matching
`inputs[].id` gives `MCP_PARSE_009`.

`~/.copilot/mcp-config.json` and `.mcp.json`: `${NAME}` and `${NAME:-default}` in `command`,
`args`, `env`, `cwd`, `url` and `headers`; a bare `$NAME` is literal text.

### Precedence

Not documented; nothing is marked `shadowed`.

Sources: [MCP configuration reference](https://code.visualstudio.com/docs/agents/reference/mcp-configuration),
[variables reference](https://code.visualstudio.com/docs/reference/variables-reference).
Verified on 2026-11-26.

## Detected

A client is detected when one of its own config files or config directories exists, even if
it cannot be read. `.mcp.json` in the project is shared by Claude Code and VS Code: it is
listed under both, but by itself it does not mark either client as detected.

## Precedence

Precedence is applied only inside one client, by server name, and only where the client
documents an order: Claude Code (managed > local > project > user) and Codex (closest project
> ... > user > system; profiles only with `--profile`; untrusted projects ignored). An entry
that loses is still listed by `servers`, with the note `shadowed`. Cursor, VS Code and Claude
Desktop do not document an order, so nothing is marked shadowed there. Servers with the same
name in different clients are independent.

`.mcp.json` is parsed once for Claude Code and once for VS Code, each with its own fields and
variable syntax.

## Statuses

| Status | Meaning |
| ------ | ------- |
| found | The file exists and can be opened. |
| missing | Nothing exists at the path. |
| unreadable | Something exists but cannot be used: not a regular file, broken symlink or permission denied. |

## Findings

| Code | Severity | Title | Meaning and fix |
| ---- | -------- | ----- | --------------- |
| `MCP_DISC_001` | WARNING | Config file cannot be read | A config file or directory exists but cannot be opened or listed. Check its permissions and ownership; `mcp-doctor` needs read access only. |
| `MCP_DISC_002` | WARNING | Config location cannot be resolved | The home or current directory is unknown, so user or project locations were skipped. Set `HOME` (`USERPROFILE` on Windows) or run from an existing directory. |

Warnings exit with code 0, see [exit codes](exit-codes.md).

## Not covered yet

- Claude Code approval settings in `~/.claude/settings*.json` (`enabledMcpjsonServers`,
  `disabledMcpjsonServers`, `enableAllProjectMcpServers`), plugin-provided servers and
  claude.ai connectors.
- Local-scope servers of projects other than the current directory in `~/.claude.json`.
- The `"mcp"` key of VS Code `settings.json`.
- Codex `requirements.toml`, cloud-managed config and `[profiles]` tables.
- `--json` output and `--project` to pick another directory.
- Forks of VS Code such as VSCodium.
- WSL, dev containers and VS Code remote windows.

Paths verified on 2026-11-25 against the vendors' documentation.
Schemas, variables and precedence verified on 2026-11-26 against the sources named in each
section.

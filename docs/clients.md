# Supported clients and config paths

`mcp-doctor clients` looks for MCP configuration files of five AI clients. It only checks
fixed, documented locations: it stats each path and opens-then-closes regular files to prove
they can be read. It never reads file contents, never writes anything and never starts a
program.

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

## Cursor

| Scope | macOS | Linux | Windows | Format |
| ----- | ----- | ----- | ------- | ------ |
| user | `~/.cursor/mcp.json` | same | same | JSON |
| project | `.cursor/mcp.json` | same | same | JSON |

Config directory (detection only): `~/.cursor`.

Source: [Cursor MCP](https://cursor.com/docs/context/mcp).

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

## Detected

A client is detected when one of its own config files or config directories exists, even if
it cannot be read. `.mcp.json` in the project is shared by Claude Code and VS Code: it is
listed under both, but by itself it does not mark either client as detected.

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

- Parsing config files and listing their servers.
- `--json` output and `--project` to pick another directory.
- VS Code `settings.json` and forks such as VSCodium.
- WSL, dev containers and VS Code remote windows.

Paths verified on 2026-11-25 against the vendors' documentation.

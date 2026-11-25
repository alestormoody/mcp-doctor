# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Go module and the `mcp-doctor version` command.
- Diagnostic core: `Secret` type, `Finding`, finding-code registry and exit codes 0/1/2.
- CI on Linux, macOS and Windows.
- GoReleaser snapshot build.
- Project documentation and GitHub templates.
- `mcp-doctor clients`: read-only discovery of MCP config files for Claude Code, Codex, Claude Desktop, Cursor and VS Code on macOS, Linux and Windows.
- Finding codes `MCP_DISC_001` (config file cannot be read) and `MCP_DISC_002` (config location cannot be resolved), both WARNING.
- `mcp-doctor servers`: lists every MCP server configured for the five clients, with client, scope, transport, target and file position. Values of arguments, environment variables and headers are never shown; URLs only as `scheme://host[:port]`.
- `mcp-doctor validate`: reports syntax and structure problems of every config file with file, line and column, and a summary line.
- JSON, JSONC and TOML parsing for Claude Code, Codex, Claude Desktop, Cursor and VS Code, with file, line and column for every server, variable references kept unexpanded and per-client scope precedence (Claude Code, Codex).
- Finding codes:
  - `MCP_PARSE_001` (ERROR) config file has a syntax error.
  - `MCP_PARSE_002` (ERROR) config file has an invalid structure.
  - `MCP_PARSE_003` (ERROR) server has neither a command nor a URL.
  - `MCP_PARSE_004` (WARNING) duplicate key.
  - `MCP_PARSE_005` (INFO) unknown or unsupported field in a server entry.
  - `MCP_PARSE_006` (WARNING) comments or trailing commas in a plain JSON file.
  - `MCP_PARSE_007` (WARNING) config file too large to parse.
  - `MCP_PARSE_008` (ERROR) transport type conflicts with the fields.
  - `MCP_PARSE_009` (WARNING) undefined input reference.
  - `MCP_PARSE_010` (INFO) file starts with a byte order mark.
- Decision record [0001](docs/decisions/0001-format-preserving-edits.md): how later releases will edit JSON, JSONC and TOML files without changing untouched bytes.

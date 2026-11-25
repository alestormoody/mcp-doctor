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

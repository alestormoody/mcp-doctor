# MCP Doctor

MCP Doctor is a local, read-only-by-default command-line tool that diagnoses Model Context Protocol (MCP) server configurations across AI clients.

**Status:** Pre-alpha. `mcp-doctor clients` lists where five AI clients keep their MCP config files, `mcp-doctor servers` lists the MCP servers configured in them with file and position, and `mcp-doctor validate` reports syntax and structure problems. All three are read-only: no file is ever written and no server is started. `mcp-doctor version` prints the version.

Planned milestones: see [docs/roadmap.md](docs/roadmap.md).

## Same name, different tools

MCP Doctor (https://github.com/alestormoody/mcp-doctor) is a separate project. Not affiliated with other projects that use a similar name, including the npm packages `mcp-doctor` (Crooj026) and `@wigu/mcp-doctor` (realwigu), the PyPI packages `mcp-doctor` (Jiansen) and `mcp-config-doctor`, and the GitHub repositories `destilabs/mcp-doctor` and `vishalhabib99/mcp-doctor`. Several of them install a command called `mcp-doctor` too. To see which one runs on your machine, use `which -a mcp-doctor` (Windows: `where mcp-doctor`); `mcp-doctor version` prints the full name, version and project URL.

## Documentation

- [Supported clients and config paths](docs/clients.md)
- [Servers and validate](docs/servers.md)
- [OS support](docs/os-support.md)
- [Exit codes](docs/exit-codes.md)
- [Roadmap](docs/roadmap.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

## License

Apache License 2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).

Not affiliated with Anthropic, OpenAI, Cursor or Microsoft; MCP is the Model Context Protocol.

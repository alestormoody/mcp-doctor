# OS support

| OS | Architecture | CI-tested | Release binary | Status |
| -- | ------------ | --------- | -------------- | ------ |
| macOS | arm64 | yes | planned for v0.1 | pre-alpha |
| macOS | amd64 | build only | planned for v0.1 | pre-alpha |
| Linux | amd64 | yes | planned for v0.1 | pre-alpha |
| Linux | arm64 | build only | planned for v0.1 | pre-alpha |
| Windows | amd64 | yes | planned for v0.1 | pre-alpha |

"Build only" means the binary is cross-compiled by the GoReleaser snapshot build but tests
do not run on that platform.

Windows on arm64 is not planned for v0.1.

The minimum Go version to build from source is 1.26.

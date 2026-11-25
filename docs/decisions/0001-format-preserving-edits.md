# 0001: Format-preserving edits of client config files

- Status: accepted
- Date: 2026-11-26

## Context

Later milestones (fix and migrate) must change MCP client config files without losing what the user
wrote: comments, key order, whitespace, line endings, trailing commas, unknown keys and their values.
Re-marshalling a decoded model or a `map[string]any` loses all of that, so it is never used to write.

The files are JSON, JSONC (JSON with comments and trailing commas) and TOML. Both parsing libraries
were tested on the pinned versions: `github.com/tailscale/hujson` (commit `b80ff77dac4f`) and
`github.com/pelletier/go-toml/v2` v2.4.3.

## Decision

**JSON and JSONC.** Parse the original bytes with hujson, apply RFC 6902 operations with `Patch` on a
second, independent AST obtained by parsing the same bytes again, then `Pack`. A leading UTF-8 byte
order mark is stripped before parsing and put back after packing; the newline appended so that a line
comment at end of file parses is removed again.

**TOML.** Never re-encode. Locate the byte span of one `mcp_servers.<name>` entry from the key ranges
of the go-toml `unstable` parser, splice new bytes into the original buffer at that span, then parse the
result again and refuse the edit if it does not parse. The span is:

- `[mcp_servers.<name>]` table: from the `[` of its header to the start of the next table header, or end of file;
- inline table (`<name> = { ... }` under `[mcp_servers]`) and dotted keys (`mcp_servers.<name>.x = ...`):
  from the first to the last `key = value` expression under that path.

Any change that cannot be expressed this way is refused and the user gets manual instructions instead.

## Evidence

Tests in `internal/config/spike_test.go`, run on every fixture in `internal/config/testdata/spike`
(JSON, JSONC with comments and trailing commas, non-ASCII names, a line comment at end of file without a
final newline, and the three TOML shapes plus a quoted server name), each also as CRLF and BOM variants:

- `TestEditSpikeJSONNoOp`: parse then pack is byte-identical.
- `TestEditSpikeJSONReplace`: replacing one server's `command` changes only the bytes of that value;
  comments, trailing commas and CRLF line endings survive.
- `TestEditSpikeJSONLimits`: asserts the hujson limits listed below, so a library upgrade that changes
  them fails the build.
- `TestEditSpikeTOMLNoOp`: splicing a span with itself is byte-identical; splicing a changed command
  leaves every byte outside the span identical and the result parses with the new value.
- `TestTOMLKeys`: key paths and offsets for tables, inline tables, quoted names and comments.

## Known limits

- hujson `Value.Clone` drops trailing commas (it turns the empty marker that records a trailing comma
  into nil). Edits therefore work on a fresh parse of the original bytes, never on a Clone.
- A failed `Patch` leaves its receiver partially changed: always patch a copy and discard it on error.
- `add` inserts compact JSON (`"c":{"command":"z"}`) with no indentation: inserted values must be
  formatted to match the surrounding file before insertion.
- go-toml `unstable` gives `Table` expressions an empty range, so a table span runs from its header to
  the next header. Comments directly above the next header therefore fall inside the preceding table's
  span: trailing comment lines must be trimmed from a span before splicing.
- A sub-table such as `[mcp_servers.<name>.env]` has its own header. The bytes kept for a server run from
  its header to the end of its last sub-table, so a sub-table placed after another server's table stays
  inside; that covering range then also holds the tables in between. Such entries are refused for edits.
- Dotted-key entries interleaved with other keys are refused, since the span would include unrelated lines.
- Only the three TOML shapes above are supported.

## Consequences

- `go-toml/v2/unstable` has no compatibility promise: it stays pinned at v2.4.3 and is imported by
  exactly one production file (`internal/config/toml.go`); the spike tests catch behaviour changes.
- No production edit API exists until the fix and migrate milestones; parsing code depends only on the
  read side (positions and key ranges).

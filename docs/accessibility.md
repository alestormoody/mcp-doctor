# Accessibility

## Principles

- Never colour only: severity is always printed as a text label (`PASS`, `INFO`,
  `WARNING`, `ERROR`, `CRITICAL`, `SKIPPED`).
- Honour `NO_COLOR`: when it is set, no colour is emitted.
- No prompts without a TTY: when input is not interactive the tool fails with a clear
  error instead of hanging.
- Plain ASCII fallbacks for symbols and box drawing.
- Machine-readable output (planned, not implemented yet).

## Status

Nothing interactive exists yet: the only command is `mcp-doctor version`, which prints
plain text. A documented screen-reader pass (VoiceOver and NVDA) is due in milestone M08.

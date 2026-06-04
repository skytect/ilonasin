# 476 TUI Log Table Chrome Helper

## Context

Request logs and fallback logs both render table chrome the same way: compute
columns, write a header, then write a separator when available. The table
helpers already centralize header and separator rendering, but the call-site
composition is still duplicated in each log section.

The active TUI backlog calls for deduplicating repeated visual language and
keeping logs compact and table-like.

## Goal

Move shared log table chrome composition into one helper used by request and
fallback log rendering.

## Scope

1. Update only TUI log display files, expected to be
   `internal/tui/log_tables.go`, `internal/tui/log_requests.go`, and
   `internal/tui/log_fallbacks.go`.
2. Keep request-specific column sizing and labels in `log_requests.go`.
3. Keep fallback-specific column sizing and labels in `log_fallbacks.go`.
4. Do not change management DTOs, storage, routing behavior, keybindings,
   snapshot refresh behavior, logging policy, or IO capture behavior.
5. Do not add permanent tests.

## Implementation

1. Add a helper that writes `plainTableHeader` plus optional
   `plainTableSeparator` into a `strings.Builder`.
2. Use the helper from `writeRecentRequests`.
3. Use the helper from `writeFallbacks`.
4. Run `gofmt` on touched Go files.

## Verification

Run:

```sh
git diff --check
go test ./internal/tui
go test ./...
go vet ./...
```

Run a direct CLI smoke by building a temporary `ilonasin` binary, starting
`ilonasin serve` with an isolated temporary home and config, checking the
management health and snapshot endpoints over the Unix socket, running bounded
`ilonasin manage` at several terminal widths, then cleaning up all temporary
files and processes.

## Acceptance

- Request and fallback log table chrome use one shared composition helper.
- Column sizing and labels remain local to request and fallback logs.
- Header and separator rendering remain unchanged.
- No unrelated files are changed or staged.

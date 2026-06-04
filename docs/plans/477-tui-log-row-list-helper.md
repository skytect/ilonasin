# 477 TUI Log Row List Helper

## Context

Request logs and fallback logs both render row lists with the same structure:
insert a blank line before every row after the first, render the summary row,
then append a trailing newline. This repeated composition controls readability
for multi-line log items, which the active TUI backlog calls out as an area to
keep consistent.

## Goal

Move shared log row-list composition into one helper used by request and
fallback log rendering.

## Scope

1. Update only TUI log display files, expected to be
   `internal/tui/log_details.go`, `internal/tui/log_requests.go`, and
   `internal/tui/log_fallbacks.go`.
2. Keep request-specific row rendering in `log_requests.go`.
3. Keep fallback-specific row rendering in `log_fallbacks.go`.
4. Preserve existing blank-line and trailing-newline behavior.
5. Do not change management DTOs, storage, routing behavior, keybindings,
   snapshot refresh behavior, logging policy, or IO capture behavior.
6. Do not add permanent tests.

## Implementation

1. Add a helper that takes a row count and row-render callback, writing rows
   into a `strings.Builder` with the existing blank-line separation.
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

- Request and fallback log row-list rendering use one shared helper.
- Existing blank-line separation between multi-line items is preserved.
- Existing trailing newline after each rendered row is preserved.
- No unrelated files are changed or staged.

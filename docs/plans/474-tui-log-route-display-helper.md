# 474 TUI Log Route Display Helper

## Context

Request logs and fallback logs now both format provider/model routes through
small local helpers. The helpers have the same formatting rules: render
`provider/model` when both values exist, otherwise render the one available
value without a stray slash.

Keeping that logic duplicated makes the logs view harder to keep consistent as
the TUI cleanup proceeds.

## Goal

Move shared provider/model route display formatting into one log display helper
used by both request and fallback log rendering.

## Scope

1. Update only TUI log display files, expected to be
   `internal/tui/log_details.go`, `internal/tui/log_requests.go`, and
   `internal/tui/log_fallbacks.go`.
2. Keep request-specific route selection in `log_requests.go`.
3. Keep fallback-specific row extraction in `log_fallbacks.go`.
4. Do not change management DTOs, storage, routing behavior, keybindings,
   snapshot refresh behavior, or logging policy.
5. Do not add permanent tests.

## Implementation

1. Add a shared helper that takes provider and model strings and formats them
   through the existing safe wrapped display path.
2. Use the shared helper from request log route rendering.
3. Use the shared helper from fallback log route rendering.
4. Remove the now-local duplicated request route formatter.
5. Run `gofmt` on touched Go files.

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

- Request and fallback log route rendering share one formatter.
- Normal `provider/model` rendering is unchanged.
- Partial provider/model rows still render without stray slashes.
- No unrelated files are changed or staged.

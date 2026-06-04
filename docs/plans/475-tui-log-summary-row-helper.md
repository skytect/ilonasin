# 475 TUI Log Summary Row Helper

## Context

Request logs and fallback logs both compose a summary row the same way: render a
compact table row, render detail rows, then pass both through
`wrapTargetedLinesPreserveBlank`. That repeated composition is small, but it is
part of the ongoing logs cleanup where the TUI should be concise and shared
presentation rules should live in one place.

## Goal

Move shared log summary-row composition into one helper used by request and
fallback log rendering.

## Scope

1. Update only TUI log display files, expected to be
   `internal/tui/log_details.go`, `internal/tui/log_requests.go`, and
   `internal/tui/log_fallbacks.go`.
2. Keep request-specific state, table row construction, and detail fields in
   `log_requests.go`.
3. Keep fallback-specific table row construction and detail fields in
   `log_fallbacks.go`.
4. Do not change management DTOs, storage, routing behavior, keybindings,
   snapshot refresh behavior, logging policy, or IO capture behavior.
5. Do not add permanent tests.

## Implementation

1. Add a helper that takes a pre-rendered head row, detail rows, and width, then
   wraps them with the existing blank-preserving line wrapper.
2. Use the helper from `requestSummaryRow`.
3. Use the helper from `fallbackSummaryRow`.
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

- Request and fallback summary rows use one shared composition helper.
- Request and fallback row content is unchanged.
- Wrapped multi-line log rows still preserve blank separation.
- No unrelated files are changed or staged.

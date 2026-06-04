# 463 TUI Request Log Detail Rows

## Goal

Make request logs easier to scan by replacing chip-heavy per-request detail
lines with compact aligned detail rows.

## Scope

- Keep the existing request table header and main table row.
- Replace the expanded `meta` and `metrics` chip blocks under each request with
  aligned key/value rows.
- Preserve route, credential, model, status/error, token mix, total tokens,
  cache hit rate, attempts, auth retries, fallbacks, latency, TTFT, and TPS
  details.
- Keep request logs as metadata-only unless IO logging is enabled.
- Do not change storage, management DTOs, request metadata recording, fallback
  logs, pruning display, keybindings, or pane layout policy.
- Do not add permanent tests.

## Verification

- `gofmt` on touched Go files.
- `git diff --check`.
- `go test ./internal/tui`.
- `go test ./...`.
- `go vet ./...`.
- Build a temporary binary, run isolated `serve`, verify management health and
  snapshot over the Unix socket, run bounded `manage` at 70, 100, and 140
  columns, then clean up.

## Risks

- Detail rows must still wrap cleanly at narrow widths.
- Removing chips from details should not hide status or token/cache information.
- Shared helpers should not accidentally alter fallback log rendering unless
  explicitly in scope.

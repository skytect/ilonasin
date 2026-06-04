# 462 TUI Combined Usage Provider Rows

## Goal

Reduce repeated provider rows in the Usage metrics pane while preserving token,
cache, latency, TTFT, throughput, cost, and stream details.

## Scope

- Combine token usage and latency summaries by provider instance.
- Keep the all-provider overview at the top.
- Keep stream completion summaries separate because they are keyed by completion
  status, not provider.
- Avoid changing management DTOs, storage, request logging, subscriptions,
  health, keybindings, or layout policy.
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

- A provider can have token usage without latency data, or latency data without
  token usage; both cases must still render.
- Combining rows should not hide stream status.
- The pane should stay readable at narrow widths.

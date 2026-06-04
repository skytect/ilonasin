# 461 TUI Endpoint Health Rows

## Goal

Make the Usage health pane read as endpoint status, not event history.

## Scope

- Keep health detail visible, but present each daemon health row as a current endpoint check.
- Remove event-led grouping/count language from the health pane.
- Show route, credential, state, HTTP/error, last observation time, and retry timing compactly.
- Leave quota, logs, storage, management DTOs, and keybindings unchanged.
- Do not add permanent tests.

## Verification

- `gofmt` on touched Go files.
- `git diff --check`.
- `go test ./internal/tui`.
- `go test ./...`.
- `go vet ./...`.
- Build a temporary binary, run isolated `serve`, verify management health and snapshot over the Unix socket, run bounded `manage` at 70, 100, and 140 columns, then clean up.

## Risks

- Removing aggregation helpers could hide repeated-row information if the storage query changes later.
- Health rows must still distinguish credential-specific failures.
- The empty state and pane subhead should not imply event-log semantics.

# 470 TUI Detail Line Helper Boundary

## Goal

Move the shared aligned detail-line renderer out of the request log file and
give it a neutral name.

## Scope

- Move `requestDetailLine` from `log_requests.go` to the shared detail helper
  file.
- Rename it to `detailMetricLine`.
- Update the local-token usage call site.
- Preserve request log and local-token usage rendering behavior.
- Do not change storage, management DTOs, metadata recording, pruning display,
  keybindings, pane layout policy, or CLI behavior.
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

- This should be a pure move and rename; formatting output must remain the same.
- The helper should not gain request-log-specific policy in its new location.
- The local-token pane must keep its usage detail formatting.

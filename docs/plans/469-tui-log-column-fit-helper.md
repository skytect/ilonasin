# 469 TUI Log Column Fit Helper

## Goal

Use one shared table-column fitting helper for request and fallback log tables.

## Scope

- Move `fitTableColumns` from request logs to the shared log table helper file.
- Replace fallback log table's hand-rolled width shrink/grow logic with
  `fitTableColumns`.
- Preserve fallback base column widths, minimum widths, and grow order.
- Keep request-specific and fallback-specific table schema local to their files.
- Preserve request log, fallback log, and detail-row rendering behavior.
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

- Fallback columns currently shrink down to one cell when needed; minimums must
  preserve that behavior.
- The shared helper should remain table-only and not gain log-pane policy.
- Narrow-width rendering must remain stable.

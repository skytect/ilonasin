# 466 TUI Log Table Helper Boundary

## Goal

Move shared plain-table rendering helpers out of the request log file so request,
fallback, and log-detail renderers share a clear table helper boundary.

## Scope

- Add a small shared TUI log-table helper file.
- Move `fitPlainCellFirstLine`, `wrappedPlainTableRow`, `wrapPlainTableCell`,
  and `padPlainCell` into that shared file.
- Leave request-specific and fallback-specific column sizing, labels, and row
  construction in their existing files.
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

- This should be a pure move; accidental changes to wrapping or padding would
  affect several panes.
- Imports must remain minimal after moving `ansi` usage.
- Request and fallback table policy should not move into the generic helper.

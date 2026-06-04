# 467 TUI Log Table Chrome Helpers

## Goal

Remove duplicated log table header and separator rendering while keeping each
log pane's table schema local to that pane.

## Scope

- Add shared `plainTableHeader` and `plainTableSeparator` helpers beside the
  existing plain table row helpers.
- Use them from request and fallback log renderers.
- Keep request-specific and fallback-specific columns, labels, row construction,
  and sorting unchanged.
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

- Label count and column count can diverge; helper must only render overlapping
  entries safely.
- Separator behavior for zero or negative widths should remain unchanged.
- This should not move pane-specific table policies into generic helpers.

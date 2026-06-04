# 468 TUI Log Table Wrapper Cleanup

## Goal

Remove request and fallback log table header/separator wrappers that only
delegate to shared table chrome helpers.

## Scope

- Call `plainTableHeader` and `plainTableSeparator` directly from request and
  fallback log renderers.
- Keep request-specific `requestTableColumns` and `requestTableLabels`.
- Keep fallback-specific `fallbackTableColumns` and fallback labels.
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

- Direct call sites must compute columns once where labels depend on columns.
- Removing wrappers must not move table policy into shared helpers.
- Empty-width separator behavior must stay unchanged.

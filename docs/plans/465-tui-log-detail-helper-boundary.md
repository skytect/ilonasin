# 465 TUI Log Detail Helper Boundary

## Goal

Move shared log detail-row rendering out of the request log file so request and
fallback log code have a cleaner boundary.

## Scope

- Add a small shared TUI log-detail helper file.
- Move `logDetailField`, `logDetailRows`, and the label-width helper into that
  shared file.
- Rename request-specific field collection to make its request scope clear.
- Preserve request and fallback log rendering behavior.
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

- A pure move can accidentally change helper visibility or call sites.
- Request-specific and fallback-specific detail collection should remain in
  their own files.
- The old request-only helper name must not survive on shared code.

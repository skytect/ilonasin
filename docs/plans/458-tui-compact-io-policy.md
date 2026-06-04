# 458 TUI Compact IO Policy

## Goal

Make the Logs IO policy and pruning pane take less space while keeping the same operational detail.

## Scope

- Change only `internal/tui/log_pruning.go`.
- Replace the repeated metadata, capture, and retention rows with a compact two-row policy summary.
- Keep visible counts for request, fallback, health, and quota metadata rows.
- Keep IO capture mode, policy, content boundary, retention target, telemetry kept-pending-prune status, manual prune mode, and cutoff visible.
- Keep last-prune results visible when present.
- Do not change logging behavior, pruning behavior, management APIs, config, SQLite, or keybindings.

## Verification

- `gofmt` on touched Go files.
- `git diff --check`.
- `find . -name '*_test.go' -type f -print`.
- `go test ./internal/tui`.
- `go test ./...`.
- `go vet ./...`.
- Build a temp `ilonasin` binary, run isolated `serve`, smoke `/health` and `/snapshot`, and open `manage` at 70, 100, and 140 columns with cleanup afterward.

## Risks

- Over-compression could hide the IO logging boundary, so mode, policy, and content stay visible.
- The pane must still expose enough context for the `p` prune action to feel grounded.

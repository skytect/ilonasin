# 460 TUI Live Snapshot Refresh

## Goal

Make the management TUI keep SQLite-backed state live without relying on manual or action-triggered reloads.

## Scope

- Add a periodic management snapshot refresh loop in the TUI.
- Refresh all snapshot-backed panes through the existing daemon-owned management snapshot API.
- Keep the current subscription usage refresh loop separate because it can trigger upstream fetches.
- Use an async Bubble Tea command with a timeout and an in-flight guard, not a blocking reload inside `Update`.
- Preserve subscription usage rows, pools, observed time, and keepalive state during automatic snapshot refreshes.
- Avoid changing management APIs, SQLite schema, provider behavior, config, or keybindings.
- Do not make the TUI read or write SQLite directly.
- Preserve current pane focus, scroll offsets, selections, and in-progress input modes where practical.

## Verification

- `gofmt` on touched Go files.
- `git diff --check`.
- `find . -name '*_test.go' -type f -print`.
- `go test ./internal/tui`.
- `go test ./...`.
- `go vet ./...`.
- Build a temp `ilonasin` binary, run isolated `serve`, smoke `/health` and `/snapshot`, and open `manage` at 70, 100, and 140 columns with cleanup afterward.

## Risks

- A polling loop could fight user edits if it clobbers in-progress API-key or OAuth state.
- Snapshot refresh errors should not spam the status line or break key handling.
- The new loop must not trigger upstream subscription refreshes on every tick.
- Periodic reload must remain daemon API based, not direct SQLite access.
- Slow snapshot calls must not overlap or apply stale data out of order.

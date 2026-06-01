# 128 TUI Update Dispatcher Split

## Context

Plans 103 and 106 through 127 made the TUI tabbed and moved rendering,
display helpers, actions, snapshot lifecycle, shared helpers, and model state
out of `internal/tui/tui.go`. The main TUI file now owns only the Bubble Tea
`Update` dispatcher and `Run`.

The architecture says `ilonasin manage` should be a first-class local control
plane that talks through the daemon-owned management API and does not mutate
`config.toml`. The event dispatcher is TUI behavior, not runtime wiring. Moving
it into a focused file leaves `tui.go` as the TUI entrypoint and makes future
dispatcher decomposition easier.

## Goal

Move the Bubble Tea `Update` dispatcher out of `internal/tui/tui.go` into a
dedicated same-package file without changing behavior.

After this slice, `tui.go` owns only `Run`. The dispatcher lives in
`internal/tui/update.go`.

## Scope

1. Create `internal/tui/update.go`.
2. Move `func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)` intact.
3. Keep `Run` in `tui.go`.
4. Do not change key bindings, tab behavior, scroll behavior, OAuth message
   handling, account actions, pruning behavior, subscription usage refresh,
   snapshot reload behavior, error text, logging event names, or management
   client calls.
5. Do not add permanent tests.
6. Do not push.

## Non-Goals

- No dispatcher refactor within this slice.
- No visual redesign.
- No management API, provider, storage, or config changes.
- No account, OAuth, observability, snapshot, render, or model-state changes.
- No TUI dependency changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/update.go` with the same `package tui`.
2. Move `Update` from `tui.go` into the new file.
3. Add only the imports needed by the moved dispatcher.
4. Remove now-unused imports from `tui.go`.
5. Run `gofmt`.
6. Review the diff to confirm it is move-only apart from imports.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
cfg="$tmp/config.toml"
cat >"$cfg" <<EOF
[server]
bind = "127.0.0.1:0"
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" &
pid="$!"
for _ in $(seq 1 80); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] && curl --silent --fail --unix-socket "$sock" \
    http://ilonasin/_ilonasin/manage/snapshot >/dev/null && \
    curl --silent --fail --unix-socket "$sock" \
    http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null; then
    break
  fi
  sleep 0.1
done
set +e
timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  exit "$manage_status"
fi
kill "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
git diff --check
```

Acceptance:

- Compile/package check passes.
- Vet passes.
- Fresh binary builds.
- Direct `serve` smoke starts the daemon and exposes snapshot and subscription
  usage management routes.
- Direct `manage` smoke runs in a pseudo-terminal, reaches the daemon-backed
  TUI path, and exits cleanly or times out with status 124. Any other status
  fails the smoke.
- `git diff --check` passes.

## Review Questions

1. Is moving `Update` into `update.go` the right next boundary now that model
   state and helpers are split?
2. Should `Run` remain alone in `tui.go` as the package entrypoint for this
   behavior-preserving slice?
3. Is the smoke coverage sufficient for a move-only dispatcher split?

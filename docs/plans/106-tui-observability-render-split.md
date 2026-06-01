# 106 TUI Observability Render Split

## Context

`internal/tui/tui.go` is now over 1,500 lines. It contains model state,
keyboard handling, layout, account rendering, observability rendering, pruning
rendering, OAuth helpers, sanitizers, and logging helpers. Plan 103 made the TUI
tabbed and scrollable, and Plan 104 added subscription usage rendering to the
observability tab. The TUI is moving toward a first-class management interface,
so render code should be modular enough to evolve without crowding event and
state logic.

The observability tab is the largest rendering cluster and is mostly isolated:
recent requests, usage totals, latency, stream, health, quota, subscription
usage, subscription pools, keepalive status, fallbacks, and telemetry pruning.

## Goal

Move observability and pruning rendering out of `internal/tui/tui.go` into a
dedicated same-package file without changing behavior.

After this slice, `tui.go` remains responsible for model state, update logic,
tab layout, account rendering, and shared helpers. Observability rendering lives
in `internal/tui/observability.go`.

## Scope

1. Create `internal/tui/observability.go`.
2. Move these methods intact:
   - `writeObservability`
   - `writePruning`
3. Keep method receivers and helper calls unchanged.
4. Keep `pruneTelemetry` in `tui.go` for this slice because it is update/action
   logic, not rendering.
5. Do not change TUI text, key bindings, layout, scrolling, snapshot data
   handling, management clients, subscription usage refresh behavior, or
   sanitization helpers.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No new TUI dependency.
- No management API, provider, storage, or config changes.
- No split of account rendering in this slice.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/observability.go` with the same `package tui`.
2. Move `writeObservability` and `writePruning` from `tui.go` into the new file.
3. Add only the imports needed by the moved methods.
4. Run `gofmt`.
5. Review the diff to confirm it is a move-only render split.

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
    http://ilonasin/_ilonasin/manage/snapshot >/dev/null; then
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
- Direct `serve` smoke starts the daemon and exposes the management snapshot
  route.
- Direct `manage` smoke runs in a pseudo-terminal through `script`, reaches the
  daemon-backed TUI path, and exits cleanly or times out with status 124. Any
  other status fails the smoke.
- `git diff --check` passes.

## Review Questions

1. Is observability rendering the right next TUI cluster to split first?
2. Should `pruneTelemetry` remain in `tui.go` for now because it is action
   logic?
3. Is a move-only render split worth doing before larger TUI polish work?

# 108 TUI Layout Render Split

## Context

Plans 106 and 107 moved observability and account rendering out of
`internal/tui/tui.go`. The main TUI file still contains model state,
keyboard/action logic, daemon management calls, snapshot application, OAuth
commands, sanitizers, logging helpers, and the tabbed viewport/layout
rendering from Plan 103.

The architecture says `ilonasin manage` is a first-class local management UI
that talks through the daemon-owned management API. Keeping layout and
viewport rendering separate from management mutations makes the UI easier to
evolve without mixing display mechanics with credential and telemetry actions.

## Goal

Move tab, viewport, status, and scroll rendering helpers out of
`internal/tui/tui.go` into a dedicated same-package file without changing
behavior.

After this slice, `tui.go` still owns state, update/action logic, snapshot
loading, daemon management calls, OAuth commands, sanitizers, and logging
helpers. Layout and viewport rendering lives in `internal/tui/layout.go`.

## Scope

1. Create `internal/tui/layout.go`.
2. Move these functions and methods intact:
   - `View`
   - `activeTabBody`
   - `tabBody`
   - `tabBar`
   - `statusLine`
   - `footerLine`
   - `renderViewport`
   - `splitBodyLines`
   - `viewWidth`
   - `viewHeight`
   - `viewportHeight`
   - `validActiveTab`
   - `activeScrollMax`
   - `scrollMax`
   - `scrollActive`
   - `setActiveScroll`
   - `clampScrolls`
   - `clipPlainLine`
   - `maxInt`
3. Keep `writeOverview` and `writeHelp` in `tui.go` for this slice. They are
   small render clusters and can be split later if needed.
4. Keep keyboard handling, account mutations, OAuth refresh/login, telemetry
   pruning, snapshot loading, and sanitizers in `tui.go`.
5. Do not change TUI text, key bindings, tab behavior, scrolling behavior,
   snapshot data handling, management clients, or sanitization helpers.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No new TUI dependency.
- No management API, provider, storage, or config changes.
- No split of update/action logic in this slice.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/layout.go` with the same `package tui`.
2. Move the listed layout and viewport functions from `tui.go` into the new
   file.
3. Add only the imports needed by the moved functions.
4. Run `gofmt`.
5. Review the diff to confirm it is a move-only layout split.

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
- Direct `manage` smoke runs in a pseudo-terminal, reaches the daemon-backed
  TUI path, and exits cleanly or times out with status 124. Any other status
  fails the smoke.
- `git diff --check` passes.

## Review Questions

1. Is layout and viewport rendering the right next TUI cluster to split after
   account and observability rendering?
2. Should `writeOverview` and `writeHelp` remain in `tui.go` for this
   behavior-preserving slice?
3. Is this move-only split useful before larger TUI visual polish or action
   modularization?

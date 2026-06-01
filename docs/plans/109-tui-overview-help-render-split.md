# 109 TUI Overview Help Render Split

## Context

Plans 106, 107, and 108 split observability, account, and layout rendering out
of `internal/tui/tui.go`. The main TUI file still owns two render-only methods:
`writeOverview` and `writeHelp`. It also owns model state, update/action logic,
daemon management calls, snapshot application, OAuth commands, sanitizers,
logging helpers, and account-management actions.

The architecture says `ilonasin manage` is a first-class local management UI
that talks through the daemon-owned management API. Moving the remaining
render-only overview/help code out of the action file keeps rendering concerns
separate from management mutations and continues reducing the legacy large TUI
file.

## Goal

Move overview and help rendering out of `internal/tui/tui.go` into a dedicated
same-package file without changing behavior.

After this slice, `tui.go` still owns state, update/action logic, snapshot
loading, daemon management calls, OAuth commands, sanitizers, logging helpers,
and management action helpers. Rendering is split across focused TUI files.

## Scope

1. Create `internal/tui/overview.go`.
2. Move these functions and types intact:
   - `writeOverview`
   - `writeHelp`
   - `modelCacheSummary`
   - `modelCacheSummaries`
3. Keep method receivers and helper calls unchanged.
4. Keep keyboard handling, account mutations, OAuth refresh/login, telemetry
   pruning, snapshot loading, sanitizers, and logging in `tui.go`.
5. Do not change TUI text, tab behavior, scrolling behavior, snapshot data
   handling, management clients, or sanitization helpers.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No new TUI dependency.
- No management API, provider, storage, or config changes.
- No split of update/action logic in this slice.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/overview.go` with the same `package tui`.
2. Move `writeOverview`, `writeHelp`, `modelCacheSummary`, and
   `modelCacheSummaries` from `tui.go` into the new file.
3. Add only the imports needed by the moved code.
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
- Direct `manage` smoke runs in a pseudo-terminal, reaches the daemon-backed
  TUI path, and exits cleanly or times out with status 124. Any other status
  fails the smoke.
- `git diff --check` passes.

## Review Questions

1. Is moving overview/help rendering the right final render-only split for the
   current TUI modularization sequence?
2. Should `modelCacheSummaries` move with `writeOverview` because it is only
   used there?
3. Is this move-only split useful before larger TUI action or sanitizer
   modularization?

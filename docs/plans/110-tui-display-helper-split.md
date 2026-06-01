# 110 TUI Display Helper Split

## Context

Plans 106 through 109 split TUI rendering into focused files:
observability, accounts, layout, and overview/help. `internal/tui/tui.go` still
contains shared display helpers and sanitizers alongside model state,
keyboard/action logic, daemon management calls, snapshot application, OAuth
commands, and management action helpers.

Those display helpers are used by render files to enforce the architecture's
metadata-only and redacted-output requirements. Keeping them with rendering
support code makes the safety boundary easier to audit, while leaving
management mutations and event handling in `tui.go`.

## Goal

Move shared TUI display, formatting, and redaction helpers out of
`internal/tui/tui.go` into a dedicated same-package file without changing
behavior.

After this slice, `tui.go` still owns model state, update/action logic,
snapshot loading, daemon management calls, OAuth commands, pruning actions,
logging helpers, and management action helpers. Shared display safety helpers
live in `internal/tui/display.go`.

## Scope

1. Create `internal/tui/display.go`.
2. Move these functions and variables intact:
   - `credentialDisplay`
   - `healthModelDisplay`
   - `requestModelDisplay`
   - `formatTime`
   - `formatPreciseTime`
   - `unsafeDisplayPattern`
   - `safeErrorMessagePattern`
   - `safeDisplay`
   - `safeTokenFragmentDisplay`
   - `safeEndpointDisplay`
   - `safeRefreshFailureDescriptionDisplay`
   - `safeRefreshFailureClass`
3. Keep method receivers and helper calls unchanged.
4. Keep `safeErrorMessage` in `tui.go` for this slice because it is used by
   action/OAuth error handling as well as layout status text.
5. Keep `tuiErrorClass`, `logInfo`, `logError`, `firstLogger`, and `nowTime`
   in `tui.go` for this slice because they are logging/time/action support,
   not render display helpers.
6. Do not change sanitizer rules, displayed text, tab behavior, scrolling
   behavior, management clients, snapshot data handling, or action logic.
7. Do not add permanent tests.
8. Do not push.

## Non-Goals

- No visual redesign.
- No new TUI dependency.
- No management API, provider, storage, or config changes.
- No split of update/action logic in this slice.
- No sanitizer policy change in this slice.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/display.go` with the same `package tui`.
2. Move the listed display helpers from `tui.go` into the new file.
3. Add only the imports needed by the moved helpers.
4. Run `gofmt`.
5. Review the diff to confirm it is move-only.

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

1. Is a dedicated display/redaction helper file the right next boundary after
   splitting render files?
2. Should `safeErrorMessage` stay in `tui.go` for now because it participates
   in action/OAuth error handling?
3. Is this move-only split useful before larger TUI action modularization?

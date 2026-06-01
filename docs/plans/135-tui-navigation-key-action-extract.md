# 135 TUI Navigation Key Action Extract

## Context

Plans 103 and 106 through 134 split TUI rendering, model state, lifecycle,
shared helpers, update routing, key dispatch, account actions, OAuth actions,
observability actions, and global actions into focused files.
`internal/tui/update_keys.go` still contains inline navigation behavior for:

- tab switching,
- page, home, and end scrolling,
- up/down row selection or scrolling.

Plan 103 introduced the tabbed and scrollable management TUI. The architecture
says `ilonasin manage` is a first-class local management UI and must not mutate
`config.toml`. Keeping navigation behavior in a focused action file makes the
key dispatcher more declarative while preserving current account-tab selection
semantics.

## Goal

Move navigation key action bodies out of `internal/tui/update_keys.go` into
`internal/tui/navigation_actions.go` without changing behavior.

After this slice, `update_keys.go` keeps key strings and tab-scoped action
dispatch. `navigation_actions.go` owns tab switching, scroll jumps, page
scrolling, and up/down navigation.

## Scope

1. Add `internal/tui/navigation_actions.go`.
2. Add these methods:
   - `nextTabAction`
   - `previousTabAction`
   - `selectTabAction`
   - `pageDownAction`
   - `pageUpAction`
   - `homeAction`
   - `endAction`
   - `downAction`
   - `upAction`
3. Move the bodies of the `tab`/`right`, `shift+tab`/`left`, `1`, `2`, `3`,
   `4`, `pgdown`/`ctrl+d`, `pgup`/`ctrl+u`, `home`, `end`, `down`/`j`, and
   `up`/`k` key cases into those methods.
4. Preserve account-tab local-token selection for up/down and scrolling for
   other tabs.
5. Keep key strings, command return values, reveal clearing, scroll clamping,
   tab IDs, and selection behavior unchanged.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No behavior changes.
- No visual redesign.
- No management API, provider, storage, or config changes.
- No account, OAuth, observability, or global action changes.
- No broader key-dispatch redesign beyond navigation extraction.
- No TUI dependency changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `navigation_actions.go` with `package tui`.
2. Move navigation action bodies into the new methods.
3. Replace each key case body with a method call.
4. Run `gofmt`.
5. Review the diff to confirm behavior is unchanged.

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

1. Are navigation and selection keys the right next extraction after global
   actions?
2. Should account-tab up/down selection stay in the navigation action body, or
   should it stay inline in `update_keys.go`?
3. Is `navigation_actions.go` the right boundary for tab and scroll behavior?
4. Is the smoke coverage sufficient for this focused action extraction?

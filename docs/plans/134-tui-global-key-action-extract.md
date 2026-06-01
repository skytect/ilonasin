# 134 TUI Global Key Action Extract

## Context

Plans 103 and 106 through 133 split TUI rendering, model state, lifecycle,
shared helpers, update routing, key dispatch, account actions, OAuth actions,
and observability actions into focused files. `internal/tui/update_keys.go`
still contains inline global key action bodies for:

- quitting the TUI,
- canceling visible OAuth/login state with `esc`,
- clearing reveal state with `enter`.

These are cross-tab lifecycle actions rather than account or observability
operations. Keeping them as methods makes the key dispatcher more declarative
while preserving the existing behavior. The architecture still requires the TUI
to remain a daemon-management client and not mutate `config.toml`.

## Goal

Move global key action bodies out of `internal/tui/update_keys.go` into a
focused same-package file without changing behavior.

After this slice, `update_keys.go` keeps key strings and dispatch. A new
`internal/tui/global_actions.go` owns global quit/cancel/clear action bodies.

## Scope

1. Add `internal/tui/global_actions.go`.
2. Add these methods:
   - `quitAction`
   - `cancelVisibleAction`
   - `clearRevealAction`
3. Move the bodies of the `q`/`ctrl+c`, `esc`, and `enter` key cases into
   those methods.
4. Keep key strings, command return values, reveal clearing, OAuth challenge
   clearing, and OAuth cancellation semantics unchanged.
5. Do not move navigation, scrolling, or selection keys in this slice.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No behavior changes.
- No visual redesign.
- No management API, provider, storage, or config changes.
- No account, OAuth, or observability action changes.
- No broader key-dispatch redesign.
- No TUI dependency changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `global_actions.go` with `package tui`.
2. Move global action bodies into the new methods.
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

1. Are `q`/`ctrl+c`, `esc`, and `enter` the right global action extraction
   after account, OAuth, and observability key actions?
2. Should navigation and selection keys remain in `update_keys.go` for a later
   focused slice?
3. Is `global_actions.go` the right boundary for cross-tab TUI lifecycle
   actions?
4. Is the smoke coverage sufficient for this focused action extraction?

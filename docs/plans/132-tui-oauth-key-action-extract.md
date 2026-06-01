# 132 TUI OAuth Key Action Extract

## Context

Plans 103 and 106 through 131 split TUI rendering, model state, lifecycle,
shared helpers, update routing, key dispatch, local-token actions, and account
key actions into focused files. `internal/tui/update_keys.go` still contains
inline OAuth key action bodies for:

- starting an OAuth device login,
- refreshing the selected OAuth credential,
- cycling the selected OAuth credential row.

The architecture says account and OAuth mutations must go through daemon-owned
management APIs and the TUI must not mutate `config.toml`. The current inline
key cases already use the `management.OAuthClient`; moving these action bodies
into `oauth_actions.go` keeps the key dispatcher as dispatch and makes OAuth
behavior easier to audit.

## Goal

Move remaining OAuth account key action bodies out of
`internal/tui/update_keys.go` into `internal/tui/oauth_actions.go` without
changing behavior.

After this slice, `update_keys.go` keeps key strings and tab guards.
`oauth_actions.go` owns OAuth login, refresh, cancellation, error formatting,
and OAuth key action bodies.

## Scope

1. Add these methods to `internal/tui/oauth_actions.go`:
   - `startOAuthLoginAction`
   - `refreshSelectedOAuthCredentialAction`
   - `cycleOAuthSelectionAction`
2. Move the bodies of the `l`, `r`, and `o` key cases into those methods.
3. Keep the `accounts` tab guards in `update_keys.go`.
4. Keep key strings, command return values, reveal clearing, error text,
   logging event names, reload behavior, management requests, and OAuth
   cancellation semantics unchanged.
5. Do not move `esc` in this slice because it clears both reveal state and the
   visible challenge before canceling OAuth.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No behavior changes.
- No visual redesign.
- No management API, provider, storage, or config changes.
- No pruning or subscription usage extraction in this slice.
- No broader key-dispatch redesign.
- No TUI dependency changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add the three OAuth key action methods near existing OAuth helpers.
2. Replace each key case body with a call to its new method.
3. Remove now-unused imports from `update_keys.go`.
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

1. Are `l`, `r`, and `o` the right OAuth key action extraction after account
   key actions?
2. Should the `accounts` tab guards remain in `update_keys.go` for this slice?
3. Should `esc` stay in the dispatcher until a broader global-cancel action is
   designed?
4. Is the smoke coverage sufficient for this focused OAuth action extraction?

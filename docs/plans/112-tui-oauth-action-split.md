# 112 TUI OAuth Action Split

## Context

Plans 106 through 111 split rendering, display helpers, and account-tab
actions out of `internal/tui/tui.go`. The main TUI file still contains OAuth
device-login message types and OAuth helper methods alongside model state,
the Bubble Tea event dispatcher, snapshot loading, observability actions, and
logging helpers.

The architecture says OAuth login and refresh are first-class management TUI
operations and must go through daemon-owned management APIs. The current OAuth
helpers already call the management `OAuthClient`; moving them into a focused
same-package file makes the auth boundary easier to audit without changing
behavior.

## Goal

Move OAuth-specific TUI action helpers out of `internal/tui/tui.go` into a
dedicated same-package file without changing behavior.

After this slice, `tui.go` still owns model state, `Update`, `Run`, snapshot
loading/application, observability pruning, subscription usage refresh,
logging helpers, and shared lifecycle code. OAuth message types and OAuth
action helpers live in `internal/tui/oauth_actions.go`.

## Scope

1. Create `internal/tui/oauth_actions.go`.
2. Move these types, functions, and methods intact:
   - `oauthLoginStartedMsg`
   - `oauthLoginCompletedMsg`
   - `oauthChallengeFromManagement`
   - `startOAuthLoginCmd`
   - `completeOAuthLoginCmd`
   - `cancelOAuthLogin`
   - `refreshSelectedOAuthCredential`
   - `firstOAuthLoginProvider`
   - `oauthLoginErrorMessage`
3. Keep method receivers and helper calls unchanged.
4. Keep the `Update` cases that consume `oauthLoginStartedMsg` and
   `oauthLoginCompletedMsg` in `tui.go` for this slice.
5. Do not change OAuth login, refresh, cancellation, error text, logging,
   management client calls, account selection, TUI text, or sanitization.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No OAuth behavior change.
- No management API, provider, storage, or config changes.
- No visual redesign.
- No split of the `Update` dispatcher in this slice.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/oauth_actions.go` with the same `package tui`.
2. Move the listed OAuth types and helpers from `tui.go` into the new file.
3. Add only the imports needed by the moved code.
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

1. Are OAuth action helpers the right next boundary after account action
   helpers?
2. Should the `Update` cases remain in `tui.go` for this behavior-preserving
   slice?
3. Is this move-only split useful before larger `Update` dispatcher
   modularization?

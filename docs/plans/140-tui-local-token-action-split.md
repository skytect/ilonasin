# 140 TUI Local Token Action Split

## Context

Plans 103 and 106 through 139 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, API-key input mode, OAuth actions, fallback
actions, observability actions, global actions, navigation actions, and
tab-specific key dispatch. `internal/tui/account_actions.go` still owns
local-token create/disable actions and local-token selection helpers alongside
upstream credential visibility and disable behavior.

Local API tokens are a first-class management TUI account workflow. The
architecture says local API token mutations must go through the daemon-owned
management API and must not expose full tokens in the TUI. Moving local-token
action helpers into a focused file makes that boundary easier to audit without
changing behavior.

## Goal

Move local-token TUI helpers out of `internal/tui/account_actions.go` into a
focused same-package file without changing behavior.

After this slice:

- `account_local_token_actions.go` owns local-token reveal clearing,
  local-token selection, local-token creation, and local-token disabling.
- `account_actions.go` keeps account-tab key dispatch, upstream credential
  disabling, and account visibility helpers.

## Scope

1. Add `internal/tui/account_local_token_actions.go`.
2. Move these functions unchanged:
   - `clearReveal`
   - `selectNextLocalToken`
   - `selectPreviousLocalToken`
   - `createLocalToken`
   - `disableSelectedLocalToken`
3. Preserve key behavior, command return values, reveal metadata behavior,
   error text, logging event names, reload behavior, and management requests
   unchanged.
4. Do not move upstream credential disable or account visibility helpers in
   this slice.
5. Do not add permanent tests.
6. Do not push.

## Non-Goals

- No behavior changes.
- No visual redesign.
- No management API, provider, storage, or config changes.
- No local-token generation or rendering changes.
- No TUI dependency changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `account_local_token_actions.go` with `package tui`.
2. Move the listed functions from `account_actions.go`.
3. Keep imports minimal in both files.
4. Run `gofmt`.
5. Review the diff to confirm this is relocation only.

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

1. Is local-token behavior the right next split from `account_actions.go`?
2. Should `clearReveal` move with local-token reveal metadata, even though
   other action files call it?
3. Is the smoke coverage sufficient for this relocation-only split?

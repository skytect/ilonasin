# 154 TUI OAuth Render Split

## Context

Plans 103 and 106 through 153 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, observability render sections, pruning rendering, help rendering,
and overview render sections.

`internal/tui/accounts.go` still owns several account-tab render sections:
local API tokens, upstream credentials, credential groups, OAuth accounts, and
provider accounts. OAuth account rendering is a distinct credential display
boundary tied to daemon-owned OAuth management APIs and redacted provider
account metadata. Moving it into a focused render file makes the account tab
easier to audit without changing behavior.

## Goal

Move OAuth and provider-account rendering out of `accounts.go` into a focused
same-package render file without changing behavior.

After this slice:

- `accounts.go` keeps the account tab sequence, local token rendering,
  upstream credential rendering, and fallback policy rendering.
- `accounts_oauth.go` owns OAuth account and provider account rendering.

## Scope

1. Add `internal/tui/accounts_oauth.go`.
2. Move the existing `writeOAuth` method from `accounts.go` into the new file
   unchanged.
3. Preserve all output strings, ordering, nil checks, OAuth challenge display,
   selection cursor behavior, state labels, expiry formatting, refresh failure
   display, provider account rows, and safe display helpers.
4. Keep `writeAccounts` section order unchanged.
5. Do not change OAuth actions, account actions, management DTOs, snapshot
   loading, storage, provider adapters, config, routing, key handling, or TUI
   actions.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No OAuth behavior or management API changes.
- No storage, provider, config, routing, or action changes.
- No changes to local token, upstream credential, fallback policy,
  observability, overview, help, or layout rendering.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/accounts_oauth.go` with `package tui`.
2. Move `writeOAuth` unchanged from `accounts.go`.
3. Add only the imports needed by moved code.
4. Remove any now-unused imports from `accounts.go`.
5. Run `gofmt`.
6. Review the diff to confirm this is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the accounts tab still renders
   `OAuth accounts`.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
cleanup() {
  if [ -n "${pid:-}" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
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
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" \
  >"$tmp/serve.log" 2>&1 &
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
if [ -z "${sock:-}" ]; then
  echo "management socket not found"
  cat "$tmp/serve.log"
  exit 1
fi
curl --silent --fail --unix-socket "$sock" \
  http://ilonasin/_ilonasin/manage/snapshot >/dev/null
curl --silent --fail --unix-socket "$sock" \
  http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null
set +e
printf '\tq' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage-accounts.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage-accounts.typescript" 2>/dev/null || true
  exit "$manage_status"
fi
if ! grep -q "Local API tokens" "$tmp/manage-accounts.typescript" ||
  ! grep -q "OAuth accounts" "$tmp/manage-accounts.typescript" ||
  ! grep -q "Provider accounts" "$tmp/manage-accounts.typescript"; then
  echo "accounts oauth render smoke failed"
  cat "$tmp/manage-accounts.typescript"
  exit 1
fi
git diff --check
```

Acceptance:

- Compile/package check passes.
- Vet passes.
- Existing permanent test-file inventory is reviewed.
- Fresh binary builds.
- Direct `serve` smoke starts the daemon and exposes snapshot and subscription
  usage management routes.
- Direct `manage` smoke runs in a pseudo-terminal, navigates to the accounts
  tab, renders OAuth and provider account sections, and exits cleanly or times
  out with status 124. Any other status fails the smoke.
- `git diff --check` passes.
- Moved-code diff review confirms `writeOAuth` is unchanged except for the new
  file location and imports.

## Review Questions

1. Is OAuth/provider-account rendering the right next split from
   `accounts.go`?
2. Should provider account rendering move with OAuth rendering because it is
   derived from provider account metadata?
3. Is the accounts-tab PTY smoke sufficient for this relocation-only split?

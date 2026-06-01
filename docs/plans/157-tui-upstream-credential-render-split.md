# 157 TUI Upstream Credential Render Split

## Context

Plans 103 and 106 through 156 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, observability render sections, pruning rendering, help rendering,
overview render sections, OAuth account rendering, credential group rendering,
and local API token rendering.

`internal/tui/accounts.go` now owns the account-tab section sequence, API-key
input prompt, and upstream credential rendering. Upstream credential mutation
logic already lives in `account_api_key_actions.go` and
`account_upstream_actions.go`. Moving upstream credential display into a
focused render file leaves `accounts.go` as a thin account-tab composer and
keeps display code grouped by account section.

## Goal

Move upstream credential rendering out of `accounts.go` into a focused
same-package render file without changing behavior.

After this slice:

- `accounts.go` composes the account tab sections in order.
- `accounts_upstreams.go` owns API-key input prompt display and upstream
  credential rendering.
- `account_api_key_actions.go` and `account_upstream_actions.go` continue to
  own upstream credential mutations and visibility helpers.

## Scope

1. Add `internal/tui/accounts_upstreams.go`.
2. Move API-key input prompt display and upstream credential rendering from
   `writeAccounts` into a new `writeUpstreamCredentials` method.
3. Preserve all output strings, ordering, empty-state text, enabled/disabled
   labels, redacted input display, secret prefix/last4 display, fallback group
   display, and safe display helpers.
4. Keep `writeAccounts` section order unchanged by calling:
   - `m.writeLocalTokens(b)`
   - `m.writeUpstreamCredentials(b)`
   - `m.writeFallbackPolicies(b)`
   - `m.writeOAuth(b)`
5. Do not change account actions, management DTOs, snapshot loading, storage,
   provider adapters, config, routing, key handling, or TUI actions.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No upstream credential behavior or management API changes.
- No storage, provider, config, routing, or action changes.
- No changes to local token, credential group, OAuth, provider account,
  observability, overview, help, or layout rendering.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/accounts_upstreams.go` with `package tui`.
2. Move API-key input prompt display and upstream credential render code from
   `writeAccounts` into `writeUpstreamCredentials`.
3. Add only the imports needed by moved code.
4. Remove any now-unused imports from `accounts.go`.
5. Run `gofmt`.
6. Review the diff to confirm this is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the accounts tab still renders
   `Local API tokens`, `Upstream credentials`, and `Credential groups`.

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
cat >"$cfg" <<'EOF'
[server]
bind = "127.0.0.1:0"
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" >"$tmp/serve.log" 2>&1 &
pid="$!"
for _ in $(seq 1 80); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null; then
    break
  fi
  sleep 0.1
done
if [ -z "${sock:-}" ]; then
  echo "management socket not found"
  cat "$tmp/serve.log"
  exit 1
fi
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null
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
  ! grep -q "Upstream credentials" "$tmp/manage-accounts.typescript" ||
  ! grep -q "Credential groups" "$tmp/manage-accounts.typescript"; then
  echo "accounts upstream credential render smoke failed"
  cat "$tmp/manage-accounts.typescript"
  exit 1
fi
git diff --check
```

## Acceptance

- `writeAccounts` is a thin composer and still renders account sections in the
  same order.
- `accounts_upstreams.go` owns API-key input prompt display and upstream
  credential display.
- Existing TUI upstream credential actions remain unchanged.
- Direct compile, vet, serve, management route, and PTY manage smokes pass.

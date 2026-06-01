# 156 TUI Local Token Render Split

## Context

Plans 103 and 106 through 155 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, observability render sections, pruning rendering, help rendering,
overview render sections, OAuth account rendering, and credential group
rendering.

`internal/tui/accounts.go` still owns several account-tab render sections. Local
API token mutation logic already lives in `account_local_token_actions.go`.
Moving local API token rendering into a focused same-package render file keeps
display code grouped by account section and reduces the remaining composition
surface in `accounts.go`.

## Goal

Move local API token rendering out of `accounts.go` into a focused render file
without changing behavior.

After this slice:

- `accounts.go` keeps the account tab section sequence and upstream credential
  rendering.
- `accounts_local_tokens.go` owns local API token rendering and new-token
  reveal display.
- `account_local_token_actions.go` continues to own local API token mutations.

## Scope

1. Add `internal/tui/accounts_local_tokens.go`.
2. Move the local API token block from `writeAccounts` into a new
   `writeLocalTokens` method.
3. Preserve all output strings, ordering, cursor behavior, empty-state text,
   enabled/disabled labels, safe token fragment display, and new-token reveal
   display.
4. Keep `writeAccounts` section order unchanged by calling `m.writeLocalTokens`
   before upstream credentials.
5. Do not move API-key input rendering in this slice because it belongs with
   upstream credential addition, not local API token display.
6. Do not change account actions, management DTOs, snapshot loading, storage,
   provider adapters, config, routing, key handling, or TUI actions.
7. Do not add permanent tests.
8. Do not push.

## Non-Goals

- No visual redesign.
- No local token behavior or management API changes.
- No storage, provider, config, routing, or action changes.
- No changes to upstream credential, credential group, OAuth, provider account,
  observability, overview, help, or layout rendering.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/accounts_local_tokens.go` with `package tui`.
2. Move the local API token render block from `writeAccounts` into
   `writeLocalTokens`.
3. Add only the imports needed by moved code.
4. Remove any now-unused imports from `accounts.go`.
5. Run `gofmt`.
6. Review the diff to confirm this is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the accounts tab still renders
   `Local API tokens`.

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
  echo "accounts local token render smoke failed"
  cat "$tmp/manage-accounts.typescript"
  exit 1
fi
git diff --check
```

## Acceptance

- `writeAccounts` still renders the same account tab sections in the same
  order.
- `accounts_local_tokens.go` owns local API token display.
- Existing TUI account actions remain unchanged.
- Direct compile, vet, serve, management route, and PTY manage smokes pass.

# 130 TUI Local Token Action Extract

## Context

Plans 103 and 106 through 129 split TUI rendering, model state, lifecycle,
shared helpers, top-level update routing, and key dispatch into focused files.
`internal/tui/update_keys.go` still contains inline local-token create and
disable mutations for the `n` and `d` keys.

Plan 111 explicitly left those cases inline for a later action-dispatch slice.
The architecture says account mutations must go through the daemon-owned
management API and the TUI must not mutate `config.toml`. The current inline
code already uses the `management.LocalTokenClient`; moving it into account
action methods makes that boundary easier to audit without changing behavior.

## Goal

Move local-token create and disable action logic out of `update_keys.go` into
`internal/tui/account_actions.go` without changing behavior.

After this slice, `update_keys.go` keeps key guards and dispatch. Local-token
management action bodies live with the other account actions.

## Scope

1. Add two methods to `internal/tui/account_actions.go`:
   - `createLocalToken`
   - `disableSelectedLocalToken`
2. Move the bodies of the `n` and `d` key cases into those methods intact.
3. Keep the `accounts` tab guards in `update_keys.go`.
4. Keep key strings, reveal metadata behavior, reloads, error text, logging
   event names, management requests, and return values unchanged.
5. Do not add permanent tests.
6. Do not push.

## Non-Goals

- No local-token behavior changes.
- No visual redesign.
- No management API, provider, storage, or config changes.
- No broader key dispatcher split.
- No OAuth, upstream credential, fallback, pruning, or subscription usage
  action changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add the two methods near existing local token helpers in
   `account_actions.go`.
2. Replace the `n` key body with `return m.createLocalToken()`.
3. Replace the `d` key body with `return m.disableSelectedLocalToken()`.
4. Remove now-unused imports from `update_keys.go`.
5. Run `gofmt`.
6. Review the diff to confirm behavior is unchanged.

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

1. Are local-token create and disable the right next account action extraction?
2. Should the `accounts` tab guards remain in `update_keys.go` for this slice?
3. Is the smoke coverage sufficient for this focused account action extraction?

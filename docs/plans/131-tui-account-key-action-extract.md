# 131 TUI Account Key Action Extract

## Context

Plans 103 and 106 through 130 split TUI rendering, model state, lifecycle,
shared helpers, update routing, key dispatch, and local-token account actions
into focused files. `internal/tui/update_keys.go` still contains inline account
action bodies for:

- disabling the first upstream credential,
- entering API-key input mode,
- enabling the first fallback policy,
- disabling the first fallback policy.

The architecture says account mutations must go through daemon-owned
management APIs and the TUI must not mutate `config.toml`. The current inline
key cases already delegate to account helpers and management clients; moving
the remaining action bodies into `account_actions.go` keeps the key dispatcher
as dispatch and makes account mutations easier to audit.

## Goal

Move remaining non-OAuth account key action bodies out of
`internal/tui/update_keys.go` into `internal/tui/account_actions.go` without
changing behavior.

After this slice, `update_keys.go` keeps key strings and tab guards.
`account_actions.go` owns account mutation and account-mode action bodies.

## Scope

1. Add these methods to `internal/tui/account_actions.go`:
   - `disableUpstreamCredentialAction`
   - `startAPIKeyInput`
   - `enableFallbackPolicyAction`
   - `disableFallbackPolicyAction`
2. Move the bodies of the `x`, `a`, `f`, and `F` key cases into those methods.
3. Keep the `accounts` tab guards in `update_keys.go`.
4. Keep key strings, command return values, reveal clearing, error text,
   logging event names, reload behavior, management requests, and fallback
   policy semantics unchanged.
5. Do not add permanent tests.
6. Do not push.

## Non-Goals

- No behavior changes.
- No visual redesign.
- No management API, provider, storage, or config changes.
- No OAuth action extraction in this slice.
- No pruning or subscription usage extraction in this slice.
- No TUI dependency changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add the four action methods near existing account action helpers.
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

1. Are `x`, `a`, `f`, and `F` the right next account action extraction after
   local-token actions?
2. Should the `accounts` tab guards remain in `update_keys.go` for this slice?
3. Is the smoke coverage sufficient for this focused account action extraction?

# 138 TUI API Key Action Split

## Context

Plans 103 and 106 through 137 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, observability
actions, global actions, navigation actions, and tab-specific key dispatch.
`internal/tui/account_actions.go` is now the largest TUI file and mixes local
token actions, API-key input mode, upstream credential actions, fallback policy
actions, and account visibility helpers.

The architecture says account mutations must go through daemon-owned
management APIs and the TUI must not mutate `config.toml`. API-key entry is an
account-tab workflow, but it is its own transient input mode and can be split
from the broader account action file without changing behavior.

## Goal

Move API-key input mode helpers out of `internal/tui/account_actions.go` into a
focused same-package file without changing behavior.

After this slice:

- `account_actions.go` continues to own account-tab dispatch, local-token
  actions, upstream credential actions, fallback actions, and account
  visibility helpers.
- `account_api_key_actions.go` owns `updateAPIKeyInput`, `startAPIKeyInput`,
  `clearAPIKeyInput`, and `firstAPIKeyProvider`.

## Scope

1. Add `internal/tui/account_api_key_actions.go`.
2. Move these functions unchanged:
   - `updateAPIKeyInput`
   - `startAPIKeyInput`
   - `clearAPIKeyInput`
   - `firstAPIKeyProvider`
3. Preserve key handling, command return values, error text, logging event
   names, reload behavior, management requests, provider selection, and
   API-key masking behavior unchanged.
4. Do not move upstream credential disable or fallback policy helpers in this
   slice.
5. Do not add permanent tests.
6. Do not push.

## Non-Goals

- No behavior changes.
- No visual redesign.
- No management API, provider, storage, or config changes.
- No change to API-key storage or validation semantics.
- No TUI dependency changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `account_api_key_actions.go` with `package tui`.
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

1. Is API-key input mode the right next split from the large account actions
   file?
2. Should `firstAPIKeyProvider` move with API-key input mode rather than stay
   beside account visibility helpers?
3. Is the smoke coverage sufficient for this relocation-only split?

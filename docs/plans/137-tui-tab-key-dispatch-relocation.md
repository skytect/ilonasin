# 137 TUI Tab Key Dispatch Relocation

## Context

Plans 103 and 106 through 136 split TUI rendering, model state, lifecycle,
shared helpers, update routing, key dispatch, key action bodies, and tab-scoped
dispatch. `internal/tui/update_keys.go` now delegates account and
observability keys through `updateAccountKey` and `updateObservabilityKey`, but
those helpers still live in the top-level key dispatcher file.

The architecture says `ilonasin manage` is the local management control plane,
talks through daemon-owned management APIs, and must not mutate `config.toml`.
The account and observability key helpers are domain dispatch, so placing them
next to their domain action bodies keeps boundaries easier to audit.

## Goal

Move tab-specific key dispatch helpers out of `internal/tui/update_keys.go`
into their domain action files without changing behavior.

After this slice:

- `update_keys.go` owns top-level keyboard orchestration,
- `account_actions.go` owns account-tab key dispatch and account actions,
- `observability_actions.go` owns observability-tab key dispatch and
  observability actions.

## Scope

1. Move `updateAccountKey` from `update_keys.go` to `account_actions.go`.
2. Move `updateObservabilityKey` from `update_keys.go` to
   `observability_actions.go`.
3. Preserve helper signatures, key strings, handled-flag behavior, tab guard
   behavior, command return values, action methods, error text, logging event
   names, reload behavior, and management requests unchanged.
4. Keep `updateKey` call order unchanged: API-key input first, then global and
   navigation keys, then account dispatch, then observability dispatch.
5. Do not add permanent tests.
6. Do not push.

## Non-Goals

- No behavior changes.
- No visual redesign.
- No management API, provider, storage, or config changes.
- No action body moves beyond relocating the dispatch helpers.
- No TUI dependency changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Move `updateAccountKey` into `account_actions.go`.
2. Move `updateObservabilityKey` into `observability_actions.go`.
3. Keep `tea` imports available in those files.
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

1. Should tab-specific key dispatch live next to domain action bodies rather
   than in `update_keys.go`?
2. Is relocation-only scope appropriate after plan 136 introduced the helpers?
3. Does the smoke coverage remain sufficient for a move-only helper relocation?

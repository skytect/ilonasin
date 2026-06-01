# 107 TUI Account Render Split

## Context

Plan 106 moved observability and pruning rendering out of `internal/tui/tui.go`.
The main TUI file still owns account rendering together with model state,
keyboard handling, scrolling/layout, management actions, OAuth commands,
sanitizers, and logging helpers.

Account rendering is the next cohesive render cluster:

- local API token rows,
- upstream API-key credential rows,
- credential group fallback rows,
- OAuth account rows,
- provider account rows.

The architecture says the TUI is a first-class local management interface and
must talk to the daemon-owned management API for mutable operations. Splitting
render-only account code keeps that UI boundary easier to evolve without
mixing display concerns with management actions.

## Goal

Move account-related rendering out of `internal/tui/tui.go` into a dedicated
same-package file without changing behavior.

After this slice, `tui.go` still owns state, update/action logic, layout, and
shared helpers. Account rendering lives in `internal/tui/accounts.go`.

## Scope

1. Create `internal/tui/accounts.go`.
2. Move these methods intact:
   - `writeAccounts`
   - `writeOAuth`
   - `writeFallbackPolicies`
3. Keep method receivers and helper calls unchanged.
4. Keep account actions in `tui.go`, including local-token creation/disable,
   API-key input handling, OAuth login/refresh, and fallback policy
   enable/disable. Those methods perform management mutations and belong with
   update/action logic for now.
5. Do not change TUI text, key bindings, layout, scrolling, snapshot data
   handling, management clients, OAuth challenge behavior, or sanitization
   helpers.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No new TUI dependency.
- No management API, provider, storage, or config changes.
- No split of action logic in this slice.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/accounts.go` with the same `package tui`.
2. Move `writeAccounts`, `writeOAuth`, and `writeFallbackPolicies` from
   `tui.go` into the new file.
3. Add only the imports needed by the moved methods.
4. Run `gofmt`.
5. Review the diff to confirm it is a move-only render split.

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

1. Is account rendering the right next TUI cluster to split after
   observability rendering?
2. Should account mutation/action logic remain in `tui.go` for this
   behavior-preserving slice?
3. Is a move-only account render split useful before larger TUI polish work?

# 111 TUI Account Action Split

## Context

Plans 106 through 110 split render and display helper code out of
`internal/tui/tui.go`. The remaining TUI file still contains the Bubble Tea
event dispatcher, snapshot loading, OAuth helpers, logging helpers,
observability actions, and account-tab helper methods for local token
selection, API-key entry, upstream credential disablement, fallback policy
toggling, and account-row filtering.

The architecture says `ilonasin manage` is a first-class local management UI
and all mutable operations should go through daemon-owned management clients.
The current helper methods already use management clients rather than direct
SQLite. Moving them into an account-action file makes the account management
boundary easier to audit without changing the event dispatcher behavior.

## Goal

Move account-tab interaction helpers out of `internal/tui/tui.go` into a
dedicated same-package file without changing behavior.

After this slice, `tui.go` still owns model state, `Update`, `Run`, snapshot
loading/application, OAuth command helpers, pruning actions, logging helpers,
and shared lifecycle code. Account action helpers live in
`internal/tui/account_actions.go`.

## Scope

1. Create `internal/tui/account_actions.go`.
2. Move these functions and methods intact:
   - `clearReveal`
   - `selectNextLocalToken`
   - `selectPreviousLocalToken`
   - `updateAPIKeyInput`
   - `clearAPIKeyInput`
   - `disableFirstUpstreamCredential`
   - `enableFirstFallbackPolicy`
   - `disableFirstFallbackPolicy`
   - `visibleFallbackPolicies`
   - `visibleUpstreamCredentials`
   - `visibleProviderRows`
   - `fallbackPolicyEnabled`
   - `firstAPIKeyProvider`
3. Keep method receivers and helper calls unchanged.
4. Keep local-token create/disable cases in `Update` for this slice. They are
   currently inline in the key dispatcher and can be extracted in a later
   action-dispatch slice.
5. Keep OAuth login/refresh helpers in `tui.go` for this slice.
6. Do not change key bindings, TUI text, tab behavior, management client calls,
   snapshot data handling, filtering rules, or sanitization.
7. Do not add permanent tests.
8. Do not push.

## Non-Goals

- No visual redesign.
- No new TUI dependency.
- No management API, provider, storage, or config changes.
- No change to fallback policy semantics.
- No split of the `Update` dispatcher in this slice.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/account_actions.go` with the same `package tui`.
2. Move the listed helpers from `tui.go` into the new file.
3. Add only the imports needed by the moved helpers.
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

1. Are these account-tab helpers the right next boundary after render and
   display helper splits?
2. Should inline local-token create/disable cases remain in `Update` for this
   behavior-preserving slice?
3. Is a move-only account action split useful before larger `Update`
   dispatcher modularization?

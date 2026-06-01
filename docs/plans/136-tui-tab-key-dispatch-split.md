# 136 TUI Tab Key Dispatch Split

## Context

Plans 103 and 106 through 135 split TUI rendering, model state, lifecycle,
shared helpers, top-level update routing, key dispatch, and key action bodies
into focused files. `internal/tui/update_keys.go` now contains mostly
declarative dispatch, but it still repeats tab guards for every account and
observability action key.

The architecture says `ilonasin manage` is the local management control plane,
talks through daemon-owned management APIs, and must not mutate `config.toml`.
Keeping tab-scoped dispatch in small helpers makes this boundary easier to
audit without changing behavior.

## Goal

Split tab-scoped key dispatch in `internal/tui/update_keys.go` into focused
helper methods without changing behavior.

After this slice, `updateKey` handles API-key input, global/navigation keys,
and delegates tab-scoped action keys to account and observability dispatch
helpers. Action bodies remain in their existing action files.

## Scope

1. Add these same-file helper methods:
   - `updateAccountKey`
   - `updateObservabilityKey`
2. Move the account-tab guarded cases for `n`, `d`, `x`, `a`, `l`, `r`, `o`,
   `f`, and `F` into `updateAccountKey`.
3. Move the observability-tab guarded cases for `p` and `u` into
   `updateObservabilityKey`.
4. Have `updateKey` call these helpers after global/navigation cases.
5. Preserve key strings, tab guard behavior, command return values, action
   methods, error text, logging event names, reload behavior, and management
   requests unchanged.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No behavior changes.
- No visual redesign.
- No management API, provider, storage, or config changes.
- No action body moves.
- No TUI dependency changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `updateAccountKey` and `updateObservabilityKey` below `updateKey`.
2. Move the tab-scoped cases from `updateKey` into those helpers.
3. In `updateKey`, call `updateAccountKey` and return when it handles a key,
   then call `updateObservabilityKey` and return when it handles a key.
4. Use a small `handled bool` return so unknown keys still fall through to
   `return m, nil`.
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

1. Is same-file tab-scoped dispatch splitting the right next step after action
   extraction?
2. Should the account and observability dispatch helpers return
   `(tea.Model, tea.Cmd, bool)` to preserve fallthrough behavior?
3. Are there any key cases that should remain in `updateKey` rather than the
   tab dispatch helpers?
4. Is the smoke coverage sufficient for this focused dispatch split?

# 143 TUI Subscription Usage Render Split

## Context

Plans 103 and 106 through 142 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, and account-tab workflows. `internal/tui/observability.go` still
contains one large `writeObservability` method that renders recent requests,
usage totals, latency, streams, health, quota, subscription usage, keepalive
status, and fallbacks.

The architecture says `ilonasin manage` is a first-class management UI backed
by daemon-owned management APIs and metadata-only observability. Subscription
usage rendering is a distinct observability subsection because it displays
safe Codex window snapshots, pooled account-percent summaries, and keepalive
status. Keeping it separate from request/latency/health rendering makes the
privacy-sensitive subscription display boundary easier to audit.

## Goal

Move subscription usage, subscription pool, and subscription keepalive rendering
out of `writeObservability` into focused same-package helpers without changing
behavior.

After this slice:

- `observability.go` still owns the overall observability tab sequence.
- `observability_subscription.go` owns rendering subscription account rows,
  pooled subscription rows, and keepalive status.

## Scope

1. Add `internal/tui/observability_subscription.go`.
2. Move the existing subscription usage, subscription pools, and subscription
   keepalive rendering block from `writeObservability` into a new method:
   - `writeSubscriptionUsage`
3. Keep all output strings, ordering, formatting, safe display calls, reset
   labels, stale/error behavior, and schedule fallback behavior unchanged.
4. Keep `writeObservability` responsible for section ordering and call
   `m.writeSubscriptionUsage(b)` in the same location.
5. Do not change subscription usage refresh actions, management DTOs,
   provider usage parsing, storage, config, or keepalive execution guards.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No subscription usage route or DTO changes.
- No provider, storage, management API, config, or routing changes.
- No keepalive execution changes.
- No changes to token usage, cache, latency, health, quota, or fallback
  rendering.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/observability_subscription.go` with `package tui`.
2. Move the subscription usage block from `writeObservability` into
   `writeSubscriptionUsage`.
3. Replace the moved block in `writeObservability` with
   `m.writeSubscriptionUsage(b)`.
4. Keep imports minimal in both files.
5. Run `gofmt`.
6. Review the diff with moved-code highlighting or equivalent to confirm this
   is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the observability tab still
   renders `Subscription usage` and `Subscription keepalive`.

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
if [ -z "${sock:-}" ]; then
  echo "management socket not found"
  exit 1
fi
curl --silent --fail --unix-socket "$sock" \
  http://ilonasin/_ilonasin/manage/snapshot >/dev/null
curl --silent --fail --unix-socket "$sock" \
  http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null
set +e
printf '\t\tq' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  exit "$manage_status"
fi
if ! grep -q "Subscription usage" "$tmp/manage.typescript" ||
  ! grep -q "Subscription keepalive" "$tmp/manage.typescript"; then
  echo "observability subscription render smoke failed"
  cat "$tmp/manage.typescript"
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
- Direct `manage` smoke runs in a pseudo-terminal, reaches the daemon-backed
  observability tab path, renders the subscription usage and keepalive
  headings, and exits cleanly or times out with status 124. Any other status
  fails the smoke.
- `git diff --check` passes.
- Moved-code diff review confirms the subscription usage block is unchanged
  except for the new helper wrapper and imports.

## Review Questions

1. Is subscription usage rendering the right next extraction from
   `writeObservability`?
2. Should keepalive status render with subscription usage because it describes
   the same subscription account feature?
3. Is the smoke coverage sufficient for this relocation-only render split?

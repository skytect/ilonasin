# 146 TUI Health Quota Render Split

## Context

Plans 103 and 106 through 145 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, recent request rendering, usage metrics rendering, and subscription
usage rendering. `internal/tui/observability.go` still owns the health, quota,
fallback, and pruning sections directly.

The architecture treats provider health separately from request usage and
allows local quota observations from routed requests. Health and quota
rendering both display safe status rows with normalized classes, local
credential labels/IDs, timestamps, and retry/reset timing. Moving them into a
focused helper keeps that status-boundary rendering easier to audit while
leaving fallback, subscription usage, and pruning separate.

## Goal

Move health and quota rendering out of `writeObservability` into a focused
same-package helper without changing behavior.

After this slice:

- `observability.go` still owns the overall observability tab sequence.
- `observability_health.go` owns health and quota rendering.

## Scope

1. Add `internal/tui/observability_health.go`.
2. Move the existing `Health` and `Quota` blocks from `writeObservability`
   into a new method:
   - `writeHealthAndQuota`
3. Keep all output strings, ordering, formatting, safe display calls,
   retry-after/reset labels, credential display behavior, model display
   behavior, counts, and timestamps unchanged.
4. Keep `writeObservability` responsible for section ordering and call
   `m.writeHealthAndQuota(b)` between `m.writeUsageMetrics(b)` and
   `m.writeSubscriptionUsage(b)`.
5. Do not change management DTOs, snapshot loading, metadata storage, provider
   adapters, config, routing, or TUI actions.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No health or quota metadata schema changes.
- No management API, provider, storage, config, or routing changes.
- No changes to recent request, usage metrics, subscription usage, fallback,
  or pruning rendering.
- No changes to quota pooling policy.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/observability_health.go` with `package tui`.
2. Move the health and quota blocks from `writeObservability` into
   `writeHealthAndQuota`.
3. Replace the moved blocks in `writeObservability` with
   `m.writeHealthAndQuota(b)`.
4. Keep imports minimal in both files.
5. Run `gofmt`.
6. Review the diff with moved-code highlighting or equivalent to confirm this
   is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the observability tab still
   renders `Streams`, `Health`, and `Quota`.

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
if ! grep -q "Streams" "$tmp/manage.typescript" ||
  ! grep -q "Health" "$tmp/manage.typescript" ||
  ! grep -q "Quota" "$tmp/manage.typescript"; then
  echo "observability health quota render smoke failed"
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
  observability tab path, renders the stream, health, and quota headings, and
  exits cleanly or times out with status 124. Any other status fails the smoke.
- `git diff --check` passes.
- Moved-code diff review confirms the health and quota blocks are unchanged
  except for the new helper wrapper and imports.

## Review Questions

1. Are health and quota the right cohesive next extraction from
   `writeObservability`?
2. Should fallback rendering remain separate because it is a different routing
   decision history surface?
3. Is the smoke coverage sufficient for this relocation-only render split?

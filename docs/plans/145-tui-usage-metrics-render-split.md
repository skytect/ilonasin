# 145 TUI Usage Metrics Render Split

## Context

Plans 103 and 106 through 144 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, subscription usage rendering, and recent request rendering.
`internal/tui/observability.go` still owns several independent observability
sections. The usage totals, latency, and stream sections form a cohesive
metadata-only metrics cluster separate from recent per-request rows, health,
quota, subscription usage, fallbacks, and pruning.

The architecture expects the TUI to expose metadata-only usage totals,
latency/TTFT/TPS summaries, stream completion status, and retry/fallback
events without storing or rendering prompts, completions, request bodies,
response bodies, raw provider payloads, raw SSE chunks, full bearer tokens,
full provider request IDs, or full account IDs. Moving usage metrics rendering
into a focused helper keeps that allowed telemetry display boundary easier to
audit.

## Goal

Move usage totals, latency, and stream rendering out of `writeObservability`
into a focused same-package helper without changing behavior.

After this slice:

- `observability.go` still owns the overall observability tab sequence.
- `observability_metrics.go` owns usage total, latency summary, and stream
  summary rendering.

## Scope

1. Add `internal/tui/observability_metrics.go`.
2. Move the existing `Usage totals`, `Latency`, and `Streams` blocks from
   `writeObservability` into a new method:
   - `writeUsageMetrics`
3. Keep all output strings, ordering, formatting, safe display calls, token
   counts, cache counts, cost field, latency labels, TTFT/TPS labels, and
   stream counts unchanged.
4. Keep `writeObservability` responsible for section ordering and call
   `m.writeUsageMetrics(b)` between `m.writeRecentRequests(b)` and `Health`.
5. Do not change management DTOs, snapshot loading, metadata storage, provider
   adapters, config, routing, or TUI actions.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No usage, latency, or stream metadata schema changes.
- No management API, provider, storage, config, or routing changes.
- No changes to recent request, health, quota, subscription usage, fallback,
  or pruning rendering.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/observability_metrics.go` with `package tui`.
2. Move the usage totals, latency, and stream blocks from `writeObservability`
   into `writeUsageMetrics`.
3. Replace the moved blocks in `writeObservability` with
   `m.writeUsageMetrics(b)`.
4. Keep imports minimal in both files.
5. Run `gofmt`.
6. Review the diff with moved-code highlighting or equivalent to confirm this
   is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the observability tab still
   renders `Recent requests`, `Usage totals`, `Latency`, `Streams`, and
   `Health`.

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
if ! grep -q "Recent requests" "$tmp/manage.typescript" ||
  ! grep -q "Usage totals" "$tmp/manage.typescript" ||
  ! grep -q "Latency" "$tmp/manage.typescript" ||
  ! grep -q "Streams" "$tmp/manage.typescript" ||
  ! grep -q "Health" "$tmp/manage.typescript"; then
  echo "observability usage metrics render smoke failed"
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
  observability tab path, renders the recent request, usage metric, stream, and
  health headings in order, and exits cleanly or times out with status 124. Any
  other status fails the smoke.
- `git diff --check` passes.
- Moved-code diff review confirms the usage metrics blocks are unchanged
  except for the new helper wrapper and imports.

## Review Questions

1. Are usage totals, latency summaries, and stream summaries the right cohesive
   next extraction from `writeObservability`?
2. Should request-level token/cache lines stay in `writeRecentRequests` while
   aggregate token/cache lines move with usage totals?
3. Is the smoke coverage sufficient for this relocation-only render split?

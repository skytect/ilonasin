# 144 TUI Request Observability Render Split

## Context

Plans 103 and 106 through 143 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, and subscription usage rendering. `internal/tui/observability.go`
still owns several independent observability sections. The first and largest
remaining block renders recent request metadata, including route, fallback,
shape, token, cache, and latency breakdowns.

The architecture requires metadata-only observability and explicitly allows
request metadata such as provider/model, status, normalized errors, retry and
fallback counts, token counts, cache counts, latency, TTFT, and output rate.
Keeping recent request rendering in its own helper makes this privacy-sensitive
display boundary easier to audit while preserving the daemon-backed management
API and storage boundaries.

## Goal

Move recent request metadata rendering out of `writeObservability` into a
focused same-package helper without changing behavior.

After this slice:

- `observability.go` still owns the overall observability tab sequence.
- `observability_requests.go` owns the recent request section rendering.

## Scope

1. Add `internal/tui/observability_requests.go`.
2. Move the existing `Recent requests` block from `writeObservability` into a
   new method:
   - `writeRecentRequests`
3. Keep all output strings, ordering, formatting, safe display calls, token
   breakdowns, cache breakdowns, fallback reason handling, and latency labels
   unchanged.
4. Keep `writeObservability` responsible for section ordering and call
   `m.writeRecentRequests(b)` before `Usage totals`.
5. Do not change management DTOs, snapshot loading, metadata storage, provider
   adapters, config, routing, or TUI actions.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No request metadata schema changes.
- No management API, provider, storage, config, or routing changes.
- No changes to usage totals, latency summaries, streams, health, quota,
  subscription usage, fallback, or pruning rendering.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/observability_requests.go` with `package tui`.
2. Move the recent request block from `writeObservability` into
   `writeRecentRequests`.
3. Replace the moved block in `writeObservability` with
   `m.writeRecentRequests(b)`.
4. Keep imports minimal in both files.
5. Run `gofmt`.
6. Review the diff with moved-code highlighting or equivalent to confirm this
   is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the observability tab still
   renders `Recent requests`, `Usage totals`, and `Latency`.

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
  ! grep -q "Latency" "$tmp/manage.typescript"; then
  echo "observability request render smoke failed"
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
  observability tab path, renders the recent request and following
  top-of-tab observability headings, and exits cleanly or times out with status
  124. Any other status fails the smoke.
- `git diff --check` passes.
- Moved-code diff review confirms the recent request block is unchanged except
  for the new helper wrapper and imports.

## Review Questions

1. Is recent request rendering the right next extraction from
   `writeObservability`?
2. Should token/cache/latency line rendering stay in the same recent request
   helper because it describes one request row?
3. Is the smoke coverage sufficient for this relocation-only render split?

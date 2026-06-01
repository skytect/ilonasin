# 148 TUI Pruning Render Split

## Context

Plans 103 and 106 through 147 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, recent request rendering, usage metrics rendering, health/quota
rendering, subscription usage rendering, and fallback rendering.

`internal/tui/observability.go` now owns the observability tab sequence and the
telemetry pruning render helper, while `writeOverview` invokes that pruning
helper for the overview screen. The architecture treats telemetry pruning as a
management control over metadata-only observability retention. Keeping the
pruning block in its own helper makes render ownership easier to audit:
request/usage/health/subscription/fallback rendering is separate from the
manual retention control section, without changing which screen renders it.

## Goal

Move telemetry pruning rendering out of `observability.go` into a focused
same-package helper without changing behavior.

After this slice:

- `observability.go` owns only the overall observability tab sequence.
- `observability_pruning.go` owns telemetry pruning rendering.

## Scope

1. Add `internal/tui/observability_pruning.go`.
2. Move the existing `writePruning` method from `observability.go` into the
   new file unchanged.
3. Keep all output strings, ordering, formatting, nil checks, availability
   checks, prune result rendering, and timestamp formatting unchanged.
4. Keep `writeObservability` section ordering unchanged.
5. Keep the existing `writeOverview` call to `m.writePruning(b)` unchanged.
6. Do not change management DTOs, pruning actions, snapshot loading, metadata
   storage, provider adapters, config, routing, or TUI key handling.
7. Do not add permanent tests.
8. Do not push.

## Non-Goals

- No visual redesign.
- No pruning API or storage changes.
- No management route, provider, config, routing, or action changes.
- No changes to recent request, usage metrics, health/quota, subscription
  usage, or fallback rendering.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/observability_pruning.go` with `package tui`.
2. Move `writePruning` unchanged from `observability.go`.
3. Remove any now-unused imports from `observability.go`.
4. Keep imports minimal in the new file.
5. Run `gofmt`.
6. Review the diff to confirm this is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the overview still renders
   `Telemetry pruning` and the observability tab still renders `Fallbacks`.

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
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" \
  >"$tmp/serve.log" 2>&1 &
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
  cat "$tmp/serve.log"
  exit 1
fi
curl --silent --fail --unix-socket "$sock" \
  http://ilonasin/_ilonasin/manage/snapshot >/dev/null
curl --silent --fail --unix-socket "$sock" \
  http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null
set +e
printf 'q' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage-overview.typescript" >/dev/null
overview_status="$?"
set -e
if [ "$overview_status" -ne 0 ] && [ "$overview_status" -ne 124 ]; then
  cat "$tmp/manage-overview.typescript" 2>/dev/null || true
  exit "$overview_status"
fi
if ! grep -q "Providers:" "$tmp/manage-overview.typescript" ||
  ! grep -q "Telemetry pruning" "$tmp/manage-overview.typescript"; then
  echo "overview pruning render smoke failed"
  cat "$tmp/manage-overview.typescript"
  exit 1
fi
set +e
printf '\t\t\033[4~q' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage-observability.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage-observability.typescript" 2>/dev/null || true
  exit "$manage_status"
fi
if ! grep -q "Fallbacks" "$tmp/manage-observability.typescript"; then
  echo "observability fallback render smoke failed"
  cat "$tmp/manage-observability.typescript"
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
- Direct `manage` smoke runs one pseudo-terminal capture for the overview
  pruning section and a second pseudo-terminal capture for the daemon-backed
  observability tab path. Both exit cleanly or time out with status 124. Any
  other status fails the smoke.
- `git diff --check` passes.
- Moved-code diff review confirms `writePruning` is unchanged except for the
  new file location and imports.

## Review Questions

1. Is pruning rendering the right final extraction from `observability.go`?
2. Is `observability_pruning.go` the right boundary for the retention-control
   section?
3. Is the smoke coverage sufficient for this relocation-only render split?

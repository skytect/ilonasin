# 150 TUI Model Cache Summary Split

## Context

Plans 103 and 106 through 149 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, observability render sections, pruning rendering, and help
rendering.

`internal/tui/overview.go` still owns both overview rendering and the
model-cache summary aggregation helper used by that rendering. The architecture
expects the TUI to show provider instances, model cache, and metadata-only
usage while keeping management UI code modular. Moving model-cache summary
shaping into a focused file leaves the overview renderer as section
composition and keeps data shaping separate from text rendering.

## Goal

Move overview model-cache summary shaping out of `overview.go` into a focused
same-package helper file without changing behavior.

After this slice:

- `overview.go` owns overview rendering only.
- `overview_model_cache.go` owns model-cache summary shaping for the overview
  view.

## Scope

1. Add `internal/tui/overview_model_cache.go`.
2. Move these declarations from `overview.go` into the new file unchanged:
   - `modelCacheSummary`
   - `modelCacheSummaries`
3. Keep the summary sorting, UTC timestamp formatting, provider instance ID
   grouping, and count behavior unchanged.
4. Keep `writeOverview` rendering text and section ordering unchanged.
5. Do not change management DTOs, snapshot loading, storage, provider adapters,
   config, routing, key handling, or TUI actions.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No model cache API or storage changes.
- No management route, provider, config, routing, or action changes.
- No changes to help, observability, account, or layout rendering.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/overview_model_cache.go` with `package tui`.
2. Move `modelCacheSummary` and `modelCacheSummaries` unchanged from
   `overview.go`.
3. Remove any now-unused imports from `overview.go`.
4. Keep imports minimal in the new file.
5. Run `gofmt`.
6. Review the diff to confirm this is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the overview tab still renders
   `Model cache`.

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
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage-overview.typescript" 2>/dev/null || true
  exit "$manage_status"
fi
if ! grep -q "Providers:" "$tmp/manage-overview.typescript" ||
  ! grep -q "Model cache" "$tmp/manage-overview.typescript" ||
  ! grep -q "No cached models." "$tmp/manage-overview.typescript"; then
  echo "overview model-cache render smoke failed"
  cat "$tmp/manage-overview.typescript"
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
- Direct `manage` smoke runs in a pseudo-terminal, renders the overview tab
  model-cache section, and exits cleanly or times out with status 124. Any
  other status fails the smoke.
- `git diff --check` passes.
- Moved-code diff review confirms `modelCacheSummary` and
  `modelCacheSummaries` are unchanged except for the new file location and
  imports.

## Review Questions

1. Is model-cache summary shaping the right next extraction from `overview.go`?
2. Is `overview_model_cache.go` the right boundary for overview-specific model
   cache aggregation?
3. Is the overview PTY smoke sufficient for this relocation-only split?

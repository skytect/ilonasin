# 147 TUI Fallback Render Split

## Context

Plans 103 and 106 through 146 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, recent request rendering, usage metrics rendering, health/quota
rendering, and subscription usage rendering. `internal/tui/observability.go`
now owns the observability section sequence plus fallback and pruning
rendering.

The architecture expects retry/fallback events to be visible as metadata-only
observability while preserving routing and privacy boundaries. Fallback rows
render provider/model, local credential labels or IDs, and normalized fallback
reason. Moving fallback rendering into a focused helper keeps routing-decision
history separate from health/quota status, subscription usage, and pruning.

## Goal

Move fallback rendering out of `writeObservability` into a focused
same-package helper without changing behavior.

After this slice:

- `observability.go` owns the overall observability tab sequence and pruning
  rendering.
- `observability_fallbacks.go` owns fallback history rendering.

## Scope

1. Add `internal/tui/observability_fallbacks.go`.
2. Move the existing `Fallbacks` block from `writeObservability` into a new
   method:
   - `writeFallbacks`
3. Keep all output strings, ordering, formatting, safe display calls,
   credential display behavior, provider/model display behavior, fallback
   reason rendering, and timestamps unchanged.
4. Keep `writeObservability` responsible for section ordering and call
   `m.writeFallbacks(b)` after `m.writeSubscriptionUsage(b)`.
5. Keep `writePruning` in `observability.go` because it is a separate pruning
   section and can be handled in a later slice if useful.
6. Do not change management DTOs, snapshot loading, metadata storage, provider
   adapters, config, routing, fallback policies, or TUI actions.
7. Do not add permanent tests.
8. Do not push.

## Non-Goals

- No visual redesign.
- No fallback metadata schema changes.
- No management API, provider, storage, config, routing, or fallback policy
  changes.
- No changes to recent request, usage metrics, health/quota, subscription
  usage, or pruning rendering.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/observability_fallbacks.go` with `package tui`.
2. Move the fallback block from `writeObservability` into `writeFallbacks`.
3. Replace the moved block in `writeObservability` with
   `m.writeFallbacks(b)`.
4. Keep imports minimal in both files.
5. Run `gofmt`.
6. Review the diff with moved-code highlighting or equivalent to confirm this
   is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the observability tab still
   renders `Quota`, `Subscription usage`, and `Fallbacks`.

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
printf '\t\t\033[4~q' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  exit "$manage_status"
fi
if ! grep -q "Quota" "$tmp/manage.typescript" ||
  ! grep -q "Subscription usage" "$tmp/manage.typescript" ||
  ! grep -q "Fallbacks" "$tmp/manage.typescript"; then
  echo "observability fallback render smoke failed"
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
  observability tab path, scrolls near the later sections, renders quota,
  subscription usage, and fallback headings, and exits cleanly or times out
  with status 124. Any other status fails the smoke.
- `git diff --check` passes.
- Moved-code diff review confirms the fallback block is unchanged except for
  the new helper wrapper and imports.

## Review Questions

1. Is fallback history the right next extraction from `writeObservability`?
2. Should pruning remain in `observability.go` for this slice because it is a
   separate control/action result section?
3. Is the smoke coverage sufficient for this relocation-only render split?

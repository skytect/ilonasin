# 152 TUI Overview Observability Summary Split

## Context

Plans 103 and 106 through 151 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, observability render sections, pruning rendering, help rendering,
model-cache summary shaping, and provider instance rendering.

`internal/tui/overview.go` still owns overview section composition and the
compact observability summary block. The full observability tab already has
focused render helpers for recent requests, usage metrics, health/quota,
subscription usage, and fallbacks. Moving the overview summary into a focused
helper keeps the overview file as section composition while preserving the
short metadata-only summary shown on the first tab.

## Goal

Move the compact overview observability summary out of `overview.go` into a
focused same-package helper without changing behavior.

After this slice:

- `overview.go` owns overview section composition.
- `overview_observability.go` owns the overview-only observability summary.

## Scope

1. Add `internal/tui/overview_observability.go`.
2. Move the existing `Observability summary` block from `writeOverview` into
   `writeOverviewObservabilitySummary`.
3. Preserve all output strings, ordering, safe display calls, token fields,
   cache/reasoning rates, latency labels, TTFT labels, TPS-after-TTFT labels,
   and formatting.
4. Keep `writeOverview` section order unchanged.
5. Do not change the full observability tab helpers or output.
6. Do not change management DTOs, snapshot loading, storage, provider adapters,
   config, routing, key handling, or TUI actions.
7. Do not add permanent tests.
8. Do not push.

## Non-Goals

- No visual redesign.
- No usage, latency, or request metadata schema changes.
- No management route, provider, config, routing, storage, or action changes.
- No changes to provider instance, model cache, pruning, account, help, or
  layout rendering.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/overview_observability.go` with `package tui`.
2. Add `func (m Model) writeOverviewObservabilitySummary(b *strings.Builder)`
   containing the existing compact observability summary block.
3. Replace the inline block in `writeOverview` with
   `m.writeOverviewObservabilitySummary(b)`.
4. Remove any now-unused imports from `overview.go`.
5. Run `gofmt`.
6. Review the diff to confirm this is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the overview tab still renders
   `Observability summary`.

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
  ! grep -q "Observability summary" "$tmp/manage-overview.typescript" ||
  ! grep -q "recent requests" "$tmp/manage-overview.typescript"; then
  echo "overview observability summary render smoke failed"
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
  observability summary section, and exits cleanly or times out with status
  124. Any other status fails the smoke.
- `git diff --check` passes.
- Moved-code diff review confirms the compact observability summary block is
  unchanged except for the new helper call, new file location, and imports.

## Review Questions

1. Is the compact observability summary the right next extraction from
   `overview.go`?
2. Is `overview_observability.go` the right boundary for overview-specific
   observability summary rendering?
3. Is the overview PTY smoke sufficient for this relocation-only split?

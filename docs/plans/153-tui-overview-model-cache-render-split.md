# 153 TUI Overview Model Cache Render Split

## Context

Plans 103 and 106 through 152 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, observability render sections, pruning rendering, help rendering,
model-cache summary shaping, provider instance rendering, and overview
observability summary rendering.

`internal/tui/overview.go` still owns overview section composition and the
model-cache render block. `internal/tui/overview_model_cache.go` already owns
the overview-specific model-cache summary shaping. Moving the render block into
the same focused helper file keeps model-cache overview display in one
auditable place and leaves `overview.go` as pure section composition.

## Goal

Move overview model-cache rendering out of `overview.go` into the existing
model-cache overview helper file without changing behavior.

After this slice:

- `overview.go` owns overview section composition.
- `overview_model_cache.go` owns overview model-cache summary shaping and
  rendering.

## Scope

1. Add `func (m Model) writeOverviewModelCache(b *strings.Builder)` to
   `internal/tui/overview_model_cache.go`.
2. Move the existing `Model cache` render block from `writeOverview` into that
   helper.
3. Preserve all output strings, ordering, empty-state text, provider instance
   ID/count/updated-at formatting, and summary behavior.
4. Keep `writeOverview` section order unchanged.
5. Do not change provider instance rendering, observability summary rendering,
   pruning rendering, management DTOs, snapshot loading, storage, provider
   adapters, config, routing, key handling, or TUI actions.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No model-cache API or storage changes.
- No management route, provider, config, routing, storage, or action changes.
- No changes to provider instance, observability summary, pruning, account,
  help, or layout rendering.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Update `internal/tui/overview_model_cache.go` to import `fmt` and
   `strings` alongside existing summary dependencies.
2. Add `writeOverviewModelCache` containing the existing model-cache render
   block.
3. Replace the inline block in `writeOverview` with
   `m.writeOverviewModelCache(b)`.
4. Remove any now-unused imports from `overview.go`.
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
- Moved-code diff review confirms the model-cache render block is unchanged
  except for the new helper call, file location, and imports.

## Review Questions

1. Is model-cache rendering the right next extraction from `overview.go`?
2. Should model-cache rendering live with the existing overview model-cache
   summary shaping helper?
3. Is the overview PTY smoke sufficient for this relocation-only split?

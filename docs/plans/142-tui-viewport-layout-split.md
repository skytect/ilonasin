# 142 TUI Viewport Layout Split

## Context

Plans 103 and 106 through 141 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, observability
actions, and account-tab workflows. `internal/tui/layout.go` now owns both the
top-level frame rendering and the viewport/scroll mechanics.

The architecture says `ilonasin manage` is a first-class Bubble Tea/Lipgloss
management UI that talks through the daemon-owned management API. Keeping
viewport mechanics separate from frame rendering makes future TUI polish easier
to audit without touching management mutations, snapshot loading, config, or
provider behavior.

## Goal

Move viewport and scroll helpers out of `internal/tui/layout.go` into a focused
same-package file without changing behavior.

After this slice:

- `layout.go` owns top-level frame, tab, status, and footer rendering.
- `viewport.go` owns body line splitting, viewport dimensions, clipping, tab
  validation, and scroll clamping.

## Scope

1. Add `internal/tui/viewport.go`.
2. Move these functions and methods unchanged:
   - `renderViewport`
   - `splitBodyLines`
   - `viewWidth`
   - `viewHeight`
   - `viewportHeight`
   - `validActiveTab`
   - `activeScrollMax`
   - `scrollMax`
   - `scrollActive`
   - `setActiveScroll`
   - `clampScrolls`
   - `clipPlainLine`
   - `maxInt`
3. Keep `View`, `activeTabBody`, `tabBody`, `tabBar`, `statusLine`, and
   `footerLine` in `layout.go`.
4. Preserve all UI text, tab behavior, scroll behavior, viewport height
   calculation, line clipping, environment fallback behavior, and key behavior.
5. Do not change management clients, snapshot loading, storage, provider
   adapters, config, or TUI mutation boundaries.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No new TUI dependency.
- No management API, provider, storage, or config changes.
- No changes to tab key dispatch or account actions.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/viewport.go` with `package tui`.
2. Move the listed viewport and scroll helpers from `layout.go`.
3. Keep imports minimal in both files.
4. Run `gofmt`.
5. Review the diff with moved-code highlighting or equivalent to confirm this
   is relocation only plus import cleanup.

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
- Existing permanent test-file inventory is reviewed.
- Fresh binary builds.
- Direct `serve` smoke starts the daemon and exposes snapshot and subscription
  usage management routes.
- Direct `manage` smoke runs in a pseudo-terminal, reaches the daemon-backed
  TUI path, and exits cleanly or times out with status 124. Any other status
  fails the smoke.
- `git diff --check` passes.

## Review Questions

1. Is viewport/scroll mechanics the right next split from `layout.go`?
2. Should frame rendering stay in `layout.go` for this behavior-preserving
   slice?
3. Is the smoke coverage sufficient for a relocation-only viewport split?

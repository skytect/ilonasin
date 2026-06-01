# 124 TUI Snapshot Lifecycle Split

## Context

Plans 103 and 106 through 112 made the TUI tabbed and moved rendering, display
helpers, account actions, and OAuth actions out of `internal/tui/tui.go`. The
main TUI file still owns model state, `Update`, `Run`, snapshot loading and
application, observability actions, and logging helpers.

The architecture says `ilonasin manage` is a first-class local management UI
that reads and mutates through the daemon-owned management API and must not
edit `config.toml`. The current snapshot lifecycle helpers already use the
management snapshot client; moving them into a focused same-package file keeps
that read boundary easier to audit without changing behavior.

Recent management subscription usage work already exposes
`GET /_ilonasin/manage/subscription-usage` and refresh support. This TUI slice
does not change that API shape.

## Goal

Move TUI snapshot loading and application helpers out of `internal/tui/tui.go`
into a dedicated same-package file without changing behavior.

After this slice, `tui.go` still owns model state, `Update`, `Run`,
observability pruning, logging helpers, and shared lifecycle code. Snapshot
loading and snapshot-to-model application helpers live in
`internal/tui/snapshot.go`.

## Scope

1. Create `internal/tui/snapshot.go`.
2. Move these methods and helpers intact:
   - `reload`
   - `applySnapshot`
   - `applySubscriptionUsage`
   - `providersFromSnapshot`
3. Keep `Run` in `tui.go` because it wires the Bubble Tea program and model
   dependencies.
4. Keep all management client calls, snapshot fields, selection clamping,
   subscription usage application, filtering, and scroll clamping unchanged.
5. Do not change key bindings, rendering, provider rows, usage rows,
   subscription usage rows, or error text.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No management API, provider, storage, or config changes.
- No subscription usage DTO or route changes.
- No split of the `Update` dispatcher in this slice.
- No logging helper split in this slice.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/snapshot.go` with the same `package tui`.
2. Move the listed snapshot helpers from `tui.go` into the new file.
3. Add only the imports needed by the moved code.
4. Remove now-unused imports from `tui.go`.
5. Run `gofmt`.
6. Review the diff to confirm it is move-only apart from imports.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
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
- Fresh binary builds.
- Direct `serve` smoke starts the daemon and exposes snapshot and subscription
  usage management routes.
- Direct `manage` smoke runs in a pseudo-terminal, reaches the daemon-backed
  TUI path, and exits cleanly or times out with status 124. Any other status
  fails the smoke.
- `git diff --check` passes.

## Review Questions

1. Are snapshot lifecycle helpers the right next TUI boundary after render,
   display, account action, and OAuth action splits?
2. Should `Run` remain in `tui.go` for this behavior-preserving slice?
3. Is the smoke coverage sufficient for a move-only snapshot helper split?

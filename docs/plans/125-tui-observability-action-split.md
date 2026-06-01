# 125 TUI Observability Action Split

## Context

Plans 103 and 106 through 112 made the TUI tabbed and moved rendering,
display helpers, account actions, and OAuth actions out of `internal/tui/tui.go`.
Plan 124 moved snapshot loading and application into `internal/tui/snapshot.go`.
The main TUI file still owns model state, `Update`, `Run`, observability
pruning, logging helpers, and shared time/error helpers.

The architecture says telemetry pruning belongs in the management TUI and must
go through the daemon-owned management API. The current `pruneTelemetry`
method already calls the `management.TelemetryPruneClient`; moving it into a
focused observability action file makes that mutation boundary easier to audit
without changing behavior.

## Goal

Move the TUI observability pruning action out of `internal/tui/tui.go` into a
dedicated same-package file without changing behavior.

After this slice, `tui.go` still owns model state, `Update`, `Run`, logging
helpers, `nowTime`, and safe error display helpers. Observability action logic
lives in `internal/tui/observability_actions.go`.

## Scope

1. Create `internal/tui/observability_actions.go`.
2. Move `pruneTelemetry` intact.
3. Keep the `Update` key case for `p` in `tui.go` for this slice.
4. Keep `nowTime`, `logInfo`, and `logError` in `tui.go` because they are
   shared helpers used by multiple action files.
5. Do not change the pruning cutoff, management request, prune result storage,
   log event names, key bindings, rendering, snapshot reload behavior, or error
   text.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No management API, provider, storage, or config changes.
- No snapshot, subscription usage, account, or OAuth changes.
- No split of the `Update` dispatcher in this slice.
- No logging helper split in this slice.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/observability_actions.go` with the same `package tui`.
2. Move `pruneTelemetry` from `tui.go` into the new file.
3. Add only the imports needed by the moved method.
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

1. Is `pruneTelemetry` the right next TUI action boundary after the snapshot
   lifecycle split?
2. Should shared `nowTime`, `logInfo`, and `logError` remain in `tui.go` for
   this move-only slice?
3. Is the smoke coverage sufficient for a move-only observability action split?

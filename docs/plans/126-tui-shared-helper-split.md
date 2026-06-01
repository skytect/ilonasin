# 126 TUI Shared Helper Split

## Context

Plans 103 and 106 through 125 made the TUI tabbed and split rendering,
display helpers, account actions, OAuth actions, snapshot lifecycle, and
observability actions out of `internal/tui/tui.go`. The main TUI file still
owns model state, `Update`, `Run`, and several shared helper functions used by
multiple TUI files.

The architecture says `ilonasin manage` should be a first-class Bubble Tea TUI
that talks through the daemon-owned management API and does not mutate
`config.toml`. The shared helpers are not UI state or dispatch logic; moving
them out keeps `tui.go` focused on model wiring and event handling before a
later dispatcher split.

## Goal

Move shared TUI helper functions out of `internal/tui/tui.go` into a dedicated
same-package file without changing behavior.

After this slice, `tui.go` owns model state, `NewModel`, `Init`, `Update`, and
`Run`. Shared helper logic lives in `internal/tui/helpers.go`.

## Scope

1. Create `internal/tui/helpers.go`.
2. Move these helpers intact:
   - `safeErrorMessage`
   - `nowTime`
   - `logInfo`
   - `logError`
   - `tuiErrorClass`
   - `firstLogger`
3. Keep `safeErrorMessagePattern` in `display.go` because that is where the
   existing display sanitizer constant lives.
4. Do not change error sanitization, log event shape, error classification,
   time handling, key bindings, rendering, management client calls, snapshot
   reload behavior, or subscription usage behavior.
5. Do not add permanent tests.
6. Do not push.

## Non-Goals

- No visual redesign.
- No management API, provider, storage, or config changes.
- No account, OAuth, observability, or snapshot action changes.
- No split of the `Update` dispatcher in this slice.
- No new logging fields or sanitizer policy changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/helpers.go` with the same `package tui`.
2. Move the listed helpers from `tui.go` into the new file.
3. Add only the imports needed by the moved helpers.
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

1. Are these shared helpers the right next TUI boundary before splitting the
   `Update` dispatcher?
2. Should `safeErrorMessagePattern` remain in `display.go` for this move-only
   slice?
3. Is the smoke coverage sufficient for a move-only shared helper split?

# 129 TUI Key Dispatch Split

## Context

Plans 103 and 106 through 128 made the TUI tabbed and moved rendering,
display helpers, actions, snapshot lifecycle, shared helpers, model state, and
the Bubble Tea `Update` dispatcher into focused files. `internal/tui/update.go`
now owns both top-level Bubble Tea message routing and the large keybinding
switch.

The architecture says `ilonasin manage` is a first-class local control plane
that talks through the daemon-owned management API and does not mutate
`config.toml`. Separating key handling from top-level message routing keeps the
dispatcher easier to audit before deeper action extraction.

## Goal

Move key-message handling out of `internal/tui/update.go` into a focused
same-package file without changing behavior.

After this slice, `update.go` owns top-level Bubble Tea message routing.
`internal/tui/update_keys.go` owns keyboard dispatch.

## Scope

1. Create `internal/tui/update_keys.go`.
2. Move the `tea.KeyMsg` handling block from `Update` into:
   - `func (m Model) updateKey(key tea.KeyMsg) (tea.Model, tea.Cmd)`
3. Change `Update` only to call `m.updateKey(key)` where the moved block used
   to live.
4. Do not change key strings, command return values, tab guards, API key input
   handling, account actions, OAuth actions, pruning behavior, subscription
   usage refresh, snapshot reload behavior, error text, logging event names, or
   management client calls.
5. Do not add permanent tests.
6. Do not push.

## Non-Goals

- No behavior changes.
- No visual redesign.
- No management API, provider, storage, or config changes.
- No extraction of individual key cases in this slice.
- No action semantics changes.
- No TUI dependency changes.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Add `internal/tui/update_keys.go` with the same `package tui`.
2. Move the key switch from `Update` into `updateKey`.
3. Keep the moved body intact apart from the new method wrapper.
4. Add only the imports needed by the moved key handler.
5. Remove now-unused imports from `update.go`.
6. Run `gofmt`.
7. Review the diff to confirm behavior is unchanged.

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

1. Is key dispatch the right next boundary inside `update.go`?
2. Should individual key cases remain together for this behavior-preserving
   slice?
3. Is the smoke coverage sufficient for this dispatcher split?

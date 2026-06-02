# 234 TUI View Helper Boundary

## Context

`docs/ilonasin-architecture.md` treats `ilonasin manage` as a polished
Bubble Tea/Lipgloss control plane. Recent slices removed the old whole-tab
viewport renderer, moved the active UI to API/providers/usage/logs dashboard
panes, and made pane-local scrolling the only scrolling model.

One stale file boundary remains: `internal/tui/viewport.go` no longer contains
a viewport renderer. It only holds shared view helpers such as `viewWidth`,
`viewHeight`, `viewportHeight`, `validActiveTab`, `clampScrolls`,
`clipPlainLine`, `splitBodyLines`, and `maxInt`. Keeping these helpers in a
file named `viewport.go` preserves old whole-viewport vocabulary after the
dashboard migration and makes later reviews harder.

This slice is a behavior-neutral TUI cleanup. It should not change rendering,
key handling, storage, management APIs, server routes, provider behavior,
configuration, IO logging, or public API behavior.

## Goal

Move shared TUI view helper functions out of the stale `viewport.go` file into a
neutral helper boundary, leaving no file named for the removed viewport
renderer.

## Scope

1. Add `internal/tui/view_helpers.go`.
2. Move the current contents of `internal/tui/viewport.go` into
   `internal/tui/view_helpers.go`.
3. Delete `internal/tui/viewport.go`.
4. Preserve all helper names, signatures, behavior, imports, and call sites.
5. Verify no old viewport-rendering symbols or stale tab-wide scroll helpers
   remain:
   - `renderViewport`;
   - `activeScrollMax`;
   - `scrollMax`;
   - `scrollActive`;
   - `setActiveScroll`;
   - `scrollOffsets`.
6. Keep `viewportHeight` as a function name for now because it describes the
   available dashboard body height and is used by the active pane renderer.
7. Do not change TUI visible output, pane layout, keybindings, management DTOs,
   storage, provider adapters, server routes, config, IO logging policy, schema,
   or public API behavior.
8. Do not add permanent tests.
9. Do not modify or stage concurrent plan `300` work.

## Non-Goals

- No new TUI layout work.
- No helper renaming beyond the file boundary.
- No server, provider, storage, management, config, or logging changes.
- No permanent test files.

## Verification

Run:

```sh
test ! -e internal/tui/viewport.go
if rg -n 'renderViewport|activeScrollMax|scrollMax|scrollActive|setActiveScroll|scrollOffsets' internal/tui; then
  echo "stale viewport scroll code remains" >&2
  exit 1
fi
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
tmp=$(mktemp -d)
tmpbin="$tmp/bin"
mkdir -p "$tmpbin"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
port=$(python - <<'PY'
import socket
s=socket.socket()
s.bind(('127.0.0.1',0))
print(s.getsockname()[1])
s.close()
PY
)
cat >"$tmp/config.toml" <<EOF
[server]
bind = "127.0.0.1:$port"

[paths]
database = "$tmp/home/ilonasin.sqlite"
log_dir = "$tmp/home/logs"
cache_dir = "$tmp/home/cache"

[logging]
capture_io = false

[subscription_keepalive]
enabled = false

[providers.deepseek]
type = "deepseek"

[providers.codex]
type = "codex"
EOF
cleanup() {
  if [ -n "${pid:-}" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$tmp/config.toml" >"$tmp/serve.log" 2>&1 &
pid=$!
for i in $(seq 1 50); do
  if [ -d "$tmp/home/run" ] && find "$tmp/home/run" -name 'manage-*.sock' -type s | rg . >/dev/null; then
    break
  fi
  sleep 0.1
done
sock="$(find "$tmp/home/run" -name 'manage-*.sock' -type s | head -n 1)"
test -S "$sock"
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null
timeout 4s script -q -e -c "stty cols 140 rows 34; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >/dev/null || true
```

Diff review must explicitly verify:

- the move is source-equivalent;
- imports are unchanged except for file movement;
- no behavior code changed outside the file move;
- no server, provider, storage, management, config, schema, or IO logging files
  changed;
- no permanent tests were added.

## Acceptance

- `internal/tui/viewport.go` is gone.
- Shared TUI view helpers live in `internal/tui/view_helpers.go`.
- No stale whole-viewport renderer or tab-wide scroll symbols remain.
- Compile, vet, whitespace, serve smoke, manage smoke, and senior
  implementation reviews pass.

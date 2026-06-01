# 229 TUI Remove Dead Help Pane

## Context

`docs/ilonasin-architecture.md` treats `ilonasin manage` as a polished local
management UI, not a debug/help panel. Plan 228 removed the API help pane from
`apiPanes`, replacing prose-heavy help content with compact section-local key
hints. That left dead TUI helpers behind:

- `internal/tui/help.go` contains `writeHelp`, which is no longer reachable;
- `internal/tui/control_sections.go` still has `helpBody`, which only wraps
  `writeHelp` and is no longer referenced.

Keeping unreachable help prose preserves stale UI architecture and makes future
reviews chase dead code. This slice removes that stale path.

The worktree currently contains unrelated uncommitted server changes in:

- `internal/server/chat_nonstream.go`;
- `internal/server/chat_stream.go`;
- `internal/server/credentials.go`.

This slice must not modify or stage those files.

## Goal

Remove the dead TUI help-pane renderer without changing visible TUI behavior or
management APIs.

## Scope

1. Delete `internal/tui/help.go`.
2. Remove `helpBody` from `internal/tui/control_sections.go`.
3. Verify no live TUI references remain to `writeHelp`, `helpBody`,
   `Guidance`, or any removed API help pane symbol.
4. Preserve all current top-level sections, pane IDs, keybindings, dashboard
   layout, compact API/provider/usage/log content, redaction behavior, local
   time formatting, and management snapshot loading.
5. Do not change server code, provider code, management DTOs, SQLite, config,
   routing, IO logging behavior, or public APIs.
6. Do not add permanent tests.
7. Do not modify or stage unrelated dirty server files.

## Non-Goals

- No new TUI layout work.
- No new keybindings.
- No documentation rewrite beyond this plan.
- No server-side behavior change.

## Verification

Run:

```sh
if rg 'writeHelp|helpBody|Guidance|apiPaneGuidance' internal/tui -n; then
  echo "dead TUI help code remains" >&2
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
cat >"$tmp/config.toml" <<EOF2
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
EOF2
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
snapshot="$(curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot)"
printf '%s' "$snapshot" | jq -e '.providers | length >= 2' >/dev/null
for cols in 76 140; do
  timeout 4s script -q -e -c "stty cols $cols rows 32; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage-$cols.out" || true
  rg "api|providers|usage|logs|Chat Completions|Responses|Anthropic|Local API tokens" "$tmp/manage-$cols.out" >/dev/null
  if rg "Guidance|tab / shift\+tab switch sections" "$tmp/manage-$cols.out"; then
    echo "removed help pane prose remains visible" >&2
    exit 1
  fi
done
```

During diff review, explicitly verify:

- only `internal/tui/help.go`, `internal/tui/control_sections.go`, and this plan
  changed for this slice;
- the deleted helper had no live references;
- API pane IDs remain stable for summary and local tokens;
- no unrelated dirty server files are staged.

## Acceptance

- Dead help-pane code is removed.
- Current compact TUI behavior is unchanged except that unreachable code is gone.
- Compile, vet, serve smoke, manage smoke, reference search, and whitespace
  checks pass.
- Three implementation reviewers approve the focused cleanup.

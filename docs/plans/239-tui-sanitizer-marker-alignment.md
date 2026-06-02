# 239 TUI Sanitizer Marker Alignment

## Context

`docs/ilonasin-architecture.md` says the application must not render or persist
raw SSE chunks, tool arguments, or raw tool results in normal operation. The
management snapshot sanitizer already redacts markers for `sse chunk`,
`tool argument`, and `tool result`.

The TUI display sanitizer has similar privacy patterns but does not include
those three marker families. Management remains the primary snapshot sanitizer,
but the TUI should be at least as defensive for rendered dynamic labels.

## Goal

Align TUI display sanitizer marker coverage with the management snapshot
sanitizer for SSE chunk and tool argument/result markers.

## Scope

1. Update TUI `unsafeDisplayPattern` to redact:
   - `sse_chunk`;
   - `sse-chunk`;
   - `sse chunk`;
   - `ssechunk`;
   - `tool_argument`;
   - `tool-argument`;
   - `tool argument`;
   - `toolargument`;
   - `tool_result`;
   - `tool-result`;
   - `tool result`.
   - `toolresult`.
2. Update TUI account-display sanitizer with the same marker coverage.
3. Preserve existing TUI-safe labels such as `chat_completions`,
   `anthropic_messages`, `cache hit`, `reasoning`, and normal email addresses.
4. Do not change management sanitizers, DTOs, storage, server routes, provider
   behavior, config, logging, schema, or TUI layout.
5. Do not add permanent tests.
6. Do not touch unrelated concurrent work.

## Non-Goals

- No sanitizer policy redesign.
- No shared sanitizer package in this slice.
- No management snapshot sanitizer changes.
- No changes to persisted metadata.
- No changes to IO logging scrubbers.

## Verification

Run:

```sh
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
for cols in 80 120 160; do
  timeout 4s script -q -e -c "stty cols $cols rows 32; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage-$cols.out" || true
  rg "api|providers|usage|logs" "$tmp/manage-$cols.out" >/dev/null
done
```

Also run a temporary focused sanitizer smoke, then remove it before commit. It
should assert:

- `safeDisplay` redacts every marker variant listed in scope;
- `safeAccountDisplay` redacts every marker variant listed in scope;
- safe labels including `chat_completions`, `anthropic_messages`,
  `cache hit`, `reasoning`, `tools enabled`, `tool count`, and
  `user@example.com` still render.

## Acceptance

- TUI display redaction covers the same SSE/tool marker families as management
  snapshot display redaction.
- Existing normal labels remain visible.
- No server, storage, provider, management DTO, logging, config, schema, or TUI
  layout behavior changes.
- Compile, vet, serve/manage smoke, focused sanitizer smoke, whitespace checks,
  and implementation review pass.

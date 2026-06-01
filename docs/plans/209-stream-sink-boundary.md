# 209 Stream Sink Boundary

## Context

`docs/ilonasin-architecture.md` separates route handling, provider adapter
execution, response writing, and metadata-only observability. Streaming chat now
has separate execution and recording helpers, but `internal/server/chat_stream.go`
still carries the concrete SSE sink implementation used by provider adapters.

The sink is transport-level response writing: it owns SSE headers, `data:`
framing, `[DONE]`, flushing, and IO-debug logging. Keeping it in the route
execution file makes the file mix orchestration with wire transport details.

## Goal

Move the streaming chat SSE sink implementation into its own server file
without changing headers, status, event framing, `[DONE]` framing, flushing,
sink-start tracking, or IO logging.

## Scope

1. Add `internal/server/chat_stream_sink.go`.
2. Move `streamSink` and its methods from `internal/server/chat_stream.go` into
   the new file:
   - `WriteEvent`;
   - `WriteDone`;
   - `start`;
   - `logStreamEvent`.
3. Preserve exact behavior:
   - first write sets `Content-Type: text/event-stream`,
     `Cache-Control: no-cache`, and `Connection: keep-alive`;
   - first write sends HTTP `200`;
   - `WriteEvent` emits `data: <event.Data>\n\n`;
   - `WriteDone` emits `data: [DONE]\n\n`;
   - both paths log streamed output only when `capture_io` is enabled;
   - both paths flush after successful writes.
4. Keep `handleStreamingChat` constructing the same `streamSink` value and
   reading `sink.started` as before.
5. Do not touch streaming execution, recording, pre-response errors,
   non-streaming chat, Anthropic compatibility, Responses API, provider
   adapters, storage, management, TUI, config, or public route shape.

## Non-Goals

- No behavior change.
- No shared SSE abstraction in this slice.
- No permanent tests.

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
snapshot="$(curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot)"
printf '%s' "$snapshot" | jq -e '.providers | length >= 2' >/dev/null
timeout 3s script -q -e -c "stty cols 140 rows 45; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >/dev/null || true
```

Also run a temporary focused in-package smoke covering:

- `WriteEvent` sets the same headers, writes status `200`, writes
  `data: <payload>\n\n`, flushes, and marks `started`;
- a second `WriteEvent` does not rewrite the response status;
- `WriteDone` writes `data: [DONE]\n\n`, flushes, and marks `started`;
- `logStreamEvent` remains gated by `capture_io` and is not invoked when the
  server or request is nil.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- moved code is byte-for-byte behavior equivalent except for package imports;
- `chat_stream.go` still constructs `&streamSink{w: w, flusher: flusher,
  server: s, request: r}`;
- all `sink.started` checks still refer to the same field;
- no execution, recording, response error, IO logging, provider, management, or
  TUI code changed.

## Acceptance

- Streaming chat sink transport logic lives in `chat_stream_sink.go`.
- `chat_stream.go` no longer contains the concrete SSE sink implementation.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass.

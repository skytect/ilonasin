# 200 Anthropic SSE Boundary

## Context

`docs/ilonasin-architecture.md` separates local API request handling, strict
request validation, provider routing, metadata recording, and response writers.
The Anthropic Messages-compatible route currently mixes route orchestration with
Anthropic SSE event construction in `internal/server/anthropic_route.go`.

Recent slices split `/v1/models` response shaping and Responses SSE writing out
of route files. This slice applies the same boundary cleanup to the Anthropic
compatibility surface without changing behavior.

## Goal

Move Anthropic SSE event construction into a focused server helper file so the
route stays responsible for validation, routing, execution, metadata, and
pre-response errors.

## Scope

1. Add `internal/server/anthropic_sse.go`.
2. Move these helpers from `anthropic_route.go` into the new file:
   - `writeAnthropicSSE`
   - `blockStart`
   - `messageStart`
   - `initialAnthropicOutputTokens`
   - `writeAnthropicEvent`
3. Keep these in `anthropic_route.go`:
   - `handleAnthropicMessages`
   - Anthropic model fallback resolution
   - early request metadata recording
   - result extraction and pre-response error handling
   - `writeAnthropicError`
4. Preserve exact stream event order and payload shape:
   - `message_start`
   - per-block `content_block_start`, optional `content_block_delta`,
     `content_block_stop`
   - `message_delta`
   - `message_stop`
5. Preserve streaming headers, flushing behavior, JSON response behavior, and
   Anthropic-shaped error envelopes.
6. Do not change Anthropic request parsing, provider validation, credential
   pooling, chat execution, IO logging, storage, management, TUI, model
   fallback aliases, or public endpoints.

## Non-Goals

- No behavior change.
- No new Anthropic compatibility features.
- No typed replacement for all event maps in this slice.
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

Also run a temporary focused in-package smoke that writes a full Anthropic
stream with text and tool-use blocks and proves:

- streaming headers are preserved:
  - `Content-Type: text/event-stream`
  - `Cache-Control: no-cache`
  - `Connection: keep-alive`
- exact event order is preserved:
  - `message_start`
  - text `content_block_start`
  - text `content_block_delta`
  - text `content_block_stop`
  - tool `content_block_start`
  - tool `content_block_delta`
  - tool `content_block_stop`
  - `message_delta`
  - `message_stop`
- text and tool-use block-start payloads;
- text and tool-use `content_block_delta` payloads;
- `content_block_stop` payloads;
- `message_start` usage fields;
- final `message_delta` stop and usage fields;
- final `message_stop` payload;
- initial output-token behavior;
- serialized `event: ...` / `data: ...` frame shape.

Remove the temporary smoke before commit.

During diff review, explicitly verify that:

- `writeAnthropicError` remains in `anthropic_route.go`;
- non-streaming `writeJSON(w, http.StatusOK, resp)` is unchanged;
- `handleAnthropicMessages`, model fallback, metadata recording, and
  pre-response error behavior are otherwise unchanged.

## Acceptance

- `anthropic_route.go` no longer contains Anthropic SSE event construction.
- Anthropic SSE writing lives in `anthropic_sse.go`.
- Public Anthropic stream behavior is unchanged.
- Compile, vet, serve smoke, manage smoke, focused Anthropic SSE smoke, and
  whitespace checks pass.

# 206 Stream Chat Error Writer

## Context

`docs/ilonasin-architecture.md` separates route handling, provider adapter
execution, metadata recording, and response writers. OpenAI-compatible
streaming chat still contains inline pre-response upstream error writing inside
`handleStreamingChat`.

Recent slices moved equivalent response-writing decisions out of the main
handler path for Anthropic SSE, Responses SSE, and non-streaming chat. This
slice applies the same boundary cleanup to streaming chat without changing the
wire shape, retry behavior, or metadata semantics.

## Goal

Move streaming chat pre-response upstream error writing into a focused helper
without changing status codes, error envelopes, error codes, summary status
normalization, stream start behavior, recording order, or fallback/quota/health
metadata.

## Scope

1. Add a helper in `internal/server/chat_stream.go`, such as
   `writeStreamingChatPreResponseError`.
2. Move only this decision out of `handleStreamingChat`:
   - write an OpenAI-compatible error only when the final stream attempt has an
     error or status `>= 400`, and the SSE sink has not started;
   - normalize local status to `502` when the upstream status is missing,
     below `400`, or `>= 500`;
   - return the normalized summary so later metadata observes the same
     `StatusCode` mutation as today;
   - use default error code `"upstream_stream_error"`;
   - preserve the current specific error-code exceptions for
     `"upstream_auth_failed"`, `"rate_limit_exceeded"`,
     `"insufficient_quota"`, and any non-empty Codex provider error class.
3. Keep in `handleStreamingChat`:
   - flusher validation;
   - stream execution;
   - OAuth refresh retries;
   - retry/fallback planning;
   - health, quota, and fallback event construction;
   - metadata finalization and recording;
   - stream metric recording;
   - SSE sink behavior and IO logging.
4. Do not touch non-streaming chat, Anthropic compatibility, Responses API,
   provider adapters, storage, management, TUI, config, or public route shape.

## Non-Goals

- No behavior change.
- No new error response schema.
- No shared OpenAI/Anthropic error abstraction in this slice.
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

Also run a temporary focused in-package smoke proving the helper preserves:

- no write when the sink already started;
- no write when there is no attempt error and status is below `400`;
- missing status plus attempt error writes `502` with code
  `"upstream_stream_error"` and returns summary status `502`;
- status `503` writes local `502`;
- status `429` plus `"rate_limit_exceeded"` writes `429` with code
  `"rate_limit_exceeded"`;
- status `401` plus `"upstream_auth_failed"` writes `401` with code
  `"upstream_auth_failed"`;
- status `429` plus `"insufficient_quota"` writes `429` with code
  `"insufficient_quota"`;
- non-Codex status `404` plus a generic non-empty error class writes `404` with
  code `"upstream_stream_error"`;
- Codex provider plus arbitrary non-empty error class writes that class as the
  error code.

Remove the temporary smoke before commit.

During diff review, explicitly verify that:

- the helper is the only behavior moved;
- summary status normalization still feeds later request metadata;
- retry/fallback, health/quota/fallback metadata, SSE sink behavior, IO
  logging, non-streaming chat, Anthropic compatibility, Responses API, storage,
  management, and TUI code are unchanged.

## Acceptance

- Streaming chat pre-response error writing is centralized in one helper.
- OpenAI-compatible streaming chat error response behavior is unchanged.
- Compile, vet, serve smoke, manage smoke, focused helper smoke, and whitespace
  checks pass.

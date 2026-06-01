# 205 Non-Stream Chat Error Writer

## Context

`docs/ilonasin-architecture.md` separates route handling, provider adapter
execution, metadata recording, and response writers. The non-streaming chat
handler still contains inline pre-response upstream error writing for invalid
body, truncated body, retryable upstream failures, missing upstream bodies, and
non-2xx upstream statuses.

The Anthropic compatibility route already isolates equivalent pre-response
error checks in `writeAnthropicPreResponseError`. This slice applies the same
boundary cleanup to OpenAI-compatible non-streaming chat while preserving the
wire shape and recording order.

## Goal

Move non-streaming chat pre-response error writing into a focused helper without
changing status codes, error envelopes, error codes, metadata recording,
client-disconnect handling, raw success response writing, or upstream retry
behavior.

## Scope

1. Add a helper in `internal/server/chat_nonstream.go` or
   `internal/server/response.go`, such as `writeNonStreamingChatPreResponseError`.
2. Move these checks out of `handleNonStreamingChat` into the helper:
   - invalid upstream response body;
   - upstream response body too large;
   - retryable upstream request failure;
   - upstream error with no body;
   - non-2xx upstream status.
3. Preserve exact `writeError` calls:
   - invalid body: `502`, message
     `"upstream returned an invalid chat completion response"`,
     type `"api_error"`, code `"upstream_invalid_response"`;
   - body too large: `502`, message
     `"upstream response body exceeded the configured limit"`,
     type `"api_error"`, code `"upstream_body_too_large"`;
   - retryable failure: `502`, message `"upstream request failed"`,
     type `"api_error"`, code `errorClass`;
   - missing body: `502`, message `"upstream request failed"`,
     type `"api_error"`, code `errorClass`;
   - non-2xx status: `status`, message `"upstream request failed"`,
     type `"api_error"`, code `errorClass`.
4. Keep in `handleNonStreamingChat`:
   - execution;
   - status/error-class derivation;
   - Responses-output-items invalid-response adjustment;
   - metadata recording before response writing;
   - client-disconnect early return;
   - success `writeRaw`.
5. Do not change streaming chat, Anthropic compatibility, Responses API,
   provider adapters, retry/fallback logic, health/quota/fallback metadata, IO
   logging, storage, management, TUI, or public endpoints.

## Non-Goals

- No behavior change.
- No new error shape.
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

- invalid-body error status/body;
- body-too-large error status/body;
- retryable failure status/body/code;
- missing-body status/body/code;
- non-2xx status/body/code;
- no write for a successful response.

Remove the temporary smoke before commit.

During diff review, explicitly verify that:

- `recordNonStreamingChat` remains before response writing;
- client-disconnect early return remains before response writing;
- success `writeRaw` is unchanged;
- retry/fallback, health/quota/fallback metadata, stream behavior, Anthropic
  behavior, IO logging, storage, management, and TUI code are unchanged.

## Acceptance

- Non-streaming chat pre-response error writing is centralized in one helper.
- OpenAI-compatible non-streaming chat error response behavior is unchanged.
- Compile, vet, serve smoke, manage smoke, focused helper smoke, and whitespace
  checks pass.

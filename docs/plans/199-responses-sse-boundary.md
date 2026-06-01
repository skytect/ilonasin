# 199 Responses SSE Boundary

## Context

`docs/ilonasin-architecture.md` separates the local OpenAI-compatible request
surface, strict request validation, routing, provider adapters, and response
writers. `internal/server/responses_route.go` still mixes route orchestration
with Responses SSE event construction, output-item serialization, usage object
construction, and local response ID generation.

This is similar to slice 198 for `/models`: the behavior is useful, but keeping
wire-shape construction in the route handler file makes the server boundary
harder to maintain as the Responses surface grows.

## Goal

Move local Responses SSE response construction into a focused server helper file
without changing route behavior or public wire shape.

## Scope

1. Add `internal/server/responses_sse.go`.
2. Move these functions out of `responses_route.go`:
   - `writeResponsesSSE`
   - `responseFunctionCallItem`
   - `responseOutputItem`
   - `writeResponseSSEEvent`
   - `responsesUsage`
   - `localResponsesID`
3. Keep `handleResponses`, early metadata recording, result extraction, and
   pre-stream error handling in `responses_route.go`.
4. Preserve exact event order and payload shape:
   - `response.created`
   - output `response.output_item.done` events for text/function/output items;
   - `response.output_item.added` before `done` for web search calls;
   - final `response.completed` with usage details.
5. Preserve streaming headers, flushing behavior, local ID format, and existing
   unsupported output item errors.
6. Do not change OpenAI request parsing, provider validation, credential
   pooling, chat execution, Anthropic compatibility, IO logging, storage,
   management, TUI, or public endpoints.

## Non-Goals

- No behavior change.
- No new Responses API features.
- No typed replacement for all SSE maps in this slice.
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

Also run a focused no-network source or fake-adapter smoke that proves the
moved SSE helpers preserve:

- text output event shape;
- function-call item shape from chat tool calls;
- Responses output item shape for at least one existing structured item;
- usage object fields;
- response ID prefix and length.

Remove any temporary smoke files before commit.

## Acceptance

- `responses_route.go` is focused on route orchestration, validation, execution,
  and metadata.
- Responses SSE event construction lives in `responses_sse.go`.
- Public Responses SSE behavior is unchanged.
- Compile, vet, serve smoke, manage smoke, focused Responses SSE smoke, and
  whitespace checks pass.

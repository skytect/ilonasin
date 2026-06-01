# 177 Codex Responses Stream Split

## Context

`docs/ilonasin-architecture.md` keeps provider-specific behavior inside
provider adapters, but the Codex `/responses` adapter should still be modular
inside that boundary. Recent slices moved request construction into
`codex_responses_request.go` and non-streaming response parsing into
`codex_responses_parse.go`.

`internal/provider/codex_responses.go` still owns non-streaming execution,
local ID/header helpers, shared parser/stream helpers, streaming execution,
stream SSE parsing, stream event normalization, usage mapping, and stream chunk
marshaling. The streaming path is now the next coherent cluster to separate.

## Goal

Move Codex `/responses` streaming execution and stream normalization into
`internal/provider/codex_responses_stream.go` without changing behavior.

After this slice:

- `codex_responses_stream.go` owns streaming execution, stream state, stream
  SSE reading, stream event handling, stream chunk writers, and stream chunk
  marshal helpers.
- `codex_responses.go` continues to own non-streaming execution, local response
  IDs, request IDs, response headers, shared read-error classification, shared
  usage mapping, shared tool helpers, and aggregate byte defaults.
- `codex_responses_parse.go` continues to own non-streaming parser state and
  custom tool aggregation.
- `codex_responses_request.go` continues to own request/model construction.

## Scope

1. Create `internal/provider/codex_responses_stream.go`.
2. Move these declarations from `codex_responses.go`:
   - `(a HTTPChatAdapter) streamCodexChat`
   - `codexStreamState`
   - `codexStreamToolCall`
   - `includeStreamUsage`
   - `(a HTTPChatAdapter) readCodexStream`
   - `(a HTTPChatAdapter) handleCodexStreamEvent`
   - all `writeCodex*Chunk` stream writer helpers
   - `(state *codexStreamState) codexToolCall`
   - `(state *codexStreamState) aggregateBytes`
   - `codexToolCallArguments`
   - `marshalCodexStreamChunk`
   - `marshalCodexUsageChunk`
3. Leave these shared helpers in `codex_responses.go`:
   - `MaxCodexAggregateAssistantBytes`
   - `completeCodexChat`
   - `codexToolCall`
   - `classifyCodexReadError`
   - `localChatCompletionID`
   - `codexRequestIDs`
   - `newCodexRequestIDs`
   - `localCodexUUID`
   - `addCodexResponsesHeaders`
   - `codexToolCallKey`
   - `codexToolEvent`
   - `unsupportedCodexToolEvent`
   - `unsupportedCodexOutputItem`
   - `codexUsagePayload`
   - `codexUsageFromResponse`
   - `(a HTTPChatAdapter) maxCodexAggregateBytes`
4. Preserve behavior through move-only import cleanup.
5. Do not change parser validation, streaming chunks, usage fields, error
   classes, logging, storage, management routes, TUI behavior, config, or
   request construction.
6. Do not add permanent tests.

## Out of Scope

- Splitting shared Codex tool helpers into their own file.
- Moving generic stream helpers from `http_stream.go`, including
  `streamStatusForError` and `normalizedStreamErrorData`.
- Changing Codex streaming support or unsupported tool handling.
- Changing request-shape logging.
- Changing local Responses compatibility.
- Changing subscription usage, keepalive, or TUI rendering.

## Implementation Steps

1. Add `codex_responses_stream.go` with the moved streaming declarations.
2. Remove the moved declarations from `codex_responses.go`.
3. Run `gofmt`.
4. Review the diff before smoke checks and confirm this is relocation-only
   apart from import cleanup.
5. Run compile, vet, daemon, TUI, management route, and source-layout guards.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
pid=""
cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
cfg="$tmp/config.toml"
cat >"$cfg" <<'EOF'
[server]
bind = "127.0.0.1:0"
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" >"$tmp/serve.log" 2>&1 &
pid="$!"
for _ in $(seq 1 80); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null; then
    break
  fi
  sleep 0.1
done
if [ -z "${sock:-}" ]; then
  echo "management socket not found"
  cat "$tmp/serve.log"
  exit 1
fi
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null
set +e
printf '\tq' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage.typescript" 2>/dev/null || true
  exit "$manage_status"
fi
git diff --check
stream_declarations="^func \\(a HTTPChatAdapter\\) streamCodexChat|^type codexStreamState|^type codexStreamToolCall|^func includeStreamUsage|^func \\(a HTTPChatAdapter\\) readCodexStream|^func \\(a HTTPChatAdapter\\) handleCodexStreamEvent|^func writeCodexRoleChunk|^func writeCodexContentChunk|^func writeCodexToolCallChunk|^func writeCodexToolCallStartChunk|^func writeCodexToolCallArgumentsChunk|^func writeCodexToolCallArgumentsDoneChunk|^func writeCodexToolCallArgumentsByIndex|^func writeCodexToolCallDelta|^func \\(state \\*codexStreamState\\) codexToolCall|^func \\(state \\*codexStreamState\\) aggregateBytes|^func codexToolCallArguments|^func writeCodexFinishChunk|^func writeCodexUsageChunk|^func marshalCodexStreamChunk|^func marshalCodexUsageChunk"
rg -n "$stream_declarations" internal/provider/codex_responses_stream.go
if rg -n "$stream_declarations" internal/provider/codex_responses.go; then
  echo "stream declarations remain in codex_responses.go"
  exit 1
fi
shared_declarations="^const MaxCodexAggregateAssistantBytes|^func \\(a HTTPChatAdapter\\) completeCodexChat|^func codexToolCall\\(|^func classifyCodexReadError|^func localChatCompletionID|^type codexRequestIDs|^func newCodexRequestIDs|^func localCodexUUID|^func addCodexResponsesHeaders|^func codexToolCallKey|^func codexToolEvent|^func unsupportedCodexToolEvent|^func unsupportedCodexOutputItem|^type codexUsagePayload|^func codexUsageFromResponse|^func \\(a HTTPChatAdapter\\) maxCodexAggregateBytes"
rg -n "$shared_declarations" internal/provider/codex_responses.go
```

## Acceptance

- Streaming declarations compile from `codex_responses_stream.go`.
- `codex_responses.go` no longer owns stream-specific declarations.
- Shared parser/stream helpers remain available to both parser and stream code.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.
- The unrelated dirty `AGENTS.md` file is not staged or committed.

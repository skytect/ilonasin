# 179 OpenAI Chat Response Split

## Context

`docs/ilonasin-architecture.md` keeps the OpenAI-compatible API boundary
separate from provider adapters, routing, HTTP transport, management, TUI, and
SQLite storage. The `internal/openai` package is that API contract boundary,
but `internal/openai/types.go` has grown into a mixed file containing request
DTOs, request decoding, request validation, response marshaling, upstream
response extraction, stream normalization, and JSON helper functions.

Recent provider and TUI slices have improved modularity in their own packages.
The next low-risk OpenAI package slice is to move non-stream Chat Completions
response shaping and upstream response extraction into a dedicated file without
changing behavior.

## Goal

Reduce `internal/openai/types.go` responsibility by moving non-stream
Chat Completions response code into `internal/openai/chat_response.go`.

After this slice:

- `types.go` continues to own core Chat Completions request DTOs, request
  decoding, request validation, shared usage type, safe resolved model helpers,
  and shared JSON helpers.
- `chat_response.go` owns Chat Completions response DTO result types, local
  response marshaling, upstream non-stream response extraction, and usage
  response maps.
- Streaming normalization remains in `types.go` for a later stream-focused
  split, because it shares several low-level helpers with request validation.

## Scope

1. Add `internal/openai/chat_response.go`.
2. Move these declarations from `types.go`:
   - `ChatCompletionMetadata`
   - `ChatCompletionMessageResult`
   - `ResponsesOutputItem`
   - `MarshalChatCompletionResponse`
   - `MarshalChatCompletionToolCallsResponse`
   - `MarshalChatCompletionToolCallsContentResponse`
   - `usageMap`
   - `ExtractChatCompletionMetadata`
   - `ExtractChatCompletionMessageResult`
   - `normalizeChatCompletionToolCalls`
   - `extractChatCompletion`
   - `chatCompletionMessage`
   - `chatCompletionChoiceMessage`
   - `ExtractUsage`
3. Keep these declarations in `types.go`:
   - `ChatCompletionRequest`
   - `Message`
   - `ErrorEnvelope`
   - `ErrorBody`
   - `DecodeChatCompletion`
   - `(ChatCompletionRequest).HasField`
   - `(ChatCompletionRequest).Validate`
   - `Error`
   - `Usage`
   - `SafeResolvedModel`
   - `safeResolvedModelFromRaw`
   - `safeResolvedModelRune`
   - `MarshalUpstreamChatRequest`
   - streaming declarations such as `IsStreamError`,
     `NormalizedStreamChunk`, and `NormalizeStreamChunk`
   - validation and low-level JSON helper functions.
4. Keep `MessageContentString` in `content.go`, where request/content helpers
   already live.
5. Run `gofmt`.
6. Do not change public behavior, JSON shapes, errors, validation,
   provider routing, server routes, storage, management, TUI, or config.
7. Do not add permanent tests.

## Out of Scope

- Splitting request validation.
- Splitting stream normalization.
- Changing Responses API request parsing.
- Changing provider adapters or server route behavior.
- Adding new dependencies.

## Implementation Steps

1. Create `chat_response.go` with the moved declarations and minimal imports.
2. Remove the moved declarations from `types.go`.
3. Clean up imports in `types.go`.
4. Run `gofmt`.
5. Review the diff to confirm it is relocation plus import cleanup only.
6. Run compile, vet, daemon route, and manage PTY smokes.

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
smokedir="$(mktemp -d ./.tmp-openai-smoke.XXXXXX)"
cat >"$smokedir/openai_smoke.go" <<'EOF'
package main

import (
  "encoding/json"
  "fmt"
  "os"

  "ilonasin/internal/openai"
)

func main() {
  usage := openai.Usage{
    PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10,
    ReasoningTokens: 2, CachedTokens: 4, CacheWriteTokens: 1,
  }
  body, err := openai.MarshalChatCompletionResponse("chatcmpl_local", "codex/gpt-5", "hello", usage)
  if err != nil {
    panic(err)
  }
  metadata, err := openai.ExtractChatCompletionMetadata(body)
  if err != nil {
    panic(err)
  }
  if metadata.Usage.TotalTokens != 10 || metadata.ResolvedModel != "codex/gpt-5" {
    panic("metadata mismatch")
  }
  message, err := openai.ExtractChatCompletionMessageResult(body)
  if err != nil {
    panic(err)
  }
  if message.Content != "hello" || message.HasToolCalls {
    panic("message mismatch")
  }
  extractedUsage, err := openai.ExtractUsage(body)
  if err != nil || extractedUsage.CachedTokens != 4 || extractedUsage.CacheWriteTokens != 1 {
    panic("usage mismatch")
  }
  call := map[string]any{
    "id": "call_1",
    "type": "function",
    "function": map[string]any{"name": "lookup", "arguments": "{}"},
  }
  toolBody, err := openai.MarshalChatCompletionToolCallsContentResponse("chatcmpl_tool", "codex/gpt-5", "check", []map[string]any{call}, usage)
  if err != nil {
    panic(err)
  }
  toolMessage, err := openai.ExtractChatCompletionMessageResult(toolBody)
  if err != nil {
    panic(err)
  }
  if !toolMessage.HasToolCalls || len(toolMessage.ToolCalls) != 1 || toolMessage.Content != "check" {
    panic("tool call mismatch")
  }
  raw := openai.Message{Content: json.RawMessage(`"visible"`)}
  text, err := openai.MessageContentString(raw)
  if err != nil || text != "visible" {
    panic("content string mismatch")
  }
  for _, value := range []string{"bearer-token", "acct_abc", "model-with-prompt", "body"} {
    if openai.SafeResolvedModel(value) != "" {
      panic("unsafe model accepted")
    }
  }
  fmt.Fprintln(os.Stdout, "openai response smoke ok")
}
EOF
go run "$smokedir/openai_smoke.go" | grep -q "openai response smoke ok"
rm -rf "$smokedir"
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
response_declarations="^type ChatCompletionMetadata|^type ChatCompletionMessageResult|^type ResponsesOutputItem|^func MarshalChatCompletionResponse|^func MarshalChatCompletionToolCallsResponse|^func MarshalChatCompletionToolCallsContentResponse|^func usageMap|^func ExtractChatCompletionMetadata|^func ExtractChatCompletionMessageResult|^func normalizeChatCompletionToolCalls|^func extractChatCompletion|^type chatCompletionMessage|^func chatCompletionChoiceMessage|^func ExtractUsage"
rg -n "$response_declarations" internal/openai/chat_response.go
if rg -n "$response_declarations" internal/openai/types.go; then
  echo "chat response declarations remain in types.go"
  exit 1
fi
shared_declarations="^func MessageContentString|^func safeResolvedModelFromRaw|^func SafeResolvedModel|^func safeResolvedModelRune|^func IsStreamError|^type NormalizedStreamChunk|^func NormalizeStreamChunk|^func streamUsageFromMap|^func normalizeStreamChoice|^func normalizeStreamToolCalls|^func normalizeStreamLogprobs"
rg -n "$shared_declarations" internal/openai
```

## Acceptance

- Non-stream response declarations compile from `chat_response.go`.
- `types.go` no longer owns those response-specific declarations.
- Behavior is unchanged.
- Direct compile, vet, serve, management route, manage PTY, source-layout, and
  whitespace checks pass.
- Existing unrelated dirty files are not staged or committed.

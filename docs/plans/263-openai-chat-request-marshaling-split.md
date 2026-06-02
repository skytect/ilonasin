# 263 OpenAI Chat Request Marshaling Split

## Goal

Reduce `internal/openai/types.go` responsibility by moving upstream
Chat Completions request marshaling into a dedicated OpenAI request-shaping
file without changing behavior.

The `internal/openai` package is the local OpenAI-compatible contract boundary.
After earlier response and stream splits, `types.go` still owns request DTOs,
request decoding, request validation, upstream request marshaling, safe
resolved-model sanitization, and low-level JSON helpers. Upstream
Chat Completions request marshaling is a coherent boundary because it turns a
validated `ChatCompletionRequest` into the JSON sent to provider adapters.

## Scope

1. Add `internal/openai/chat_request.go`.
2. Move `MarshalUpstreamChatRequest` from `types.go` to `chat_request.go`.
3. Keep function name, arguments, exported API, JSON output shape, field
   presence behavior, and error behavior unchanged.
4. Keep `ChatCompletionRequest`, `Message`, `DecodeChatCompletion`,
   `(ChatCompletionRequest).HasField`, `(ChatCompletionRequest).Validate`,
   `Usage`, `Error`, safe resolved-model helpers, request validation helpers,
   stream normalization, and response extraction in their current files.
5. Do not change provider adapters, server routes, metadata recording, storage,
   management, TUI, config, logging policy, schema, migrations, or tests.

## Boundaries

- Behavior-preserving relocation only.
- No request field additions or removals.
- No raw prompt, completion, request body, response body, SSE chunk, tool
  argument, tool result, bearer token, OAuth token, API key, request ID, or full
  account ID storage or rendering changes.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused OpenAI request marshaling smoke, then remove it before
commit:

- build a `ChatCompletionRequest` with representative optional fields,
  including `max_tokens`, sampling controls, penalties, response format, tools,
  tool choice, parallel tool calls, prediction, user, service tier, session ID,
  metadata, logprobs, top logprobs, logit bias, stream, and stream options;
- assert `MarshalUpstreamChatRequest` emits the same field names and values;
- assert field-presence-sensitive values such as empty `tools`, `metadata`,
  `logit_bias`, `tool_choice`, and `prediction` are preserved only when
  `PresentFields` contains the field;
- assert `max_completion_tokens` remains omitted by
  `MarshalUpstreamChatRequest`, preserving the current provider-layer
  translation boundary;
- assert `stream_options` remains emitted when `stream` is true and
  `StreamOptions` is non-nil;
- assert absent optional fields are omitted;
- assert `types.go` no longer contains `MarshalUpstreamChatRequest`;
- assert provider/server files are unchanged by this slice.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- OpenAI upstream Chat Completions request marshaling lives in
  `chat_request.go`.
- `types.go` no longer owns upstream request JSON construction.
- Provider request construction continues using the same exported OpenAI API.
- JSON shape and field-presence behavior are unchanged.
- Compile, vet, focused request smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

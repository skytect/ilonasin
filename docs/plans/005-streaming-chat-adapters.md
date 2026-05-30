# Plan 005: Streaming Chat Adapters

## Goal

Add streaming `POST /v1/chat/completions` support for API-key-backed DeepSeek
and OpenRouter provider instances, preserving the strict OpenAI-compatible
surface, metadata-only observability, and provider adapter boundaries from the
previous slices.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/plans/001-initial-go-scaffold.md`
- `docs/plans/002-local-api-tokens.md`
- `docs/plans/003-upstream-api-key-credentials.md`
- `docs/plans/004-nonstreaming-chat-adapters.md`
- `AGENTS.md`

## Scope

1. Extend the provider adapter boundary with a streaming method:
   - server passes typed chat request data to the adapter,
   - adapters own upstream SSE parsing and provider-specific stream
     normalization,
   - adapters report stream summary metadata after completion or failure.
2. Support `stream: true` for DeepSeek and OpenRouter API-key provider
   instances.
3. Preserve the slice-004 non-streaming allowlist and add only:
   - `stream: true`,
   - `stream_options: {"include_usage": <bool>}`.
4. Validate `stream_options` by key presence:
   - only allowed when `stream` is true,
   - must be a non-null object,
   - must contain exactly `include_usage`,
   - `include_usage` must be boolean.
5. Forward upstream requests with:
   - resolved upstream model,
   - `stream: true`,
   - optional validated `stream_options`,
   - `Authorization: Bearer <api_key>`,
   - `Content-Type: application/json`,
   - `Accept: text/event-stream`.
6. Parse upstream data-only SSE:
   - ignore empty lines,
   - ignore comment lines beginning with `:`,
   - support one or more `data:` lines per event,
   - normalize valid normal JSON chunks into the allowed local chunk shape and
     write them as `data: ...\n\n`,
   - forward `[DONE]` exactly once and then stop reading.
7. Validate forwarded JSON chunk events as chat completion chunks before
   writing them to the local client:
   - `object` must be `chat.completion.chunk`,
   - `choices` must be present and non-null,
   - chunks with choices must have non-null non-empty choice objects,
   - final usage chunks with empty choices and complete safe `usage` are
     allowed,
   - normal chunks with non-empty choices and `usage: null` are valid,
   - `usage: null` is dropped from the normalized local chunk,
   - non-null usage objects must include integer `prompt_tokens`,
     `completion_tokens`, and `total_tokens`,
   - malformed non-error JSON is treated as an invalid upstream stream.
8. Normalize outbound chunk payloads to a strict OpenAI-compatible local shape:
   - keep top-level `id`, `object`, `created`, and `model` only when present and
     safely typed,
   - keep `choices[].index`, `choices[].delta.role`,
     `choices[].delta.content`, `choices[].delta.reasoning_content`, and
     `choices[].finish_reason` only when present and safely typed,
   - keep safe `usage` token fields only,
   - drop provider/routing/cost/debug/raw error fields and unknown extras,
   - never add, store, or log raw provider payload text.
9. Normalize OpenRouter-style top-level stream errors without forwarding raw
   provider error payloads:
   - if received before any normal data was sent locally, return a local
     OpenAI-style JSON error response,
   - if received after streaming has started, send one normalized SSE error
     event and close without `[DONE]`.
10. Use exact local error envelopes:
    - pre-stream JSON error:
      `{"error":{"message":"upstream stream failed","type":"api_error","code":"upstream_stream_error"}}`,
    - mid-stream SSE error:
      `data: {"error":{"message":"upstream stream failed","type":"api_error","code":"upstream_stream_error"}}\n\n`,
    - never include raw upstream error messages, metadata, bodies, event text,
      request IDs, generation IDs, account IDs, or provider payload fragments in
      local errors,
    - if local SSE headers were already written, the HTTP status remains `200`
      and stream metadata records the normalized failure status.
11. Bound streaming behavior:
   - upstream header/setup is bounded independently from stream lifetime,
   - no total `http.Client.Timeout` should cap a long healthy stream,
   - the streaming client must use no total `http.Client.Timeout`,
   - dial, TLS handshake, and response-header/setup phases remain bounded by
     transport/request contexts,
   - idle reads time out after 120 seconds by default and reset on upstream byte
     progress,
   - each SSE line is capped at 1 MiB by default,
   - each accumulated SSE event data payload is capped at 1 MiB by default,
   - forwarded normal data events are capped at 1,000,000 by default.
12. Make stream limits injectable:
    - production uses the defaults above,
    - `serve --check` may override idle timeout, line cap, event data cap, event
      count cap, and header/setup timeout so failure probes finish quickly.
13. Handle cancellation and local write failures explicitly:
    - client context cancellation cancels the upstream request immediately,
    - local sink/write errors stop upstream reading and close the upstream
      response body,
    - no `[DONE]` is emitted after client disconnect, cancellation, or
      mid-stream local write failure,
    - metadata is still recorded with a short detached context,
    - client disconnects record `client_disconnected`; explicit internal
      cancellations record `canceled`.
14. Record metadata-only request and stream metrics:
    - HTTP status,
    - normalized error class,
    - prompt/completion/total/reasoning token counts when safe usage appears,
    - total latency,
    - time to first output token,
    - output tokens per second,
    - stream completion status,
    - forwarded chunk count.
15. Keep Codex placeholder-only:
    - no Codex credential import,
    - no Codex/OpenAI environment auth import,
    - no keyring/file/cookie inspection,
    - no subscription, agent identity, or Codex streaming implementation.
16. Extend `serve --check` with fake TLS upstream streaming probes for both
    DeepSeek and OpenRouter instance layouts.

## Out of Scope

- Provider fallback across credentials.
- Provider model discovery.
- Tool-call streaming.
- Multimodal messages.
- OAuth credentials.
- Codex adapter implementation.
- Live provider network smoke calls.
- Permanent test files.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` is used only as a compile/package check.
- Provider adapters must not import the TUI or SQLite.
- Server must depend on narrow adapter/resolver/metadata interfaces, not storage
  concrete types.
- TUI must not be involved in request serving.
- Request/response body bytes and SSE payload bytes may exist only transiently
  for parsing, validation, upstream forwarding, response writing, and safe usage
  extraction.
- Never log or persist prompts, completions, raw bodies, raw provider payloads,
  raw stream chunks, tool arguments/results, full bearer tokens, full provider
  request IDs, full account IDs, balances, or credit totals.
- Upstream non-2xx responses before a stream starts become local coarse
  OpenAI-style errors; raw upstream error bodies are not forwarded or stored.
- Stream error metadata must be coarse and body-free. Allowed error classes
  include `credential_unavailable`, `provider_unimplemented`,
  `upstream_http_error`, `upstream_timeout`, `upstream_network_error`,
  `upstream_invalid_response`, `upstream_stream_error`,
  `upstream_stream_invalid`, `upstream_stream_too_large`, and
  `upstream_event_limit`.
- Stream completion status values are:
  - `completed`,
  - `upstream_error`,
  - `upstream_invalid`,
  - `upstream_timeout`,
  - `too_large`,
  - `event_limit`,
  - `client_disconnected`,
  - `canceled`.
- Metadata writes after a stream must use a short detached context so client
  cancellation does not suppress local metadata recording.
- The local server may set SSE headers lazily on first forwarded stream event.
  If no event has been written yet, it may still return a normal JSON error.
- Upstream read timeout implementation must not create unbounded goroutines or
  memory growth. Closing the upstream body on timeout/cancel is acceptable.
- Multi-line SSE event assembly must enforce the aggregate event data cap before
  attempting JSON validation or local writes.

## Proposed Package Changes

```text
internal/provider/
  chat.go       # add streaming interfaces and summary types
  http_chat.go  # streaming HTTP/SSE implementation
internal/openai/
  types.go      # stream_options validation and stream chunk validation
internal/server/
  server.go     # stream route handling and metadata recording
internal/metadata/
  metadata.go   # stream metric fields
internal/storage/sqlite/
  db.go         # return request metadata ID and record stream metrics
internal/app/
  app.go        # fake upstream streaming smoke checks
```

Interface shape:

```go
type ChatAdapter interface {
    CompleteChat(ctx context.Context, req ChatRequest) (ChatResult, error)
    StreamChat(ctx context.Context, req ChatRequest, sink ChatStreamSink) (ChatStreamSummary, error)
}

type ChatStreamSink interface {
    WriteEvent(ctx context.Context, event ChatStreamEvent) error
    WriteDone(ctx context.Context) error
}
```

The adapter owns upstream parsing. The server owns local response headers,
client writes, credential routing, and metadata recording.

Stream limits are configured through adapter fields or constructor options, not
package globals that smoke checks cannot override.

## Verification

Run:

```text
find . -name '*_test.go' -type f
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Smoke checks must prove:

- no permanent tests exist,
- no real provider network call happens during `serve --check`,
- the real streaming HTTP adapter is exercised against a local TLS fake
  upstream for DeepSeek and OpenRouter,
- `Accept: text/event-stream` is sent upstream,
- `stream_options.include_usage` is forwarded when requested,
- upstream comments and empty lines are ignored,
- successful streams return `text/event-stream`, flush data, forward `[DONE]`
  once, and record `completed`,
- duplicate `[DONE]` or data after `[DONE]` is not forwarded,
- pre-stream upstream non-2xx responses return local coarse JSON errors,
- pre-stream top-level provider error events return local coarse JSON errors,
- mid-stream top-level provider error events send a normalized SSE error and no
  raw provider error text,
- oversized lines, oversized aggregate events, event limits, malformed chunks,
  and idle streams record the expected completion statuses,
- client disconnect or local write failure cancels upstream work and records
  `client_disconnected` without emitting `[DONE]`,
- stream usage is recorded when a safe usage chunk appears,
- normal chunks with non-empty choices and `usage: null` are accepted, and the
  local normalized chunk omits `usage: null`,
- strict stream validation rejects `stream_options: null`, non-object
  `stream_options`, missing `include_usage`, extra keys, non-boolean
  `include_usage`, and `stream_options` when `stream` is false or omitted,
- existing unsupported fields such as `tools` and `provider_options` remain
  rejected when `stream: true`,
- selected home DB has zero check-created local/upstream credentials after
  `serve --check` and `manage --check`,
- check output does not contain fake upstream API keys, local tokens, prompts,
  completions, raw stream chunks, raw provider errors, or full response IDs.

## Review Questions

1. Is this streaming boundary narrow enough for later provider fallback and
   Codex-specific adapters?
2. Are the stream validation and error-normalization rules strict enough without
   overfitting to fake upstream behavior?
3. Are the timeout and size bounds appropriate for long healthy SSE streams?
4. Are the metadata writes sufficient while preserving the no-raw-payload
   storage boundary?

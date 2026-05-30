# Plan 004: Non-Streaming Chat Adapters

## Goal

Implement the first real provider adapter path for authenticated
`POST /v1/chat/completions` requests, covering non-streaming OpenAI-compatible
chat calls for API-key-backed DeepSeek and OpenRouter provider instances.

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
- `AGENTS.md`

## Scope

1. Add a provider adapter boundary for non-streaming chat completions:
   - server/router passes typed request data to an adapter,
   - adapters receive resolved API-key credentials only at the adapter boundary,
   - adapters return status, content type, response body, usage, latency, and
     normalized error metadata.
2. Implement API-key HTTP adapters for:
   - `deepseek`,
   - `openrouter`.
3. Keep `codex` placeholder-only:
   - no Codex credential import,
   - no Codex/OpenAI environment auth import,
   - no keyring/file/cookie inspection,
   - no subscription or agent identity handling.
4. Forward only this strict non-streaming field allowlist:
   - `model`, rewritten to the resolved upstream provider model,
   - `messages`, limited in this slice to text-content messages with roles
     `system`, `user`, or `assistant`,
   - `max_tokens`,
   - `temperature`,
   - `top_p`,
   - `stop`, limited to a string or array of strings,
   - `response_format`, limited to `{"type":"text"}` or
     `{"type":"json_object"}`.
   Unknown JSON fields remain rejected before routing. The existing loose DTO
   fields for `stream_options`, `tools`, `tool_choice`, `logprobs`,
   `top_logprobs`, message `tool_calls`, message `tool_call_id`,
   message `name`, and `provider_options` must be rejected clearly in this
   slice. Provider-specific escape hatches remain out of scope until each
   provider's typed option schema is designed.
5. Reject `stream: true` clearly in this slice. Streaming remains a later slice.
6. Normalize the model before upstream:
   - client model `<provider_instance>/<provider_model>` becomes upstream
     `provider_model`,
   - request metadata records requested and resolved provider/model.
7. Resolve one eligible upstream API-key credential for the requested provider
   instance only after the provider instance and adapter are known.
8. Send upstream requests with:
   - `Authorization: Bearer <api_key>`,
   - `Content-Type: application/json`,
   - provider base URL plus `/chat/completions`.
9. Return upstream non-streaming response bodies to the local client without
   persisting prompts, completions, request bodies, response bodies, raw provider
   payloads, bearer tokens, full provider request IDs, or full account IDs.
10. Record metadata-only request ledger fields:
    - client token ID,
    - resolved upstream credential ID,
    - requested/resolved provider instance,
    - requested/resolved model,
    - HTTP status,
    - normalized error class,
    - latency,
    - token counts when safely parsed from `usage`.
11. Extend `serve --check` with real HTTP adapter execution against an
    in-process fake upstream server:
    - no real provider network calls,
    - no real provider keys,
    - authenticates a temporary local token,
    - resolves a temporary upstream credential in an isolated DB,
    - posts a non-streaming chat request through the real server handler,
    - proves the real HTTP adapter rewrites `model`, sends bearer auth, joins
      `/chat/completions`, enforces the response body cap, and extracts safe
      usage fields,
    - verifies an OpenAI-compatible successful response,
    - verifies `stream: true` is rejected,
    - leaves the selected home DB with zero check-created local/upstream
      credentials.

## Out of Scope

- Streaming/SSE translation.
- Provider fallback across multiple credentials.
- Provider model discovery.
- Tool-call execution.
- OAuth credentials.
- Codex adapter implementation.
- Real live provider smoke calls.
- Permanent test files.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` is used only as a package compile check.
- Provider adapters must not import the TUI or SQLite.
- Server must depend on narrow adapter/resolver interfaces, not storage
  concrete types.
- TUI must not be involved in request serving.
- Request/response body bytes may exist only transiently for parsing,
  validation, upstream forwarding, response writing, and safe usage extraction.
- Never log or persist prompts, completions, raw bodies, raw provider payloads,
  raw stream chunks, tool arguments/results, full bearer tokens, full provider
  request IDs, full account IDs, balances, or credit totals.
- `ResolvedAPIKeyCredential` must stay redacted under default formatting.
- Local error responses should use OpenAI-style envelopes. Upstream responses
  should be returned as the client-facing HTTP response only and must not be
  stored in SQLite.
- Adapter error metadata must be coarse and body-free. Allowed error classes
  include `credential_unavailable`, `provider_unimplemented`,
  `upstream_http_error`, `upstream_timeout`, `upstream_network_error`,
  `upstream_invalid_response`, and `upstream_body_too_large`.
- Timeouts and body sizes should be bounded. The initial upstream response body
  cap is 16 MiB, enforced in the HTTP adapter before usage extraction or client
  response writing.
- Unsupported-field rejection must use key-presence validation, not only Go zero
  values, so explicit `null` or empty unsupported fields such as
  `provider_options: null`, `stream_options: null`, `tools: []`, or
  `messages[].name: ""` are rejected.
- A malformed 2xx upstream response is converted to a local OpenAI-style
  `upstream_invalid_response` error. Non-2xx upstream responses are returned to
  the client as upstream responses, but only coarse metadata is persisted.

## Proposed Package Changes

```text
internal/provider/
  chat.go       # adapter interfaces and common request/result types
  http_chat.go  # DeepSeek/OpenRouter HTTP adapter implementation
internal/openai/
  types.go      # outbound request shaping and safe usage extraction
internal/server/
  server.go     # route adapter invocation and metadata recording
internal/app/
  app.go        # production adapter wiring and fake upstream check wiring
internal/storage/sqlite/
  migrations.go # credential_id metadata migration if the column is absent
```

Interface shape:

```go
type ChatAdapter interface {
    CompleteChat(ctx context.Context, req ChatRequest) (ChatResult, error)
}

type ChatAdapters interface {
    ForProvider(providerType string) (ChatAdapter, bool)
}
```

`ChatRequest` contains only typed request data, provider instance metadata,
resolved model ID, and resolved API-key credential. It does not contain local
client token data.

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
```

Smoke checks must prove:

- no permanent tests exist,
- no real provider network call happens during `serve --check`,
- the real HTTP adapter is exercised against a local fake upstream,
- selected home DB has zero check-created local/upstream credentials after
  `serve --check` and `manage --check`,
- `stream: true` is rejected clearly,
- unsupported loose DTO fields such as `tools` and `provider_options` are
  rejected clearly instead of forwarded,
- fake upstream success path returns OpenAI-compatible JSON through the real
  HTTP adapter,
- malformed fake upstream 2xx JSON is converted to `upstream_invalid_response`,
- check output does not contain fake upstream API keys, local tokens, prompts,
  completions, or full response IDs.

## Review Questions

1. Is this slice the right next step after upstream API-key credential
   lifecycle?
2. Are adapter boundaries narrow enough for future streaming/fallback work?
3. Does the fake upstream check prove real HTTP adapter behavior without
   creating permanent tests or contacting real providers?
4. Are metadata-only constraints preserved?

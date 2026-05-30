# Plan 013: Codex Non-Streaming Chat

## Goal

Make `POST /v1/chat/completions` work for `codex/...` models when the client
uses non-streaming chat.

Codex upstream chat uses `POST /responses` with an SSE response, even when the
local OpenAI-compatible client asked for a non-streaming chat completion. This
slice consumes the Codex SSE stream to completion, extracts assistant text and
usage, and returns a normal OpenAI chat completion body to the local client.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- prior plans `001` through `012`
- Codex source snapshot `/tmp/codex-src-0.133.0/codex-rs`
- `AGENTS.md`

## Scope

1. Enable Codex chat as an OAuth-only capability:
   - Codex remains non-API-key and OAuth-backed,
   - DeepSeek/OpenRouter remain API-key-backed,
   - provider chat capability no longer requires `APIKey` when the provider is
     Codex,
   - Codex still does not use API-key resolver methods,
   - DeepSeek/OpenRouter still never receive OAuth credentials.
2. Keep credential boundaries explicit:
   - server resolves API-key credentials only for API-key chat providers,
   - server resolves one eligible OAuth access token for Codex chat,
   - no chat path reads an OAuth refresh token,
   - no automatic refresh or 401 recovery is added,
   - no OAuth account fallback is added.
3. Add a Codex `/responses` chat adapter path:
   - request `POST {base}/responses`,
   - send `Authorization: Bearer <oauth access token>`,
   - send `Content-Type: application/json`,
   - send `Accept: text/event-stream`,
   - send strict minimal JSON with only `model`, optional `instructions`,
     `input`, `store`, and `stream`,
   - `stream` is always true upstream because Codex `/responses` streams,
   - `store` is false,
   - `tools`, `tool_choice`, `parallel_tool_calls`, `reasoning`, `include`,
     prompt cache key, service tier, client metadata, account headers, cookies,
     installation IDs, and refresh tokens are not sent.
4. Translate the local strict OpenAI chat subset into Codex Responses input:
   - `system` messages become the Codex `instructions` string,
   - multiple system messages are joined with blank-line separators,
   - `user` messages become `ResponseItem` messages with
     `{"type":"message","role":"user","content":[{"type":"input_text","text":...}]}`,
   - `assistant` messages become `ResponseItem` messages with
     `{"type":"message","role":"assistant","content":[{"type":"output_text","text":...}]}`,
   - only the already-validated string content subset is accepted,
   - no tool calls, tool outputs, image input, raw request bodies, or provider
     options are introduced.
5. Consume Codex SSE for non-streaming compatibility:
   - parse SSE frames without storing raw chunks,
   - use the existing stream header timeout, idle timeout, max line bytes, max
     event bytes, and max events defaults from the API-key streaming adapter,
   - add a separate max aggregate assistant text bound for the synthesized
     non-streaming response,
   - honor client cancellation and return `client_disconnected` without
     writing partial text to metadata,
   - canonical text source is assistant `response.output_item.done` message
     `output_text` content in event order,
   - `response.output_text.delta` is accumulated only as a fallback when no
     completed assistant message text was received,
   - if both event families appear, completed assistant message text wins and
     deltas are discarded to avoid duplicated output,
   - stop only after `response.completed`,
   - never expose raw `response.id`; mint a local `chatcmpl_...` ID derived
     from time/randomness or a one-way digest with no raw provider ID substring,
   - extract usage from `response.completed.response.usage` into the existing
     safe metadata fields,
   - map `input_tokens_details.cached_tokens` and
     `output_tokens_details.reasoning_tokens` when present,
   - return successfully on the first `response.completed` and ignore any
     later upstream bytes by closing the response body,
   - treat `response.failed`, `response.incomplete`, malformed SSE, invalid
     completed usage, missing `response.completed`, too many events, too-large
     event, too-large aggregate output, timeout, and upstream HTTP failure as
     marker-free upstream failures,
   - ignore rate limit snapshots, reasoning encrypted content, model
     verification hints, server model headers, account IDs, provider request
     IDs, raw provider payloads, and tool events.
6. Return an OpenAI-compatible non-streaming chat completion:
   - `object: "chat.completion"`,
   - local response ID is a newly minted `chatcmpl_...` value with no raw
     provider response ID substring,
   - one assistant message with concatenated text content,
   - `finish_reason: "stop"`,
   - usage fields populated when Codex provided them, otherwise zero,
   - response body is returned to the client but never stored.
7. Keep Codex streaming explicit:
   - `stream: true` for `codex/...` returns a coarse local unimplemented error,
   - DeepSeek/OpenRouter streaming behavior remains unchanged.
8. Normalize local Codex chat failures:
   - upstream non-2xx HTTP before SSE starts returns HTTP 502 with
     `upstream_http_error`,
   - `response.failed` and `response.incomplete` return HTTP 502 with
     `upstream_response_failed`,
   - malformed SSE, malformed JSON event data, invalid completed usage, missing
     `response.completed`, too many events, too-large line, too-large event,
     and too-large aggregate output return HTTP 502 with
     `upstream_invalid_response`,
   - upstream network and idle/header timeout errors return HTTP 502 with
     `upstream_timeout` or `upstream_network_error`,
   - client cancellation records `client_disconnected` and does not attempt to
     write a partial response,
   - credential resolution failure returns HTTP 401 with `credential_unavailable`,
   - all failure envelopes use existing coarse OpenAI-compatible error JSON and
     never include raw SSE, response bodies, prompts, completions, tokens,
     account IDs, request IDs, or provider messages.
9. Extend `serve --check`:
   - seed a valid Codex OAuth access credential,
   - assert non-streaming `codex/...` chat returns HTTP 200 and a valid
     OpenAI chat completion body,
   - assert fake upstream received exact `POST /responses`,
   - assert fake upstream received OAuth access bearer auth only,
   - seed disabled, expired, and missing-access Codex OAuth credentials and
     assert they are skipped or rejected without being sent upstream,
   - assert fake upstream did not receive API-key markers, refresh-token
     markers, account IDs, cookies, prompt-cache keys, client metadata, raw
     provider payload markers, or installation IDs,
   - assert request JSON exactly matches the minimal Codex Responses shape and
     has no extra fields,
   - assert system/user/assistant messages map to `instructions` and `input`,
   - assert usage from `response.completed` is recorded in metadata,
   - assert mixed `response.output_text.delta` and `response.output_item.done`
     streams return the completed message text exactly once,
   - assert raw upstream `response.id` marker is not returned, stored, logged,
     or displayed,
   - assert raw prompts, completions, request bodies, response bodies, raw SSE
     chunks, tool arguments/results, bearer tokens, provider request IDs,
     account IDs, balances, and credits do not appear in metadata, model cache,
     CLI output, errors, or TUI output,
   - assert malformed SSE, missing completion, failed response, timeout,
     idle/hung stream without completion, too many events, too-large stream
     event, too-large aggregate output, HTTP failure, invalid completed usage,
     and client cancellation produce marker-free normalized failures,
   - assert completed-then-hung and completed-then-late-output streams return
     from the first `response.completed` without leaking later text,
   - assert Codex `stream: true` remains a coarse unimplemented error,
   - assert DeepSeek/OpenRouter chat and model discovery checks still pass.

## Out of Scope

- Codex streaming passthrough.
- Codex WebSocket `/responses`.
- Browser OAuth login.
- Device-code OAuth login.
- Automatic token refresh during chat.
- 401 recovery and retry after refresh.
- OAuth account fallback or 429 account cycling.
- Codex tool calls, local shell calls, custom tools, file input, image input,
  encrypted reasoning preservation, prompt cache key, service tier, and account
  headers.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` remains a compile/package check only.
- Do not push.
- Provider adapters do not import SQLite, TUI, or config loaders.
- Storage does not perform HTTP.
- Server receives only the read-only OAuth bearer resolver for chat; no serve
  path can read `oauth_refresh` secret material.
- Codex chat must not make Codex an API-key provider.
- Errors stay coarse and marker-free.
- Request/response bodies and raw SSE frames are parsed in memory only and are
  not stored in SQLite or metadata.

## Proposed Package Changes

```text
internal/provider/
  provider.go       # mark Codex chat-capable through OAuth
  chat.go           # add explicit bearer chat credential shape
  codex_chat.go     # Codex /responses request, SSE parser, normalization
  http_chat.go      # keep API-key chat path unchanged
internal/openai/
  types.go          # helper to build local chat.completion responses
internal/server/
  server.go         # branch API-key chat vs Codex OAuth chat
internal/app/
  app.go            # serve-check Codex non-streaming chat smoke
```

Credential shape:

```go
type ChatCredential struct {
    ID                 int64
    ProviderInstanceID string
    Kind               CredentialKind
    BearerToken        string
}
```

API-key providers fill `Kind: api_key` with the API key as bearer material.
Codex fills `Kind: oauth_access` from the OAuth bearer resolver.

## Verification

Run:

```text
find . -name '*_test.go' -type f -print
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

- no permanent test files exist,
- Codex non-streaming chat uses OAuth access bearer auth,
- Codex non-streaming chat returns an OpenAI-compatible chat completion,
- Codex streaming remains unavailable with a coarse local error,
- DeepSeek/OpenRouter API-key chat and streaming still work,
- Codex model discovery and OAuth refresh checks still work,
- no refresh token is read or sent by `serve`,
- no prompt, completion, request body, response body, raw SSE chunk, bearer
  token, account ID, provider payload, request ID, balance, credit, tool
  argument, or tool result marker leaks into persistent storage or visible
  output.

## Review Questions

1. Is consuming Codex `/responses` SSE to synthesize one non-streaming chat
   completion the right smallest useful chat slice?
2. Is it acceptable to map `system` messages into `instructions` and keep all
   other validated string messages as Responses `input`?
3. Should Codex chat use a single OAuth access credential first, leaving
   account fallback to a later 429-focused slice?

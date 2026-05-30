# Plan 016: Codex Streaming Chat

## Goal

Make `POST /v1/chat/completions` stream for `codex/...` models through the
existing Codex OAuth and `/responses` adapter path.

The architecture requires streaming chat completions as part of the local
OpenAI-compatible surface. Previous slices implemented API-key streaming,
Codex non-streaming chat by consuming `/responses` SSE, device login, and
automatic Codex OAuth refresh. This slice removes the remaining local
`codex` streaming placeholder without adding tools, WebSockets, browser login,
revocation, or account cycling.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- prior plans `001` through `015`
- Codex source snapshot `/tmp/codex-src-0.133.0/codex-rs`
- `AGENTS.md`

## Scope

1. Enable Codex streaming behind the existing adapter boundary:
   - `server` keeps routing, local auth, OAuth bearer resolution, refresh, and
     metadata ownership,
   - provider adapters still own upstream `/responses` request construction,
     SSE parsing, and provider-specific normalization,
   - storage, TUI, config loading, and credential mutation do not enter the
     provider package.
2. Reuse the existing Codex `/responses` request shape:
   - `POST {base}/responses`,
   - `Authorization: Bearer <oauth access token>`,
   - `Content-Type: application/json`,
   - `Accept: text/event-stream`,
   - strict JSON fields only `model`, optional `instructions`, `input`,
     `store`, and `stream`,
   - `store` is false,
   - `stream` is true,
   - no tools, tool choice, parallel tool calls, prompt cache key, client
     metadata, service tier, installation IDs, cookies, refresh tokens, or raw
     provider options.
3. Keep account-header behavior aligned with the current credential model:
   - this slice uses the same bearer-only Codex auth as existing Codex
     non-streaming chat,
   - the current credential model stores an account hash and safe labels, not
     the full account ID needed for `ChatGPT-Account-ID`,
   - do not add account headers, FedRAMP headers, full account ID storage, or
     account ID reconstruction in this slice,
   - assert the streaming `/responses` auth headers match existing Codex
     non-streaming auth, except for response `Accept` handling,
   - if real backend behavior later proves `ChatGPT-Account-ID` mandatory, fix
     that as a separate credential architecture change because it affects all
     Codex bearer requests, not only streaming.
4. Keep Codex OAuth credential semantics:
   - Codex streaming uses only OAuth access credentials,
   - expired credentials refresh before the stream starts through the existing
     server refresh boundary,
   - upstream 401 before local stream start refreshes the same credential ID
     and retries once,
   - no refresh or fallback after local stream bytes have been written,
   - no 429 refresh, account cycling, cross-provider fallback, or model
     switching.
5. Translate Codex `/responses` SSE to local OpenAI chat-completion SSE:
   - create one local `chatcmpl_...` stream ID, not derived from upstream
     `response.id`,
   - use the client-requested `codex/...` model string for local chunks,
   - map `response.output_text.delta` to
     `chat.completion.chunk` assistant content deltas,
   - if no deltas were seen, map `response.output_text.done` text to one
     content chunk,
   - if no delta or `response.output_text.done` text was seen, map assistant
     `response.output_item.done` `output_text` content to one content chunk,
   - if both event families appear, prefer deltas and ignore completed-message
     text to avoid duplication,
   - ignore unsupported non-content Codex events such as rate-limit snapshots,
     encrypted reasoning, model verification hints, server model headers,
     account IDs, and provider request IDs,
   - fail with a coarse marker-free `upstream_invalid_response` class on
     tool-call, tool-result, code interpreter, file-search, function call,
     local shell, or other tool events, because this slice does not request or
     support tools,
   - on `response.completed`, emit a local final chunk with
     `finish_reason: "stop"` and no usage,
   - when the client requested `stream_options: {"include_usage": true}`, emit
     a separate local usage-only chunk with `choices: []` after the finish
     chunk,
   - emit local `[DONE]` after the optional usage-only chunk,
   - record safe usage metadata even when the local final usage chunk is not
     emitted.
6. Bound and normalize Codex streaming reads:
   - use the existing stream header timeout, idle timeout, max line bytes, max
     event bytes, and max event count limits,
   - do not use an `http.Client.Timeout` that caps a healthy long stream,
   - close the upstream body on timeout, cancellation, local write error, or
     after `response.completed`,
   - never store or log raw SSE events or raw provider payloads,
   - `response.failed`, `response.incomplete`, malformed JSON event data,
     missing `response.completed`, too many events, too-large line/event,
     timeout, and upstream HTTP failure produce coarse local failures,
   - close the upstream response on the first `response.completed`, so
     duplicate completion events after completion are unreachable by design.
7. Define local streaming failure semantics:
   - credential resolution or pre-request refresh failure before stream start
     returns HTTP 401 with `credential_unavailable`,
   - upstream 401 before stream start refreshes and retries once; refresh
     failure or retry 401 returns a coarse local streaming auth failure,
   - upstream non-401 HTTP failure before stream start returns a coarse local
     stream error without raw upstream body text,
   - `response.failed` and `response.incomplete` use marker-free local
     streaming error payloads while preserving safe metadata classes such as
     `upstream_response_failed` or `upstream_response_incomplete`,
   - provider SSE failure before any local event returns JSON error
     `upstream_stream_error` or the more specific safe metadata class,
   - provider SSE failure after local events sends one normalized SSE error
     event with a coarse safe code and no `[DONE]`,
   - client cancellation records `client_disconnected` and sends no synthetic
     content or `[DONE]`,
   - all metadata classes and stream completion statuses remain marker-free.
8. Preserve existing API-key streaming:
   - DeepSeek and OpenRouter streaming behavior remains unchanged,
   - existing fallback-before-stream-start behavior remains unchanged for
     API-key providers,
   - Codex does not join API-key fallback groups.
9. Use a narrow server path for Codex streaming:
   - remove the current Codex stream placeholder,
   - resolve one OAuth bearer credential with the same model credential
     resolver used by Codex non-streaming chat,
   - convert that credential to `provider.ChatCredential` and call
     `ChatAdapter.StreamChat`,
   - do not pass Codex through the API-key streaming context or API-key
     fallback groups,
   - if the adapter returns a pre-stream 401 summary, call
     `refreshOAuthCredentialForRetryIfBearer` and retry the same credential ID
     once before any local stream bytes are written,
   - never refresh, switch credentials, or retry after local stream start.
10. Extend `serve --check`:
   - seed a valid Codex OAuth access credential and assert `stream: true`
     returns local SSE chunks plus `[DONE]`,
   - assert fake upstream receives exact Codex `/responses` request shape and
     OAuth access bearer only,
   - assert streaming auth headers match existing Codex non-streaming auth,
     except for response `Accept` handling,
   - assert streamed local chunks use `chat.completion.chunk`, a local
     `chatcmpl_...` ID, the local `codex/...` model string, an initial
     assistant role chunk, assistant deltas, a final `finish_reason: "stop"`
     chunk without usage, an optional separate usage-only chunk when requested,
     and `[DONE]`,
   - assert upstream `response.id`, raw provider payload markers, prompts,
     bearer tokens, refresh tokens, account IDs, request IDs, rate-limit
     payloads, tool markers, and cookies are not stored or returned,
   - assert returned assistant content is visible only in the client stream, and
     is not stored in SQLite metadata, model cache, health events, fallback
     events, TUI output, CLI output, or local error envelopes,
   - assert delta-only, output-text-done-only, output-item-only, mixed
     delta/output-text-done/output-item, usage, no-usage,
     completed-then-hung, and late-output-after-completed cases,
   - assert malformed SSE, missing completion, tool events, `response.failed`,
     `response.incomplete`, too-large line/event, too-many-events, idle
     timeout, upstream HTTP error, upstream 401 refresh, refresh failure, retry
     401, 429 no-refresh, 5xx no-refresh, and client cancellation are
     normalized and marker-free,
   - assert stream metadata records safe status, error class, retry count,
     usage, time-to-first-token, output rate, chunk count, and completion
     status,
   - assert existing DeepSeek/OpenRouter chat, streaming, fallback, model
     discovery, Codex non-streaming chat, Codex OAuth refresh, device login,
     and TUI checks still pass.

## Out of Scope

- Codex WebSocket `/responses`.
- Codex tools, local shell tools, tool-call streaming, custom tools, file input,
  image input, encrypted reasoning preservation, prompt cache key, service
  tier, account headers, FedRAMP headers, cookies, and installation IDs.
- Browser authorization-code callback login.
- Revocation/logout.
- OAuth account fallback or 429 account cycling.
- Importing Codex `auth.json`, keyring, cookies, or environment auth.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` remains a compile/package check only.
- Do not push.
- Provider adapters do not import SQLite, TUI, config loaders, or credential
  storage.
- Storage performs no HTTP.
- Server may trigger OAuth refresh through the credential service before local
  stream start, but it never receives refresh-token material.
- Provider adapters surface normalized stream summaries only; they do not
  perform credential refresh.
- Codex streaming must not make Codex an API-key provider.
- Local streaming chunks must be generated from safe typed fields, not by
  forwarding raw Codex event JSON.
- Request bodies, response bodies, raw SSE chunks, prompts, completions, bearer
  tokens, provider IDs, request IDs, account IDs, balances, credits, tool
  arguments, and tool results must not be stored or displayed.

## Proposed Package Changes

```text
internal/provider/
  chat.go       # no broad interface change expected; reuse stream summary
  http_chat.go  # Codex /responses streaming normalization
internal/server/
  server.go     # narrow Codex OAuth stream handler and pre-stream retry
internal/openai/
  types.go      # helper for local chat.completion.chunk construction if useful
internal/app/
  app.go        # serve-check Codex streaming smoke cases
```

Expected local chunk shape:

```json
{
  "id": "chatcmpl_<local>",
  "object": "chat.completion.chunk",
  "created": 1780000000,
  "model": "codex/gpt-5.5-codex",
  "choices": [
    {"index": 0, "delta": {"content": "text"}, "finish_reason": null}
  ]
}
```

Initial role chunk shape:

```json
{
  "id": "chatcmpl_<local>",
  "object": "chat.completion.chunk",
  "created": 1780000000,
  "model": "codex/gpt-5.5-codex",
  "choices": [
    {"index": 0, "delta": {"role": "assistant"}, "finish_reason": null}
  ]
}
```

Finish chunk shape:

```json
{
  "id": "chatcmpl_<local>",
  "object": "chat.completion.chunk",
  "created": 1780000000,
  "model": "codex/gpt-5.5-codex",
  "choices": [
    {"index": 0, "delta": {}, "finish_reason": "stop"}
  ]
}
```

Optional usage-only chunk shape when `stream_options.include_usage` is true:

```json
{
  "id": "chatcmpl_<local>",
  "object": "chat.completion.chunk",
  "created": 1780000000,
  "model": "codex/gpt-5.5-codex",
  "choices": [],
  "usage": {
    "prompt_tokens": 3,
    "completion_tokens": 4,
    "total_tokens": 7
  }
}
```

The usage-only chunk is omitted unless the client requested
`stream_options.include_usage: true`, but safe usage is still recorded in
metadata when Codex sends it. This matches the OpenAI chat streaming shape where
the usage chunk has empty `choices`.

## Verification

Run:

```text
tmpbin=""
tmp=""
cleanup() {
  [ -n "$tmpbin" ] && rm -rf "$tmpbin"
  [ -n "$tmp" ] && rm -rf "$tmp"
}
trap cleanup EXIT
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
- Codex `stream: true` returns OpenAI-compatible local SSE,
- Codex streaming uses OAuth access bearer auth and never API keys,
- expired Codex OAuth credentials refresh before streaming,
- upstream 401 before local streaming retries once with the same credential ID,
- no refresh occurs after local stream start or for 429/5xx,
- stream errors and metadata are marker-free,
- raw Codex event JSON, response IDs, prompts, completions, tokens, account
  IDs, request IDs, tool arguments/results, and provider payloads are not
  stored or displayed,
- DeepSeek/OpenRouter API-key streaming and existing Codex non-streaming and
  refresh behavior still pass.

## Review Questions

1. Is mapping Codex `response.output_text.delta` into local chat completion
   delta chunks the right smallest streaming compatibility layer?
2. Should `response.output_item.done` be used only as a fallback when no deltas
   were seen, to avoid duplicated text?
3. Is it acceptable to keep Codex stream refresh/retry strictly pre-stream-start
   and exact credential ID only?

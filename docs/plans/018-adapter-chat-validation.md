# Plan 018: Adapter Chat Validation

## Goal

Move chat feature validation to the provider adapter boundary and make model
capability metadata match the request behavior that is actually implemented.

The architecture requires provider adapters to own provider-specific request
translation and unsupported-field rejection. Current validation rejects some
known OpenAI fields before routing, while `/v1/models` can advertise tools or
logprobs that `/v1/chat/completions` then refuses globally. This slice makes
the local API stricter and more truthful without adding tool execution,
logprobs, JSON Schema, or provider escape hatches.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `017`

## Scope

1. Split request validation into two layers:
   - OpenAI request decoding and shape validation remains in `internal/openai`,
   - provider feature validation happens only after model routing selects the
     provider instance and chat adapter,
   - ordering is route model, select adapter, validate features, then resolve
     credentials, refresh OAuth, apply fallback, or construct an upstream
     request,
   - server stays responsible for auth, routing, credential resolution,
     metadata, and HTTP response writing.
2. Keep strict request decoding:
   - unknown top-level fields still return a clear local error,
   - known but currently unsupported fields such as `tools`, `tool_choice`,
     `logprobs`, `top_logprobs`, and `provider_options` can decode so the
     selected adapter can reject them,
   - decoded requests retain raw top-level key presence so adapters can reject
     `tools: []`, `tool_choice: null`, `logprobs: null`,
     `top_logprobs: null`, `provider_options: null`, and
     `response_format: null` deterministically,
   - unsupported message fields such as `name`, `tool_call_id`, and
     `tool_calls` remain rejected by shape validation because no current
     adapter can safely forward or normalize tool messages,
   - `role: "tool"` remains rejected by shape validation.
3. Add adapter-owned feature validation:
   - make feature validation a required provider chat adapter method,
   - adapters that do not validate fail closed at compile time,
   - DeepSeek and OpenRouter allow the currently forwarded common fields:
     `model`, `messages`, `stream`, `stream_options`, `max_tokens`,
     `temperature`, `top_p`, `stop`, and `response_format` with `type` `text`
     or `json_object`,
   - DeepSeek and OpenRouter reject tools, tool choice, logprobs, top logprobs,
     and provider options because they are not implemented locally yet,
   - Codex allows only fields the `/responses` adapter actually translates or
     local streaming needs: `model`, `messages`, `stream`, and
     `stream_options`,
   - Codex rejects `max_tokens`, `temperature`, `top_p`, and `stop` because the
     current `/responses` request builder does not translate them,
   - Codex rejects `response_format` instead of silently ignoring it,
   - all rejection messages are stable product wording and do not mention
     slices.
4. Make model capabilities truthful:
   - remove `tools` and `logprobs` from discovered capability flags until chat
     actually supports them,
   - keep `json_object` only where the current chat adapter forwards
     `response_format`,
   - keep `stream`, `chat`, and `reasoning` where existing behavior and safe
     metadata support them,
   - expected current capability strings are:
     DeepSeek `chat,json_object,reasoning,stream`,
     Codex `chat,reasoning,stream`, and OpenRouter flags derived from safe
     supported parameters excluding `tools` and `logprobs`,
   - do not advertise capabilities that require provider escape hatches or tool
     execution.
5. Preserve existing serving behavior:
   - DeepSeek and OpenRouter non-streaming and streaming chat continue to work,
   - Codex non-streaming and streaming chat continue to work for supported
     fields,
   - credential fallback, OAuth refresh, metadata recording, model cache, TUI,
     storage, and config behavior do not change.
6. Extend smoke coverage without permanent tests:
   - `serve --check` asserts unsupported known fields are rejected after routing
     by the selected adapter,
   - Codex `response_format` is rejected and is not silently dropped,
   - Codex `max_tokens`, `temperature`, `top_p`, and `stop` are rejected and
     are not silently dropped,
   - DeepSeek/OpenRouter `response_format: {"type":"json_object"}` still
     reaches the fake upstream,
   - model discovery no longer caches `tools` or `logprobs` capability flags
     until implemented,
   - validation failures for `tools`, `tool_choice`, `logprobs`,
     `top_logprobs`, `provider_options`, Codex `response_format`, and Codex
     sampling fields do not hit fake upstreams, trigger OAuth refresh, trigger
     credential fallback, or store unsupported request details,
   - validation happens before credential resolution, including a configured
     provider with no eligible upstream credential,
   - stale "in this slice" wording no longer appears in local chat validation
     errors.

## Out of Scope

- Implementing tools, tool calls, tool result messages, JSON Schema,
  provider-specific grammars, logprobs, top logprobs, or provider options.
- Adding DeepSeek `thinking`, DeepSeek `reasoning_effort`, OpenRouter
  `reasoning`, routing preferences, plugins, BYOK, guardrails, or app
  attribution headers.
- Changing upstream credential selection, fallback policy, OAuth refresh,
  streaming normalization, or metadata schema.
- Replacing provider adapters or splitting `HTTPChatAdapter` by provider.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- Do not push.
- Provider adapters must not import SQLite, TUI, config loaders, or credential
  storage.
- OpenAI decode must not forward unknown fields.
- Provider adapters must reject unsupported behavior before constructing an
  upstream request.
- Provider adapters must reject unsupported behavior before credential lookup,
  OAuth refresh, fallback, or upstream I/O.
- No request bodies, response bodies, prompts, completions, tool arguments,
  tool results, raw provider payloads, raw SSE chunks, bearer tokens, provider
  request IDs, or account IDs are stored or displayed.

## Proposed Package Changes

```text
internal/openai/
  types.go       # shape validation and stable unsupported wording
internal/provider/
  chat.go        # adapter validation boundary
  http_chat.go   # provider-specific supported-field checks and capabilities
internal/server/
  server.go      # call adapter validation after routing
internal/app/
  app.go         # serve-check validation and capability smoke cases
```

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Acceptance:

- no permanent tests exist,
- compile/package, vet, build, `serve --check`, `manage --check`, and diff
  whitespace checks pass,
- unsupported known fields are rejected by adapter-owned validation,
- Codex no longer silently accepts `response_format`,
- DeepSeek/OpenRouter supported fields still reach upstream unchanged,
- `/v1/models` and cached model metadata no longer advertise unimplemented
  tools or logprobs support,
- local validation messages contain no stale slice-era wording.

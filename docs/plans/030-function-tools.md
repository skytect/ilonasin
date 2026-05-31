# Plan 030: Function Tools

## Goal

Accept OpenAI-style function tools for DeepSeek and OpenRouter chat
completions, while continuing to reject tools for Codex.

DeepSeek and OpenRouter both document `tools` and `tool_choice` on chat
completions. Current local validation decodes these fields but rejects them at
the adapter boundary. This slice adds a strict function-tool subset without
adding local tool execution, OpenRouter server tools, provider routing controls,
or Codex tool translation.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `029`
- official OpenRouter docs checked through Context7 on 2026-05-31:
  `https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request`
  and `https://openrouter.ai/docs/guides/features/tool-calling`
- official DeepSeek docs checked through Context7 on 2026-05-31:
  `https://api-docs.deepseek.com/api/create-chat-completion` and DeepSeek tool
  calling guide material

## Scope

1. Accept a strict function-tool request subset:
   - `tools`, when present, must be an array with at most `128` entries,
   - each tool must be an object with only `type` and `function`,
   - `type` must be `"function"`,
   - OpenRouter server-tool types such as `openrouter:web_search` remain
     unsupported,
   - `function` must be an object,
   - `function` may contain only `name`, `description`, `parameters`, and
     `strict`,
   - `function.name` is required, must be 1 through 64 characters, and may
     contain only ASCII letters, digits, `_`, and `-`,
   - duplicate function names are rejected,
   - `function.description`, when present, must be a string,
   - `function.parameters`, when present, must be a JSON object,
   - `function.parameters` must be preserved with `json.RawMessage` or
     `UseNumber` so nested numeric schema values are not rounded or reformatted
     through `float64`,
   - `function.strict`, when present, must be a boolean, but `true` remains
     unsupported because DeepSeek strict mode requires the beta base URL and
     OpenRouter strict semantics are not part of this slice,
   - tool schemas are forwarded as structured JSON but never persisted.
2. Accept strict `tool_choice`:
   - strings `none`, `auto`, and `required` are accepted,
   - named function choice
     `{"type":"function","function":{"name":"..."}}` is accepted,
   - named choice must reference a function supplied in `tools`,
   - `tool_choice` with no tools is accepted only when it is `"none"`,
   - `null`, booleans, arrays, unknown strings, unknown object fields, missing
     names, malformed names, and unknown function names fail before credential
     resolution and upstream HTTP.
3. Support tool conversation messages:
   - assistant messages may include `tool_calls`,
   - assistant message `content` may be a JSON string or `null` when
     `tool_calls` is present,
   - each assistant tool call must be an object with `id`, `type`, and
     `function`,
   - assistant tool-call objects may contain only `id`, `type`, and
     `function`,
   - tool call `id` must be a non-empty string,
   - tool call `type` must be `"function"`,
   - tool call `function` may contain only `name` and `arguments`,
   - tool call `function.name` must use the same function-name validator,
   - tool call `function.arguments` must be a string,
   - tool role messages are accepted with non-empty `tool_call_id` and string
     `content`,
   - `tool_call_id`, `tool_calls`, and `name` remain rejected on roles where
     they are not supported.
4. Forward supported fields:
   - DeepSeek accepts and forwards `tools`, `tool_choice`, assistant
     `tool_calls`, and tool role messages,
   - OpenRouter accepts and forwards the same strict subset,
   - Codex rejects `tools`, `tool_choice`, assistant `tool_calls`, and tool
     role messages before credential resolution and upstream HTTP.
5. Preserve returned tool-call data:
   - non-streaming upstream chat responses can return `message.tool_calls` and
     `finish_reason:"tool_calls"` to the requesting client,
   - streaming upstream chunks can return `delta.tool_calls`, including
     incremental `function.arguments` strings,
   - partial streamed tool-call deltas are supported across multiple chunks,
     including later chunks that carry only `index` plus argument fragments,
   - streamed `tool_calls` are shape-validated and normalized like other SSE
     choice fields,
   - each streamed tool-call delta must be an object with only `index`, `id`,
     `type`, and `function`,
   - streamed `index` is required and must be a non-negative JSON integer,
   - streamed `id`, when present, must be a non-empty string,
   - streamed `type`, when present, must be `"function"`,
   - streamed `function`, when present, must be an object with only `name` and
     `arguments`,
   - streamed `function.name`, when present, must use the same function-name
     validator,
   - streamed `function.arguments`, when present, must be a string,
   - `delta.tool_calls: null`, malformed arrays, unknown keys, bad indexes,
     bad names, bad IDs, and non-string argument chunks fail as invalid
     upstream responses,
   - tool-call-only chunks count as stream-started chunks for retry and
     completion-status purposes, but do not set `time_to_first_token_ms` and do
     not count as output text tokens for tokens-per-second metadata,
   - invalid upstream streaming tool-call shapes fail as invalid upstream
     responses without leaking raw payloads.
6. Update capability metadata:
   - DeepSeek static model capabilities include `tools`,
   - OpenRouter model discovery maps `tools` and `tool_choice` supported
     parameters to the `tools` capability flag,
   - Codex capabilities remain unchanged.
7. Preserve privacy and metadata boundaries:
   - do not persist tool definitions, tool descriptions, JSON schemas,
     tool-call arguments, tool-call IDs, tool role content, request bodies,
     response bodies, raw provider payloads, or raw SSE chunks,
   - do not include those values in local errors, TUI output, CLI output, or
     SQLite metadata,
   - provider adapters may hold tool data only long enough to validate, forward,
     and return it to the requesting client.
8. Extend smoke checks without permanent tests:
   - DeepSeek and OpenRouter non-streaming tool requests reach fake upstream
     with exact `tools` and `tool_choice`,
   - nested `function.parameters` numeric schema values are forwarded exactly,
     including when the request is later re-marshaled for `provider_options` or
     `max_completion_tokens`,
   - DeepSeek and OpenRouter tool-result follow-up messages reach fake upstream
     with exact assistant `tool_calls` and tool role messages,
   - DeepSeek and OpenRouter non-streaming responses preserve
     `message.tool_calls` with `finish_reason:"tool_calls"` for the requesting
     client,
   - DeepSeek and OpenRouter streaming tool requests preserve valid
     `delta.tool_calls`,
   - streaming smoke includes a multi-chunk tool call where the first delta has
     `id`, `type`, `function.name`, and the first arguments fragment, and later
     deltas carry only `index` plus additional `function.arguments` fragments,
   - Codex valid tool requests and tool conversation messages fail before
     upstream HTTP,
   - invalid tool, `tool_choice`, assistant tool-call, and tool-message shapes
     fail before upstream HTTP and before credential resolution,
   - model cache capabilities advertise `tools` only where supported,
   - privacy scans prove tool names, tool-call IDs, arguments, schemas, and
     tool result markers do not appear in SQLite metadata, TUI output, CLI
     output, or local errors.

## Out of Scope

- Local tool execution.
- Tool argument JSON parsing or invocation.
- OpenRouter server tools such as `openrouter:web_search`.
- `parallel_tool_calls`.
- OpenRouter `provider.require_parameters`.
- DeepSeek beta strict tool mode.
- `function.strict: true`.
- Multimodal tool inputs, file inputs, and image inputs.
- Codex tool translation.
- SQLite migrations.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- Do not push.
- Storage must not perform HTTP.
- Provider adapters must not import SQLite, TUI, config loaders, or credential
  storage.
- TUI must not mutate `config.toml`.
- Request validation should happen before credential resolution and upstream
  HTTP.
- `internal/openai` owns raw request-shape validation, request marshaling, and
  stream chunk normalization.
- Provider-specific support decisions belong in `internal/provider`.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool definitions, tool arguments, tool
  results, full bearer tokens, full provider request IDs, full account IDs,
  balances, credits, or token-level details.

## Proposed Package Changes

```text
internal/openai/
  types.go       # tool request validation, message validation, stream tools
internal/provider/
  http_chat.go   # provider-specific validation and capability flags
internal/app/
  app.go         # serve/manage smoke assertions
```

Provider semantics:

```text
DeepSeek:
  function tools       -> forward
  tool_choice          -> forward
  function.strict true -> reject

OpenRouter:
  function tools       -> forward
  tool_choice          -> forward
  function.strict true -> reject

Codex:
  tools and tool messages -> reject
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
- DeepSeek and OpenRouter accept and forward valid function tools and
  `tool_choice`,
- nested function schemas are forwarded without `float64` rounding or
  reformatting, including across the `provider_options` and
  `max_completion_tokens` re-marshal path,
- DeepSeek and OpenRouter accept and forward valid assistant tool-call messages
  and tool result messages,
- DeepSeek and OpenRouter preserve non-streaming response `message.tool_calls`
  and `finish_reason:"tool_calls"` for the requesting client,
- DeepSeek and OpenRouter normalized stream chunks preserve valid tool-call
  deltas,
- Codex rejects tool requests and tool conversation messages before credential
  resolution and upstream HTTP,
- invalid tool-related request shapes fail before credential resolution and
  upstream HTTP,
- invalid upstream streaming tool-call shapes fail without leaking raw payloads,
- model cache capabilities advertise `tools` only for DeepSeek and
  OpenRouter-supported models,
- no prompt, completion, raw request body, response body, provider payload, SSE
  chunk, bearer token, provider request ID, account ID, balance, credit, tool
  name marker, tool-call ID marker, tool argument marker, tool schema marker,
  or tool result marker appears in SQLite metadata, TUI output, CLI output, or
  local errors.

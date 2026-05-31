# Plan 033: OpenRouter Parallel Tool Calls

## Goal

Accept OpenRouter `parallel_tool_calls` as a strict OpenRouter-only chat
request field.

OpenRouter documents `parallel_tool_calls` alongside `tools` and `tool_choice`.
Plan 030 added the strict function-tool subset but explicitly left
`parallel_tool_calls` out of scope. This slice completes that narrow tool
control for OpenRouter without changing local tool execution, DeepSeek beta
strict tools, Codex tool handling, or routing policy.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `032`

## Scope

1. Add strict request parsing for top-level `parallel_tool_calls`:
   - accepted value must be a JSON boolean,
   - `null`, strings, numbers, arrays, and objects are invalid,
   - validation must happen before credential resolution and upstream HTTP,
   - validation errors must use static field names and must not echo values.
2. Allow and forward the field for OpenRouter only:
   - OpenRouter accepts `parallel_tool_calls:true`,
   - OpenRouter accepts `parallel_tool_calls:false`,
   - the field is forwarded unchanged to upstream OpenRouter,
   - forwarding must preserve existing tools, tool choice,
     `provider.require_parameters`, JSON schema, logprobs, logit bias,
     advanced sampling, and token-limit translations.
3. Keep other providers strict:
   - DeepSeek rejects `parallel_tool_calls` before credential resolution,
   - Codex rejects `parallel_tool_calls` before credential resolution,
   - top-level `parallel_tool_calls` remains unrelated to
     `provider_options.deepseek` and `provider_options.openrouter`.
4. Preserve privacy and metadata boundaries:
   - do not persist `parallel_tool_calls` values or raw request bodies,
   - do not include request bodies or marker values in local errors, SQLite
     metadata, TUI output, CLI output, or fake-upstream error output.
5. Extend smoke checks without permanent tests:
   - OpenRouter non-streaming requests with `parallel_tool_calls:true` and
     `false` reach fake upstream with exact boolean values,
   - OpenRouter streaming requests with `parallel_tool_calls:true` reach fake
     upstream with exact boolean value,
   - an OpenRouter combined request with tools, tool choice,
     `parallel_tool_calls`, `provider.require_parameters`, JSON schema,
     logprobs, logit bias, `max_completion_tokens`, and an exact advanced
     sampling numeric sentinel such as `top_k` preserves every translation,
   - invalid `parallel_tool_calls` shapes fail before upstream HTTP,
   - DeepSeek and Codex valid-but-unsupported `parallel_tool_calls` requests
     fail before credential resolution and upstream HTTP,
   - no-eligible credential smokes prove invalid values and unsupported
     provider use fail before credential lookup can select anything,
   - privacy scans prove marker values do not leak.

## Out of Scope

- Local tool execution.
- Any change to tool-call response normalization.
- OpenRouter server tools such as `openrouter:web_search`.
- OpenRouter `prediction`, `user`, `verbosity`, `metadata`, `session_id`,
  `service_tier`, `cache_control`, `models`, `route`, or broad provider
  routing controls.
- DeepSeek `parallel_tool_calls`.
- DeepSeek beta strict tool mode.
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
- `internal/openai` owns top-level request parsing and raw type validation.
- Provider-specific support decisions belong in `internal/provider`.
- Request validation should happen before credential resolution and upstream
  HTTP.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool definitions, tool arguments, tool
  results, full bearer tokens, full provider request IDs, full account IDs,
  balances, credits, user identifiers, or provider routing objects.

## Proposed Package Changes

```text
internal/openai/
  types.go       # top-level field parsing, boolean validation, upstream marshal
internal/provider/
  http_chat.go   # provider capability decision
internal/app/
  app.go         # serve/manage smoke assertions
```

Provider semantics:

```text
DeepSeek:
  parallel_tool_calls -> reject

OpenRouter:
  parallel_tool_calls -> forward

Codex:
  parallel_tool_calls -> reject
```

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
rm -rf "$tmpbin" "$tmp"
git diff --check
```

`serve --check` must prove:

- OpenRouter forwards `parallel_tool_calls:true` and
  `parallel_tool_calls:false` for non-streaming chat,
- OpenRouter forwards `parallel_tool_calls:true` for streaming chat,
- combined OpenRouter requests still translate tools, tool choice,
  `provider.require_parameters`, JSON schema, logprobs, logit bias, and token
  limits correctly, and preserve an exact advanced sampling numeric sentinel,
- invalid raw values fail before upstream HTTP,
- DeepSeek and Codex unsupported valid values fail before credential resolution
  and upstream HTTP,
- marker values do not appear in local errors, SQLite metadata, TUI output,
  CLI output, or fake-upstream error output.

`manage --check` should continue proving that TUI output is metadata-only and
does not expose tool or provider option markers.

## Review Questions

1. Is OpenRouter-only `parallel_tool_calls` the right next narrow tool-control
   slice after Plan 030?
2. Should this field require `tools` to be present, or should it be forwarded
   as a provider-supported boolean whenever OpenRouter receives it?
3. Are the no-eligible and invalid-value smoke checks enough to prove validation
   ordering before credential resolution?
4. Does this preserve the boundary between common OpenAI-compatible parsing and
   provider-specific capability decisions?

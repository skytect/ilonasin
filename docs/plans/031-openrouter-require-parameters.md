# Plan 031: OpenRouter Require Parameters

## Goal

Add an explicit OpenRouter routing guard for `provider.require_parameters` so
callers can require OpenRouter to route only to providers that support supplied
feature parameters.

OpenRouter docs say unsupported feature parameters can be ignored by routed
providers unless callers set `provider.require_parameters: true`. Earlier
slices added OpenRouter JSON schema, logprobs, logit bias, and function tools,
but kept `provider.require_parameters` out of scope. This slice makes that
routing guard available through the existing namespaced `provider_options`
escape hatch without exposing broad OpenRouter provider routing controls.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/deepseek-api.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `030`

## Scope

1. Extend OpenRouter `provider_options` only:
   - accepted shape is `provider_options.openrouter.provider`,
   - `provider` must be a JSON object,
   - `provider` may contain only `require_parameters`,
   - `require_parameters` is required when `provider` is present,
   - `require_parameters` must be a boolean,
   - `provider_options.openrouter` may contain `reasoning`, `provider`, or
     both,
   - `provider_options.openrouter` must not be empty.
2. Forward to OpenRouter only:
   - translate to upstream top-level `provider.require_parameters`,
   - never forward the `provider_options` wrapper upstream,
   - preserve existing `provider_options.openrouter.reasoning` translation,
   - preserve existing `max_completion_tokens`, JSON schema, logprobs,
     logit-bias, and tools forwarding.
3. Keep other providers strict:
   - DeepSeek rejects `provider_options.openrouter`,
   - DeepSeek `provider_options.deepseek` does not accept `provider`,
   - Codex continues rejecting all `provider_options`,
   - these rejections must happen before credential resolution, not only before
     upstream HTTP.
4. Keep the escape hatch narrow:
   - client-supplied top-level `provider` remains an unknown field and is
     rejected by the local OpenAI-compatible parser,
   - do not support OpenRouter `provider.order`, `only`, `ignore`,
     `allow_fallbacks`, `sort`, `max_price`, `data_collection`, `zdr`,
     `quantizations`, or any other routing/privacy controls in this slice,
   - do not support OpenRouter top-level `models` or `route`,
   - do not auto-inject `require_parameters` for tools, logprobs, JSON schema,
     or other feature-sensitive fields.
5. Preserve privacy boundaries:
   - validation errors must use static field names and must not echo supplied
     provider option values or unsupported key names,
   - do not persist `provider_options`, provider routing objects, or raw
     request bodies,
   - provider option markers must not appear in SQLite metadata, TUI output,
     CLI output, local errors, or fake-upstream error output.
6. Extend smoke checks without permanent tests:
   - non-streaming OpenRouter requests with only
     `provider.require_parameters` reach fake upstream with exact translated
     top-level `provider`,
   - non-streaming OpenRouter requests combining `require_parameters` with
     reasoning, JSON schema, logprobs, logit bias, max completion tokens, and
     function tools preserve every existing translation,
   - streaming OpenRouter requests with `require_parameters` reach fake
     upstream with exact translated top-level `provider`,
   - invalid provider option shapes fail before credential resolution and
     upstream HTTP,
   - DeepSeek and Codex continue rejecting unsupported provider option shapes
     before credential resolution and upstream HTTP,
   - no-eligible credential smokes prove syntactically valid
     `provider_options.openrouter.provider.require_parameters` requests for
     DeepSeek and Codex fail before credential lookup can select anything,
   - top-level client `provider` remains rejected before credential resolution,
   - privacy scans prove marker values and marker key names do not leak.

## Out of Scope

- Automatic insertion of `provider.require_parameters`.
- OpenRouter provider order, allow/block lists, privacy routing, price caps,
  `data_collection`, `zdr`, quantization filters, or native provider fallback.
- OpenRouter `models`, `route`, `plugins`, `cache_control`, `metadata`,
  `session_id`, `service_tier`, multimodal routing, or tracing/debug fields.
- Cross-provider or cross-model fallback policy changes.
- DeepSeek `user_id`.
- Codex provider options.
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
- `internal/openai` should continue owning common top-level field parsing, but
  OpenRouter-specific `provider_options.openrouter.provider` semantics belong
  in `internal/provider`.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool definitions, tool arguments, tool
  results, full bearer tokens, full provider request IDs, full account IDs,
  balances, credits, or provider routing objects.

## Proposed Package Changes

```text
internal/provider/
  http_chat.go   # validate and translate OpenRouter provider.require_parameters
internal/app/
  app.go         # serve/manage smoke assertions
```

Provider semantics:

```text
DeepSeek:
  provider_options.deepseek.provider     -> reject
  provider_options.openrouter            -> reject

OpenRouter:
  provider_options.openrouter.provider.require_parameters -> forward

Codex:
  provider_options -> reject
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

- OpenRouter non-streaming `provider.require_parameters` reaches fake upstream
  as top-level `provider.require_parameters`,
- OpenRouter streaming `provider.require_parameters` reaches fake upstream as
  top-level `provider.require_parameters`,
- combined OpenRouter requests still translate reasoning, token limits, JSON
  schema, logprobs, logit bias, and tools correctly,
- invalid `provider_options.openrouter.provider` shapes fail before upstream,
- DeepSeek and Codex unsupported provider-option shapes fail before credential
  resolution and upstream HTTP,
- top-level client `provider` remains unsupported before credential resolution,
- provider option markers do not appear in local errors, SQLite metadata,
  TUI output, CLI output, or fake-upstream error output.

`manage --check` should continue proving that TUI output is metadata-only and
does not expose provider option markers.

## Review Questions

1. Is `provider.require_parameters` the right next narrow OpenRouter routing
   option, or should it wait until broader capability-aware routing?
2. Does allowing only `require_parameters` preserve the provider-options escape
   hatch boundary?
3. Should `provider_options.openrouter` accept both `reasoning` and `provider`
   in one object, or should this remain one feature per slice?
4. Are the smoke checks strong enough to catch silent forwarding of the
   `provider_options` wrapper or private marker leakage?

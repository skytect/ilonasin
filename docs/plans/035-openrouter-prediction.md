# Plan 035: OpenRouter Prediction

## Goal

Accept OpenRouter `prediction` as a strict OpenRouter-only chat request field.

OpenRouter documents `prediction?: { type: "content"; content: string }` for
chat completions as an OpenAI-compatible latency optimization where the caller
provides predicted output content. This field can contain output-like text, so
it must follow the same privacy boundary as prompts, completions, request
bodies, tool arguments, and provider payloads: forward it only when explicitly
supported, never persist it, and never echo marker values in local output.

## Architecture Inputs

- `AGENTS.md`
- all markdown files under `docs/**`
- current official OpenRouter docs for chat completion request parameters
- current official DeepSeek docs
- prior plans `001` through `034`

## Scope

1. Add strict request parsing for top-level `prediction`:
   - accepted value must be a JSON object,
   - the object must contain exactly `type` and `content`,
   - `type` must be the string `content`,
   - `content` must be a JSON string,
   - empty `content` is allowed because OpenRouter documents only the JSON
     type, not a non-empty constraint,
   - `null`, booleans, numbers, arrays, missing keys, unsupported `type`,
     extra keys, and non-string `content` are invalid,
   - validation must happen before credential resolution and upstream HTTP,
   - validation errors must use static field names and must not echo values.
2. Allow and forward the field for OpenRouter only:
   - OpenRouter accepts `prediction`,
   - the object is forwarded unchanged to upstream OpenRouter,
   - forwarding must preserve existing `user`, tools, tool choice,
     `parallel_tool_calls`, `provider.require_parameters`, JSON schema,
     logprobs, logit bias, advanced sampling, and token-limit translations.
3. Keep other providers strict:
   - DeepSeek rejects top-level `prediction` before credential resolution,
   - Codex rejects top-level `prediction` before credential resolution,
   - `prediction` is not mapped to DeepSeek beta prefix completion,
   - `prediction` is not translated to Codex request fields.
4. Preserve privacy and metadata boundaries:
   - do not persist `prediction` values or raw request bodies,
   - do not include prediction marker values in local errors, SQLite metadata,
     TUI output, CLI output, fake-upstream error output, or success responses.
5. Extend smoke checks without permanent tests:
   - OpenRouter non-streaming requests with `prediction` reach fake upstream
     with the exact object,
   - OpenRouter non-streaming requests with empty `prediction.content` reach
     fake upstream with the exact object,
   - OpenRouter streaming requests with `prediction` reach fake upstream with
     the exact object,
   - an OpenRouter combined request with `prediction`, `user`, tools, tool
     choice, `parallel_tool_calls`, `provider.require_parameters`, JSON
     schema, logprobs, logit bias, `max_completion_tokens`, and an exact
     advanced sampling numeric sentinel preserves every translation,
   - invalid `prediction` shapes, including missing `type`, missing
     `content`, unsupported `type`, extra keys, `null`, booleans, numbers,
     arrays, and non-string `content`, fail before upstream HTTP,
   - DeepSeek and Codex valid-but-unsupported `prediction` requests fail before
     credential resolution and upstream HTTP,
   - no-eligible credential smokes prove invalid values and unsupported
     provider use fail before credential lookup can select anything,
   - privacy scans prove prediction marker values do not leak.

## Out of Scope

- OpenRouter `verbosity`, `metadata`, `session_id`, `service_tier`,
  `cache_control`, `models`, `route`, `plugins`, or broad provider routing
  controls.
- Translating OpenRouter `prediction` to DeepSeek beta prefix completion.
- Supporting final assistant-message prefill.
- DeepSeek beta base URL selection.
- Codex prediction or prefill support.
- Storing prediction metadata.
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
  results, predicted output content, full bearer tokens, full provider request
  IDs, full account IDs, balances, credits, user identifiers, or provider
  routing objects.

## Proposed Package Changes

```text
internal/openai/
  types.go       # top-level prediction parsing, object validation, marshal
internal/provider/
  http_chat.go   # provider capability decision
internal/app/
  app.go         # serve/manage smoke assertions
```

Provider semantics:

```text
DeepSeek:
  prediction -> reject

OpenRouter:
  prediction -> forward

Codex:
  prediction -> reject
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

- OpenRouter forwards `prediction` for non-streaming chat,
- OpenRouter forwards `prediction` with empty `content` for non-streaming chat,
- OpenRouter forwards `prediction` for streaming chat,
- combined OpenRouter requests still translate `user`, tools, tool choice,
  `parallel_tool_calls`, `provider.require_parameters`, JSON schema,
  logprobs, logit bias, token limits, and advanced sampling correctly,
- invalid raw values, including missing `type`, missing `content`,
  unsupported `type`, extra keys, `null`, booleans, numbers, arrays, and
  non-string `content`, fail before upstream HTTP,
- DeepSeek and Codex unsupported valid values fail before credential resolution
  and upstream HTTP,
- marker values do not appear in local errors, SQLite metadata, TUI output,
  CLI output, fake-upstream error output, or success responses.

`manage --check` should continue proving that TUI output is metadata-only and
does not expose prediction, user, tool, or provider option markers.

## Review Questions

1. Is OpenRouter-only `prediction` the right next narrow common-field slice
   after `user`?
2. Should this slice accept empty `content` because the docs only specify
   string type, or reject it as useless?
3. Is exact-key validation for `type` and `content` the right local strictness,
   given this router does not silently forward unknown fields?
4. Does this preserve the boundary between common OpenAI-compatible parsing and
   provider-specific capability decisions?

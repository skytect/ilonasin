# Plan 026: OpenRouter Sampling Penalties

## Goal

Accept `presence_penalty` and `frequency_penalty` as strict typed chat request
fields for OpenRouter, while rejecting them for DeepSeek and Codex.

The architecture says common generation controls should be represented as
request fields instead of model-name suffixes or loose provider payloads.
OpenRouter documents both fields on chat completions with a `-2.0` to `2.0`
range. DeepSeek documents both fields as deprecated and no-op. The current
parser rejects them as unknown fields for every provider, which blocks a
documented OpenRouter request shape and avoids no-op behavior only by accident.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `025`
- official OpenRouter chat completion and parameter docs checked on
  2026-05-31
- official DeepSeek chat completion docs checked on 2026-05-31

## Scope

1. Extend the typed OpenAI-compatible chat request:
   - add `PresencePenalty *float64` mapped from `presence_penalty`,
   - add `FrequencyPenalty *float64` mapped from `frequency_penalty`,
   - include both fields in the strict top-level allowed-key set.
2. Validate penalty values before credential resolution:
   - validate the raw JSON values before unmarshalling into typed `*float64`
     fields, or use `json.RawMessage` / `json.Number`, so oversized numeric
     literals cannot fail through a Go unmarshal error that echoes the raw
     value,
   - if either field is present it must be a JSON number, not `null`, string,
     object, array, or boolean,
   - each value must be finite,
   - each value must be in the inclusive range `-2.0` to `2.0`,
   - invalid values fail with static field-name errors before credential
     resolution and upstream HTTP.
3. Make provider capability validation explicit:
   - OpenRouter accepts both fields and forwards them unchanged,
   - DeepSeek rejects both fields because they are documented deprecated/no-op,
   - Codex rejects both fields until Codex request semantics are separately
     designed.
4. Preserve existing privacy and metadata behavior:
   - do not persist penalty values in SQLite metadata,
   - do not print raw request bodies or provider payloads,
   - local validation errors must use static field names and must not echo raw
     request values.
5. Extend smoke checks without permanent tests:
   - OpenRouter non-streaming and streaming requests with
     `presence_penalty` alone reach the fake upstream with the exact value,
   - OpenRouter non-streaming and streaming requests with
     `frequency_penalty` alone reach the fake upstream with the exact value,
   - OpenRouter non-streaming and streaming requests with both penalties reach
     the fake upstream with the exact values,
   - accepted OpenRouter boundary values include `-2.0` and `2.0`,
   - DeepSeek non-streaming and streaming penalty requests fail before upstream
     HTTP,
   - Codex penalty requests fail before upstream HTTP,
   - null, string, boolean, object, array, out-of-range, and non-finite-like
     values such as `1e309` fail before upstream HTTP,
   - out-of-range smoke includes just-outside values below `-2.0` and above
     `2.0`,
   - invalid penalty values fail before credential resolution for configured
     providers with no eligible credential,
   - valid-but-unsupported DeepSeek and Codex penalty requests fail before
     credential resolution for configured providers with no eligible
     credential,
   - overflow-like values such as `1e309` fail without echoing the supplied
     value,
   - penalty marker values do not appear in SQLite metadata, TUI output, CLI
     output, local errors, or response bodies.

## Out of Scope

- OpenRouter `repetition_penalty`, `top_k`, `min_p`, `top_a`, `seed`,
  `logit_bias`, `metadata`, `session_id`, or routing fields.
- DeepSeek no-op pass-through.
- Codex generation-control mapping.
- Provider capability routing with `provider.require_parameters`.
- Recording penalty values in metadata.
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
- `internal/openai` may model and marshal the common request fields, but
  provider-specific support and no-op rejection belong in `internal/provider`.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens, full provider request IDs, full account IDs, balances, credits, or
  penalty values.

## Proposed Package Changes

```text
internal/openai/
  types.go       # raw-validate, parse, and marshal penalty fields
internal/provider/
  http_chat.go   # provider-specific support and rejection
internal/app/
  app.go         # serve/manage smoke assertions
```

Provider semantics:

```text
DeepSeek:
  presence_penalty  -> reject
  frequency_penalty -> reject

OpenRouter:
  presence_penalty  -> forward
  frequency_penalty -> forward

Codex:
  presence_penalty  -> reject
  frequency_penalty -> reject
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
- OpenRouter accepts and forwards valid `presence_penalty` and
  `frequency_penalty`, independently and together, for non-streaming and
  streaming chat,
- DeepSeek rejects both fields before credential resolution and upstream HTTP,
- Codex rejects both fields before credential resolution and upstream HTTP,
- invalid penalty values fail before credential resolution and upstream HTTP,
- overflow-like penalty values fail before typed unmarshal errors can echo raw
  values,
- errors do not echo supplied values,
- no penalty value, prompt, completion, raw request body, response body,
  provider payload, SSE chunk, bearer token, provider request ID, account ID,
  balance, or credit appears in SQLite metadata, TUI output, CLI output, local
  errors, or response bodies.

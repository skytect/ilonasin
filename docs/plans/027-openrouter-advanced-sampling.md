# Plan 027: OpenRouter Advanced Sampling

## Goal

Accept OpenRouter-only advanced sampling controls as strict typed chat request
fields while rejecting them for DeepSeek and Codex.

OpenRouter documents `top_k`, `min_p`, `top_a`, `repetition_penalty`, and
`seed` for chat completions. DeepSeek chat docs do not document these fields.
The local API currently rejects them as unknown fields, which blocks documented
OpenRouter request shapes while preserving DeepSeek behavior only by accident.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `026`
- official OpenRouter parameter docs checked on 2026-05-31
- official DeepSeek chat completion docs checked on 2026-05-31

## Scope

1. Extend the typed OpenAI-compatible chat request:
   - add `TopK *json.Number` mapped from `top_k`,
   - add `MinP *json.Number` mapped from `min_p`,
   - add `TopA *json.Number` mapped from `top_a`,
   - add `RepetitionPenalty *json.Number` mapped from
     `repetition_penalty`,
   - add `Seed *json.Number` mapped from `seed`,
   - include all five fields in the strict top-level allowed-key set.
2. Validate values before credential resolution:
   - validate raw JSON values before unmarshalling into typed fields, so
     oversized numeric literals cannot fail through a Go unmarshal error that
     echoes raw values,
   - store the accepted numeric token text in `json.Number` fields and marshal
     that token text upstream rather than converting accepted values through
     `float64`,
   - `top_k` must be a JSON integer in the inclusive range `0` to
     `math.MaxInt64`,
   - `min_p` and `top_a` must be JSON numbers in the inclusive range `0` to
     `1`,
   - `repetition_penalty` must be a JSON number in the inclusive range `0` to
     `2`,
   - `seed` must be a JSON integer in the inclusive `int64` range,
   - `null`, string, boolean, object, array, non-integer integer fields,
     non-finite-like values, and out-of-range values fail with static
     field-name errors.
3. Make provider capability validation explicit:
   - OpenRouter accepts all five fields and forwards them unchanged,
   - DeepSeek rejects all five fields because they are not documented for chat,
   - Codex rejects all five fields until Codex request semantics are separately
     designed.
4. Preserve existing privacy and metadata behavior:
   - do not persist sampling option values in SQLite metadata,
   - do not print raw request bodies or provider payloads,
   - local validation errors must use static field names and must not echo raw
     request values.
5. Extend smoke checks without permanent tests:
   - OpenRouter non-streaming and streaming requests with each field alone
     reach the fake upstream with the exact JSON numeric token,
   - OpenRouter non-streaming and streaming requests with all five fields reach
     the fake upstream with exact JSON numeric tokens,
   - fake-upstream assertions decode request bodies with `UseNumber` or inspect
     `json.RawMessage`, never `map[string]any` float comparisons for these
     fields,
   - accepted boundary smokes include `top_k = 9223372036854775807`,
     `seed = -9223372036854775808`, and
     `seed = 9223372036854775807`,
   - DeepSeek and Codex requests with these fields fail before upstream HTTP,
   - invalid raw values fail before credential resolution for configured
     providers with no eligible credential,
   - valid-but-unsupported DeepSeek and Codex requests fail before credential
     resolution for configured providers with no eligible credential,
   - overflow-like values fail without echoing the supplied value,
   - marker values do not appear in SQLite metadata, TUI output, CLI output,
     local errors, or response bodies.

## Out of Scope

- OpenRouter `logit_bias`, `metadata`, `session_id`, `prediction`, `user`,
  `models`, `provider`, `route`, `plugins`, cache controls, tracing, BYOK, or
  provider routing fields.
- DeepSeek pass-through for undocumented sampling fields.
- Codex generation-control mapping.
- Provider capability routing with `provider.require_parameters`.
- Recording sampling values in metadata.
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
  provider-specific support belongs in `internal/provider`.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens, full provider request IDs, full account IDs, balances, credits, or
  sampling option values.

## Proposed Package Changes

```text
internal/openai/
  types.go       # raw-validate, parse, and marshal sampling fields
internal/provider/
  http_chat.go   # provider-specific support and rejection
internal/app/
  app.go         # serve/manage smoke assertions
```

Provider semantics:

```text
DeepSeek:
  top_k              -> reject
  min_p              -> reject
  top_a              -> reject
  repetition_penalty -> reject
  seed               -> reject

OpenRouter:
  top_k              -> forward
  min_p              -> forward
  top_a              -> forward
  repetition_penalty -> forward
  seed               -> forward

Codex:
  top_k              -> reject
  min_p              -> reject
  top_a              -> reject
  repetition_penalty -> reject
  seed               -> reject
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
- OpenRouter accepts and forwards valid `top_k`, `min_p`, `top_a`,
  `repetition_penalty`, and `seed`, independently and together, for
  non-streaming and streaming chat, preserving accepted JSON numeric tokens,
- DeepSeek rejects all five fields before credential resolution and upstream
  HTTP,
- Codex rejects all five fields before credential resolution and upstream HTTP,
- invalid values fail before credential resolution and upstream HTTP,
- overflow-like values fail before typed unmarshal errors can echo raw values,
- errors do not echo supplied values,
- no sampling option value, prompt, completion, raw request body, response
  body, provider payload, SSE chunk, bearer token, provider request ID, account
  ID, balance, or credit appears in SQLite metadata, TUI output, CLI output,
  local errors, or response bodies.

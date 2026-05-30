# Plan 025: OpenRouter JSON Schema Response Format

## Goal

Accept and forward OpenRouter `response_format.type=json_schema` as an
explicit provider-supported structured-output mode while continuing to reject
it for DeepSeek and Codex.

The architecture says structured-output modes should be represented as request
fields with provider-specific capability checks. The docs say OpenRouter
supports JSON Schema through `response_format`, while DeepSeek only documents
`text` and `json_object` for chat response format. The current adapter rejects
`json_schema` for every provider, which blocks a documented OpenRouter feature.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `024`

## Scope

1. Make response-format validation provider-aware:
   - DeepSeek accepts only `{"type":"text"}` and `{"type":"json_object"}`,
   - OpenRouter accepts those plus `{"type":"json_schema","json_schema":...}`,
   - Codex continues to reject `response_format`.
2. Validate OpenRouter JSON Schema shape strictly enough to avoid arbitrary
   provider payload pass-through:
   - `response_format` must be a JSON object,
   - `type` must be the string `json_schema`,
   - `json_schema` must be a JSON object,
   - top-level `response_format` keys must be exactly `type` and
     `json_schema`,
   - `json_schema.name` is required,
   - `json_schema.name` must be a non-empty string with at most 64 bytes,
   - `json_schema.name` may contain only ASCII letters, ASCII digits,
     underscores, and dashes,
   - `json_schema.strict`, when present, must be a boolean,
   - `json_schema.schema` is required and must be a JSON object,
   - `json_schema` must not contain unsupported top-level keys beyond
     `name`, `strict`, `schema`, and `description`,
   - `description`, when present, must be a string.
3. Preserve common JSON object behavior:
   - existing `text` and `json_object` requests keep working for DeepSeek and
     OpenRouter,
   - `text` and `json_object` response formats may contain only the `type`
     field,
   - the common upstream marshaler may continue forwarding validated
     `response_format` unchanged after provider-owned validation.
4. Preserve privacy boundaries:
   - do not persist `response_format`,
   - validation errors must use static field paths and must not echo supplied
     values, unknown keys, schema bodies, wrapper bodies, names, or
     descriptions,
   - do not print schema bodies, schema names, descriptions, or raw wrapper
     contents in local errors, TUI output, smoke output, or metadata,
   - provider adapters may hold the schema only long enough to validate and
     build the upstream request body.
5. Extend smoke checks without permanent tests:
   - OpenRouter non-streaming and streaming JSON Schema requests reach the
     fake upstream with the validated `response_format`,
   - DeepSeek JSON Schema requests fail before credential resolution and
     upstream HTTP,
   - invalid OpenRouter JSON Schema shapes fail before credential resolution
     and upstream HTTP,
   - Codex response-format requests fail before credential resolution and
     upstream HTTP,
   - invalid cases include `response_format:null`, missing or non-string
     `type`, unsupported `type`, top-level unknown keys, missing, null, or
     non-object `json_schema`, missing, empty, non-string, invalid-character,
     and too-long `json_schema.name`, missing, null, or non-object
     `json_schema.schema`, non-boolean `strict`, non-string `description`,
     and unsupported `json_schema` keys,
   - private markers placed in `json_schema.name`, `description`, top-level
     unknown keys, nested unknown keys, and nested schema bodies do not appear
     in local error bodies, SQLite metadata, TUI output, or CLI output,
   - Codex still rejects `response_format`,
   - existing unsupported-field checks continue to pass.

## Out of Scope

- Tool calling or strict tool schema support.
- DeepSeek JSON Schema support.
- OpenRouter grammar or provider-specific structured-output variants.
- OpenRouter `type=grammar` and `type=python`, which remain rejected.
- Capability-aware routing with `provider.require_parameters`.
- Storing schema metadata.
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
- `internal/openai` may parse and forward common request fields, but
  provider-specific response-format capability checks belong in
  `internal/provider`.
- Unknown response-format keys are errors unless explicitly allowed for the
  selected provider.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens, full provider request IDs, full account IDs, balances, credits, or
  schema bodies.

## Proposed Package Changes

```text
internal/provider/
  http_chat.go   # provider-aware response_format validation
internal/app/
  app.go         # serve/manage smoke assertions
```

Validation semantics:

```text
DeepSeek:
  response_format.type=text        -> accept
  response_format.type=json_object -> accept
  response_format.type=json_schema -> reject

OpenRouter:
  response_format.type=text        -> accept
  response_format.type=json_object -> accept
  response_format.type=json_schema -> accept with strict json_schema wrapper

Codex:
  response_format -> reject
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
- OpenRouter accepts and forwards valid `response_format.type=json_schema` for
  non-streaming and streaming chat,
- DeepSeek rejects `response_format.type=json_schema` before credential
  resolution and upstream HTTP,
- invalid OpenRouter JSON Schema shapes fail before credential resolution and
  upstream HTTP,
- invalid Codex response-format requests fail before credential resolution and
  upstream HTTP,
- invalid response-format errors never echo supplied values, unknown keys,
  schema bodies, wrapper bodies, schema names, or descriptions,
- Codex still rejects `response_format`,
- existing `text` and `json_object` response formats continue to work,
- no schema body, schema name, description, prompt, completion, raw request
  body, response body, provider payload, SSE chunk, bearer token, provider
  request ID, account ID, balance, or credit appears in SQLite metadata, TUI
  output, CLI output, or local errors.

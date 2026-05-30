# Plan 024: Max Completion Tokens

## Goal

Accept `max_completion_tokens` as a strict OpenAI-compatible chat request
field and translate it safely at the provider boundary.

The architecture says common generation controls and token limits should be
modeled as request fields, not hidden model suffixes. OpenRouter examples use
`max_completion_tokens`, while DeepSeek documents `max_tokens`. The current
parser rejects `max_completion_tokens` as an unknown field, which breaks a
common OpenAI-compatible client shape.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `023`

## Scope

1. Extend the typed OpenAI-compatible chat request:
   - add `MaxCompletionTokens *int` mapped from `max_completion_tokens`,
   - include `max_completion_tokens` in allowed top-level request keys,
   - keep request parsing strict for all other unknown fields.
2. Validate token limit fields consistently:
   - `max_tokens`, when present, must be a positive integer,
   - `max_completion_tokens`, when present, must be a positive integer,
   - requests containing both fields are rejected,
   - invalid values fail before credential resolution and upstream HTTP.
3. Translate at the provider adapter boundary:
   - DeepSeek receives `max_tokens` whether the client supplied
     `max_tokens` or `max_completion_tokens`,
   - OpenRouter preserves `max_completion_tokens` when supplied,
   - OpenRouter preserves `max_tokens` when that field is supplied,
   - no upstream request contains both token-limit fields,
   - Codex continues to reject both fields until Codex request semantics are
     separately designed.
4. Preserve existing request and privacy behavior:
   - no token-limit field is persisted in metadata,
   - no raw request body or provider payload is stored or displayed,
   - provider option translation remains separate from token-limit
     translation.
5. Extend smoke checks without permanent tests:
   - DeepSeek non-streaming and streaming requests with
     `max_completion_tokens` reach fake upstream as `max_tokens`,
   - OpenRouter non-streaming and streaming requests with
     `max_completion_tokens` reach fake upstream as `max_completion_tokens`,
   - requests with both `max_tokens` and `max_completion_tokens` fail before
     credential resolution and upstream HTTP,
   - null, zero, negative, non-integer, and non-number values fail before
     reaching credential resolution and upstream HTTP,
   - invalid and conflicting token-limit fields return validation errors even
     for a configured provider that has no eligible credential,
   - Codex rejects `max_completion_tokens` before reaching upstream,
   - existing `max_tokens` behavior for DeepSeek and OpenRouter remains intact.

## Out of Scope

- Codex `max_output_tokens` mapping.
- OpenRouter Responses API.
- Tool calling, JSON Schema, multimodal messages, embeddings, rerank, audio,
  or video.
- Provider-specific routing policy fields such as `models`, `provider`, and
  `route`.
- Recording token-limit request fields in SQLite.
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
- `internal/openai` may model the strict common request field but must not
  encode provider-specific naming rules.
- Provider-specific upstream field names belong in `internal/provider`.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens, full provider request IDs, full account IDs, balances, or credits.

## Proposed Package Changes

```text
internal/openai/
  types.go       # parse and validate max_completion_tokens
internal/provider/
  http_chat.go   # provider-specific token-limit translation
internal/app/
  app.go         # serve/manage smoke assertions
```

Translation semantics:

```text
DeepSeek:
  max_tokens -> max_tokens
  max_completion_tokens -> max_tokens

OpenRouter:
  max_tokens -> max_tokens
  max_completion_tokens -> max_completion_tokens

Codex:
  max_tokens -> reject
  max_completion_tokens -> reject
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
- `max_completion_tokens` is accepted for DeepSeek and OpenRouter chat,
- DeepSeek receives a single upstream `max_tokens` field,
- OpenRouter receives a single upstream `max_completion_tokens` field when
  supplied by the client,
- existing `max_tokens` requests still work for DeepSeek and OpenRouter,
- invalid or conflicting token-limit fields fail before credential resolution
  and upstream HTTP,
- invalid and conflicting token-limit fields return validation errors even for
  a configured provider that has no eligible credential,
- Codex rejects `max_completion_tokens`,
- no prompt, completion, raw request body, response body, provider payload,
  SSE chunk, bearer token, provider request ID, account ID, balance, or credit
  appears in SQLite metadata, TUI output, CLI output, or local errors.

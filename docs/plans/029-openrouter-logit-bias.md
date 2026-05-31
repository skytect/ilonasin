# Plan 029: OpenRouter Logit Bias

## Goal

Accept `logit_bias` for OpenRouter chat completions as a strict typed request
field, while continuing to reject it for DeepSeek and Codex.

OpenRouter documents `logit_bias` as a chat parameter mapping tokenizer token
IDs to bias values from `-100` through `100`. The current DeepSeek chat docs in
this repository do not list `logit_bias`, and Codex request semantics are
separate from generic chat completions. This slice closes the OpenRouter gap
without adding arbitrary passthrough.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `028`
- official OpenRouter parameter docs checked on 2026-05-31:
  `https://openrouter.ai/docs/api/reference/parameters`
- official DeepSeek chat completion docs checked on 2026-05-31:
  `https://api-docs.deepseek.com/api/create-chat-completion`

## Scope

1. Keep strict OpenAI-compatible parsing:
   - `logit_bias`, when present, must be a JSON object,
   - object keys must be canonical non-empty decimal token ID strings,
   - leading zeros are rejected except for the single key `"0"`,
   - token IDs must be non-negative integers no larger than `math.MaxInt64`,
   - token ID keys must not use signs, decimal points, exponents, spaces, or
     non-decimal characters,
   - object values must be JSON numbers from `-100` through `100` inclusive,
   - accepted value number tokens must be preserved with `json.Number` or raw
     JSON so forwarding does not round decimal or exponent spellings through
     `float64`,
   - empty objects are valid and forward as `{}`,
   - raw `json.RawMessage` validation must run before typed unmarshal so
     overflow-like values cannot fail through Go's decode path with raw values
     echoed in errors,
   - `null`, string, boolean, array, nested object values, non-numeric values,
     out-of-range values, non-finite values, and overflow-like values fail
     before credential resolution and upstream HTTP.
2. Forward supported request fields:
   - OpenRouter accepts and forwards `logit_bias`,
   - DeepSeek rejects `logit_bias` before credential resolution and upstream
     HTTP,
   - Codex rejects `logit_bias` before credential resolution and upstream HTTP,
   - no provider receives `logit_bias` after local rejection.
3. Update capability metadata:
   - OpenRouter model discovery maps supported parameter `logit_bias` to a
     `logit_bias` capability flag,
   - DeepSeek static model capabilities remain unchanged,
   - Codex capabilities remain unchanged.
4. Preserve privacy and metadata boundaries:
   - do not persist `logit_bias` keys, values, request bodies, or raw provider
     payloads,
   - do not include token IDs, bias values, raw request values, or raw provider
     payloads in local errors,
   - provider adapters may hold `logit_bias` only long enough to validate and
     send the upstream request.
5. Extend smoke checks without permanent tests:
   - OpenRouter non-streaming requests with representative `logit_bias` values
     reach fake upstream with exact fields,
   - OpenRouter streaming requests with representative `logit_bias` values
     reach fake upstream with exact fields,
   - OpenRouter accepts boundary values `-100`, `0`, and `100`,
   - OpenRouter preserves accepted numeric spellings exactly when forwarding,
   - OpenRouter accepts a `math.MaxInt64` token ID key and forwards it exactly,
   - OpenRouter accepts key `"0"` and rejects leading-zero keys such as `"01"`,
   - DeepSeek and Codex valid `logit_bias` requests fail before upstream HTTP,
   - invalid values fail before upstream HTTP and before credential resolution,
   - invalid token ID keys fail before upstream HTTP and before credential
     resolution for empty, signed, decimal, exponent, space-padded,
     non-decimal, leading-zero, and above-`math.MaxInt64` keys,
   - the fake upstream decodes request bodies with `UseNumber` or raw messages
     and asserts exact forwarding when `logit_bias` is combined with
     `max_completion_tokens` and OpenRouter `provider_options`,
   - model cache capabilities advertise `logit_bias` only when OpenRouter model
     discovery reports it in `supported_parameters`,
   - privacy scans prove token ID markers and bias markers do not appear in
     SQLite metadata, TUI output, CLI output, or local errors.

## Out of Scope

- DeepSeek `logit_bias` support.
- Codex `logit_bias` support.
- OpenRouter `provider.require_parameters`.
- Tokenizer-specific validation against model vocabularies.
- Provider routing fields such as `provider`, `models`, and `route`.
- Persisting token-level telemetry or bias maps.
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
- `internal/openai` owns raw request-shape validation and request marshaling.
- Provider-specific support decisions belong in `internal/provider`.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens, full provider request IDs, full account IDs, balances, credits, or
  token-level details.

## Proposed Package Changes

```text
internal/openai/
  types.go       # raw validation, request model, upstream marshal
internal/provider/
  http_chat.go   # provider-specific validation and capability flags
internal/app/
  app.go         # serve/manage smoke assertions
```

Provider semantics:

```text
DeepSeek:
  logit_bias -> reject

OpenRouter:
  logit_bias -> forward after strict validation

Codex:
  logit_bias -> reject
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
- OpenRouter accepts and forwards valid `logit_bias` for non-streaming and
  streaming chat,
- DeepSeek and Codex reject `logit_bias` before credential resolution and
  upstream HTTP,
- invalid `logit_bias` shapes fail before credential resolution and upstream
  HTTP,
- invalid `logit_bias` token ID keys fail before credential resolution and
  upstream HTTP,
- a valid `math.MaxInt64` token ID key reaches OpenRouter upstream exactly,
- numeric `logit_bias` values are forwarded without `float64` rounding or
  reformatting, including when combined with `max_completion_tokens` and
  OpenRouter `provider_options`,
- model cache capabilities advertise `logit_bias` only for OpenRouter-supported
  models,
- no prompt, completion, raw request body, response body, provider payload, SSE
  chunk, bearer token, provider request ID, account ID, balance, credit, token
  ID marker, or bias marker appears in SQLite metadata, TUI output, CLI output,
  or local errors.

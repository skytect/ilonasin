# Plan 022: Provider Reasoning Options

## Goal

Implement explicit, namespaced provider reasoning options for DeepSeek and
OpenRouter chat completions.

The architecture says fast mode, reasoning effort, and similar behavior should
be request fields rather than model suffixes. It also allows provider-specific
escape hatches only when they are explicit and namespaced. The current request
type accepts `provider_options`, but adapter validation rejects it for every
provider. This slice makes the documented reasoning controls usable without
opening an arbitrary provider payload pass-through.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `021`

## Scope

1. Keep `provider_options` as the only top-level escape hatch:
   - it must be a JSON object when present,
   - it must contain exactly one provider namespace matching the selected
     provider type,
   - unknown namespaces or extra namespaces are rejected,
   - `codex` rejects `provider_options` until Codex request semantics are
     separately designed.
2. Add DeepSeek provider options validation and translation:
   - accepted shape is `provider_options.deepseek`,
   - allowed keys are `thinking` and `reasoning_effort`,
   - `thinking` must be an object containing only `type`,
   - `thinking.type` must be `enabled` or `disabled`,
   - `reasoning_effort` must be one of `high` or `max`,
   - translate to upstream top-level `thinking` and `reasoning_effort`,
   - never forward the `provider_options` wrapper upstream.
3. Add OpenRouter provider options validation and translation:
   - accepted shape is `provider_options.openrouter`,
   - allowed key is `reasoning`,
   - `reasoning` must be an object containing only `effort`, `max_tokens`,
     `exclude`, or `enabled`,
   - `effort`, when present, must be a string from `xhigh`, `high`, `medium`,
     `low`, `minimal`, or `none`,
   - `max_tokens`, when present, must be a positive integer,
   - `effort` and `max_tokens` are mutually exclusive,
   - `exclude` and `enabled`, when present, must be booleans,
   - translate to upstream top-level `reasoning`,
   - never forward the `provider_options` wrapper upstream.
4. Preserve existing unsupported feature behavior:
   - `tools`, `tool_choice`, `logprobs`, and `top_logprobs` remain rejected,
   - `response_format` validation remains provider-adapter-owned,
   - standard chat fields still use the existing common upstream marshaler.
5. Preserve privacy and metadata boundaries:
   - do not persist `provider_options`,
   - do not print option bodies in local errors, TUI output, smoke output, or
     metadata,
   - provider adapters may hold the option values only long enough to validate
     and build the upstream request body.
6. Extend smoke checks without permanent tests:
   - DeepSeek non-streaming and streaming requests with valid provider options
     reach the fake upstream with top-level `thinking` and
     `reasoning_effort`,
   - OpenRouter non-streaming and streaming requests with valid provider
     options reach the fake upstream with top-level `reasoning`,
   - the fake upstream rejects any forwarded `provider_options` wrapper,
   - wrong namespace, unknown keys, bad types, unsupported effort values, and
     mutually exclusive OpenRouter `effort` plus `max_tokens` fail before
     reaching upstream,
   - Codex `provider_options` fails before reaching upstream,
   - existing unsupported-field checks continue to pass,
   - privacy scans continue to prove raw prompts, completions, request bodies,
     provider payloads, raw SSE chunks, tool arguments, bearer tokens, provider
     request IDs, account IDs, balances, and credits are not stored or printed.

## Out of Scope

- Codex fast mode or reasoning-effort request semantics.
- Generic provider pass-through.
- DeepSeek `user_id`.
- DeepSeek alias mapping for `low`, `medium`, or `xhigh`.
- OpenRouter `provider`, `models`, `route`, `plugins`, cache controls, tracing,
  BYOK, privacy routing, or `provider.require_parameters`.
- Tool calling or JSON schema support.
- Response-side reasoning normalization changes.
- SQLite migrations.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- Do not push.
- Storage must not perform HTTP.
- Provider adapters must not import SQLite, TUI, config loaders, or credential
  storage.
- TUI must not mutate `config.toml`.
- The local server should validate provider options before credential
  resolution and upstream HTTP.
- Unknown provider option keys are errors, not ignored fields.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens, full provider request IDs, full account IDs, balances, or credits.

## Proposed Package Changes

```text
internal/provider/
  http_chat.go   # per-provider option validation and upstream translation
internal/app/
  app.go         # serve/manage smoke assertions
```

Validation and translation should stay in the adapter boundary because the
allowed option namespace and upstream field names depend on the resolved
provider type. `internal/openai` should remain limited to common
OpenAI-compatible parsing and common request helpers.

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
- valid DeepSeek provider options are translated to upstream top-level
  `thinking` and `reasoning_effort`,
- valid OpenRouter provider options are translated to upstream top-level
  `reasoning`,
- `provider_options` is never forwarded upstream,
- invalid or wrong-namespace provider options fail before credential resolution
  and upstream HTTP,
- Codex still rejects `provider_options`,
- existing unsupported-field checks remain intact,
- no provider option body, prompt, completion, raw request body, response body,
  provider payload, SSE chunk, bearer token, provider request ID, account ID,
  balance, or credit appears in SQLite metadata, TUI output, CLI output, or
  local errors.

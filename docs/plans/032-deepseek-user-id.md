# Plan 032: DeepSeek User ID

## Goal

Add explicit support for DeepSeek `user_id` through the existing namespaced
`provider_options.deepseek` escape hatch.

DeepSeek documents `user_id` as a provider-specific identity field for
content-safety isolation, KV-cache privacy isolation, and scheduling isolation.
It is not equivalent to the OpenAI `user` field and should not be accepted as a
generic top-level request field. Earlier slices intentionally left this field
out of scope. This slice makes it available only when callers explicitly opt in
through DeepSeek provider options.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/openrouter-api.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `031`

## Scope

1. Extend DeepSeek `provider_options` only:
   - accepted shape is `provider_options.deepseek.user_id`,
   - `user_id` must be a JSON string when present,
   - `user_id` must be non-empty,
   - `user_id` must be at most 512 bytes,
   - `user_id` must match ASCII `[A-Za-z0-9_-]+`,
   - `provider_options.deepseek` may contain `thinking`,
     `reasoning_effort`, `user_id`, or valid combinations,
   - `provider_options.deepseek` must not be empty.
2. Forward to DeepSeek only:
   - translate to upstream top-level `user_id`,
   - never forward the `provider_options` wrapper upstream,
   - preserve existing DeepSeek `thinking`, `reasoning_effort`,
     `max_completion_tokens`, logprobs, response format, and tool
     translations.
3. Keep other providers strict:
   - OpenRouter rejects `provider_options.deepseek`,
   - OpenRouter `provider_options.openrouter` does not accept `user_id`,
   - Codex continues rejecting all `provider_options`,
   - these rejections must happen before credential resolution, not only before
     upstream HTTP.
4. Keep the request surface narrow:
   - top-level client `user_id` remains an unknown field,
   - top-level client `user` remains an unknown field,
   - do not map OpenAI `user` to DeepSeek `user_id`,
   - do not invent a router-owned user identity or derive one from local API
     tokens, credential labels, account IDs, or model strings.
5. Preserve privacy boundaries:
   - validation errors must use static field names and must not echo supplied
     `user_id` values,
   - do not persist `user_id`, `provider_options`, raw request bodies, or raw
     provider payloads,
   - user-id markers must not appear in SQLite metadata, TUI output, CLI output,
     local errors, or fake-upstream error output.
6. Extend smoke checks without permanent tests:
   - non-streaming DeepSeek requests with only `user_id` reach fake upstream as
     top-level `user_id`,
   - non-streaming DeepSeek requests combining `user_id`, `thinking`,
     `reasoning_effort`, `max_completion_tokens`, `response_format`,
     logprobs, and function tools preserve existing translations,
   - streaming DeepSeek requests with `user_id` reach fake upstream as
     top-level `user_id`,
   - a valid 512-byte ASCII `user_id` reaches fake upstream,
   - invalid `user_id` shapes fail before credential resolution and upstream
     HTTP, including empty string, `null`, non-string values, 513-byte values,
     non-ASCII values, and illegal ASCII characters such as space, `.`, `/`,
     and `@`,
   - OpenRouter and Codex reject unsupported `user_id` provider-option shapes
     before credential resolution and upstream HTTP,
   - no-eligible credential smokes prove invalid DeepSeek `user_id` shapes fail
     before credential lookup can select anything,
   - top-level `user_id` and top-level `user` are rejected before credential
     resolution,
   - privacy scans prove marker values do not leak from local errors, SQLite,
     TUI output, CLI output, fake-upstream error output, successful
     non-streaming response bodies, or raw streaming output.

## Out of Scope

- OpenAI `user` support.
- A generic router user identity policy.
- Automatic `user_id` generation from local tokens, provider credentials, TUI
  accounts, config, or request metadata.
- OpenRouter user, metadata, session, or privacy routing fields.
- Codex user identity fields.
- DeepSeek beta strict tool mode.
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
  DeepSeek-specific `provider_options.deepseek.user_id` semantics belong in
  `internal/provider`.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool definitions, tool arguments, tool
  results, full bearer tokens, full provider request IDs, full account IDs,
  balances, credits, or user identifiers.

## Proposed Package Changes

```text
internal/provider/
  http_chat.go   # validate and translate DeepSeek provider_options user_id
internal/app/
  app.go         # serve/manage smoke assertions
```

Provider semantics:

```text
DeepSeek:
  provider_options.deepseek.user_id -> forward as user_id
  top-level user_id                 -> reject
  top-level user                    -> reject

OpenRouter:
  provider_options.deepseek.user_id -> reject
  provider_options.openrouter.user_id -> reject
  top-level user_id                 -> reject
  top-level user                    -> reject

Codex:
  provider_options                  -> reject
  top-level user_id                 -> reject
  top-level user                    -> reject
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

- DeepSeek non-streaming `provider_options.deepseek.user_id` reaches fake
  upstream as top-level `user_id`,
- DeepSeek streaming `provider_options.deepseek.user_id` reaches fake upstream
  as top-level `user_id`,
- combined DeepSeek requests still translate reasoning options, token limits,
  response format, logprobs, and tools correctly,
- a valid 512-byte ASCII `user_id` reaches fake upstream,
- invalid `user_id` shapes fail before credential resolution and upstream HTTP,
  including empty string, `null`, non-string values, 513-byte values,
  non-ASCII values, and illegal ASCII characters such as space, `.`, `/`, and
  `@`,
- OpenRouter and Codex unsupported user-id provider-option shapes fail before
  credential resolution and upstream HTTP,
- no-eligible credential smokes prove invalid DeepSeek `user_id` shapes fail
  before credential lookup can select anything,
- top-level client `user_id` and `user` remain unsupported before credential
  resolution,
- user-id markers do not appear in local errors, SQLite metadata, TUI output,
  CLI output, fake-upstream error output, successful non-streaming response
  bodies, or raw streaming output.

`manage --check` should continue proving that TUI output is metadata-only and
does not expose provider option or user-id markers.

## Review Questions

1. Is `provider_options.deepseek.user_id` the right explicit location for this
   provider-specific isolation field?
2. Is the `[A-Za-z0-9_-]+` and 512-byte validation strict enough without
   silently rewriting caller values?
3. Do we need any additional privacy guard beyond requiring explicit caller
   opt-in through DeepSeek provider options?
4. Are the smoke checks strong enough to catch accidental OpenAI `user` mapping
   or persistence of user identifiers?

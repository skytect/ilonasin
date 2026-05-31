# Plan 034: OpenRouter User

## Goal

Accept OpenRouter `user` as a strict OpenRouter-only chat request field.

OpenRouter documents `user?: string` for chat completions as a stable
end-user identifier used for abuse detection and user tracking. DeepSeek
documents a separate `user_id` field with different constraints and isolation
semantics. This slice adds the OpenRouter field without mapping it to
DeepSeek, deriving it from local client tokens, or storing it in local
metadata.

## Architecture Inputs

- `AGENTS.md`
- all markdown files under `docs/**`
- current official OpenRouter docs for chat completion request parameters
- current official DeepSeek docs for `user_id`
- prior plans `001` through `033`

## Scope

1. Add strict request parsing for top-level `user`:
   - accepted value must be a JSON string,
   - empty strings are invalid,
   - values longer than 512 bytes are invalid,
   - `null`, booleans, numbers, arrays, and objects are invalid,
   - validation must happen before credential resolution and upstream HTTP,
   - validation errors must use static field names and must not echo values.
2. Allow and forward the field for OpenRouter only:
   - OpenRouter accepts `user`,
   - the field is forwarded unchanged to upstream OpenRouter,
   - forwarding must preserve existing tools, tool choice,
     `parallel_tool_calls`, `provider.require_parameters`, JSON schema,
     logprobs, logit bias, advanced sampling, and token-limit translations.
3. Keep other providers strict:
   - DeepSeek rejects top-level `user` before credential resolution,
   - Codex rejects top-level `user` before credential resolution,
   - top-level `user_id` remains unknown and rejected,
   - `provider_options.deepseek.user_id` remains the only DeepSeek user
     isolation field,
   - `provider_options.openrouter.user_id` remains unsupported.
4. Preserve privacy and metadata boundaries:
   - do not persist `user` values or raw request bodies,
   - do not include user marker values in local errors, SQLite metadata, TUI
     output, CLI output, fake-upstream error output, or success responses,
   - do not derive `user` from ilonasin client tokens, provider credentials,
     OAuth accounts, account IDs, or local usernames.
5. Extend smoke checks without permanent tests:
   - OpenRouter non-streaming requests with `user` reach fake upstream with the
     exact string value,
   - OpenRouter streaming requests with `user` reach fake upstream with the
     exact string value,
   - OpenRouter accepts a 512-byte `user` value and forwards it unchanged,
   - a 513-byte `user` value fails before credential resolution and upstream
     HTTP,
   - an OpenRouter combined request with `user`, tools, tool choice,
     `parallel_tool_calls`, `provider.require_parameters`, JSON schema,
     logprobs, logit bias, `max_completion_tokens`, and an exact advanced
     sampling numeric sentinel preserves every translation,
   - invalid `user` shapes fail before upstream HTTP,
   - DeepSeek and Codex valid-but-unsupported `user` requests fail before
     credential resolution and upstream HTTP,
   - no-eligible credential smokes prove invalid values and unsupported
     provider use fail before credential lookup can select anything,
   - privacy scans prove user marker values do not leak.

## Out of Scope

- OpenRouter `prediction`, `verbosity`, `metadata`, `session_id`,
  `service_tier`, `cache_control`, `models`, `route`, `plugins`, or broad
  provider routing controls.
- DeepSeek `user_id` changes.
- Mapping top-level `user` to DeepSeek `user_id`.
- Automatic user identifier generation.
- Hashing, salting, or otherwise transforming user identifiers.
- Persisting user identifiers.
- Codex user tracking.
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
  types.go       # top-level user parsing, string validation, upstream marshal
internal/provider/
  http_chat.go   # provider capability decision
internal/app/
  app.go         # serve/manage smoke assertions
```

Provider semantics:

```text
DeepSeek:
  user -> reject
  provider_options.deepseek.user_id -> validate and forward as user_id

OpenRouter:
  user -> forward
  provider_options.openrouter.user_id -> reject

Codex:
  user -> reject
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

- OpenRouter forwards `user` for non-streaming chat,
- OpenRouter forwards `user` for streaming chat,
- OpenRouter accepts and forwards a 512-byte `user` value,
- a 513-byte `user` value fails before credential resolution and upstream HTTP,
- combined OpenRouter requests still translate tools, tool choice,
  `parallel_tool_calls`, `provider.require_parameters`, JSON schema,
  logprobs, logit bias, token limits, and advanced sampling correctly,
- invalid raw values fail before upstream HTTP,
- DeepSeek and Codex unsupported valid values fail before credential resolution
  and upstream HTTP,
- top-level `user_id` remains rejected,
- marker values do not appear in local errors, SQLite metadata, TUI output,
  CLI output, fake-upstream error output, or success responses.

`manage --check` should continue proving that TUI output is metadata-only and
does not expose user, tool, or provider option markers.

## Review Questions

1. Is OpenRouter-only `user` the right next identity slice after DeepSeek
   `provider_options.deepseek.user_id`?
2. Is a 512-byte maximum a reasonable local guardrail for this unpersisted
   upstream field, given DeepSeek documents the same maximum for `user_id` and
   OpenRouter does not document a tighter limit?
3. Should `user` be accepted as any non-empty string, or should this slice also
   restrict characters to avoid accidental private identifiers?
4. Does this preserve the boundary between common OpenAI-compatible parsing and
   provider-specific capability decisions?

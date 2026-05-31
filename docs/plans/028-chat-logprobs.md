# Plan 028: Chat Logprobs

## Goal

Accept `logprobs` and `top_logprobs` for DeepSeek and OpenRouter chat
completions, while continuing to reject them for Codex.

DeepSeek and OpenRouter both document chat `logprobs` plus `top_logprobs`.
Slice 018 intentionally rejected these fields and removed model capability
advertising until local chat behavior supported them. This slice closes that
gap without adding tools, provider routing controls, or arbitrary passthrough.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `027`
- official DeepSeek chat completion docs checked on 2026-05-31
- official OpenRouter parameter docs checked on 2026-05-31

## Scope

1. Keep strict OpenAI-compatible parsing:
   - `logprobs`, when present, must be a JSON boolean,
   - `top_logprobs`, when present, must be a JSON integer from `0` through
     `20`,
   - `top_logprobs` requires `logprobs: true`,
   - `logprobs: false` may be accepted alone, but not with `top_logprobs`,
   - raw `json.RawMessage` validation must run before typed unmarshal so
     overflow-like values cannot fail through Go's decode path with raw values
     echoed in errors,
   - `null`, string, object, array, float, out-of-range, and overflow-like
     values fail before credential resolution and upstream HTTP.
2. Forward supported request fields:
   - DeepSeek accepts and forwards `logprobs` and `top_logprobs`,
   - OpenRouter accepts and forwards `logprobs` and `top_logprobs`,
   - Codex rejects both fields until Codex request semantics are separately
     designed,
   - no provider receives either field after local rejection.
3. Preserve streaming logprob data when returned:
   - `NormalizeStreamChunk` keeps supported top-level choice fields including
     `index`, `finish_reason`, `delta`, and `logprobs`,
   - `logprobs` may be `null` or an object,
   - accepted logprobs objects may contain only `content` and
     `reasoning_content`,
   - each `content` or `reasoning_content` value must be `null` or an array,
   - each array item must be an object with `token` as a string, `logprob` as a
     finite JSON number, optional `bytes` as `null` or an array of integers from
     `0` through `255`, and optional `top_logprobs` as an array,
   - each `top_logprobs` item must use the same token object shape without a
     nested `top_logprobs` field,
   - invalid logprobs shapes fail as invalid upstream responses,
   - logprob data is returned to the requesting client but is not persisted in
     SQLite metadata, TUI output, CLI output, or local errors.
4. Update capability metadata:
   - DeepSeek static model capabilities include `logprobs`,
   - OpenRouter model discovery maps supported parameters `logprobs` and
     `top_logprobs` to a `logprobs` capability flag,
   - Codex capabilities remain unchanged.
5. Preserve privacy and metadata boundaries:
   - do not persist `logprobs`, `top_logprobs`, token strings, byte arrays, or
     raw logprob objects,
   - do not include request values, returned token strings, raw provider
     payloads, or response bodies in local errors,
   - provider adapters may hold logprob data only long enough to forward
     upstream responses to the requesting client.
6. Extend smoke checks without permanent tests:
   - DeepSeek and OpenRouter non-streaming requests with `logprobs:true` reach
     fake upstream with exact fields,
   - DeepSeek and OpenRouter non-streaming requests with
     `logprobs:true, top_logprobs:20` reach fake upstream with exact fields,
   - DeepSeek and OpenRouter streaming requests preserve choice `logprobs` in
     normalized SSE chunks,
   - invalid upstream streaming `choice.logprobs` shapes fail for string,
     number, boolean, array, unknown object keys, malformed content arrays,
     malformed token objects, malformed byte arrays, and nested top-logprob
     objects,
   - `logprobs:false` alone is accepted and forwarded,
   - invalid values fail before upstream HTTP and before credential resolution,
   - valid-but-unsupported Codex requests fail before upstream HTTP and before
     credential resolution,
   - model cache capabilities advertise logprobs only where supported,
   - privacy scans prove request values and logprob marker strings do not
     appear in SQLite metadata, TUI output, CLI output, or local errors.

## Out of Scope

- Tool calling or tool result messages.
- OpenRouter `provider.require_parameters`.
- DeepSeek strict tool mode.
- `logit_bias`.
- Provider-specific routing fields such as `provider`, `models`, and `route`.
- Persisting logprob data or token-level telemetry.
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
- `internal/openai` owns raw request-shape validation and stream chunk
  normalization.
- Provider-specific support decisions belong in `internal/provider`.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens, full provider request IDs, full account IDs, balances, credits, or
  logprob/token details.

## Proposed Package Changes

```text
internal/openai/
  types.go       # raw validation, request marshal, stream logprobs normalize
internal/provider/
  http_chat.go   # provider-specific validation and capability flags
internal/app/
  app.go         # serve/manage smoke assertions
```

Provider semantics:

```text
DeepSeek:
  logprobs     -> forward
  top_logprobs -> forward when logprobs is true

OpenRouter:
  logprobs     -> forward
  top_logprobs -> forward when logprobs is true

Codex:
  logprobs     -> reject
  top_logprobs -> reject
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
- DeepSeek and OpenRouter accept and forward valid `logprobs` and
  `top_logprobs` for non-streaming and streaming chat,
- DeepSeek and OpenRouter normalized stream chunks preserve valid choice
  `logprobs`,
- Codex rejects both fields before credential resolution and upstream HTTP,
- invalid values fail before credential resolution and upstream HTTP,
- model cache capabilities advertise logprobs only for DeepSeek and
  OpenRouter-supported models,
- no logprob token marker, prompt, completion, raw request body, response body,
  provider payload, SSE chunk, bearer token, provider request ID, account ID,
  balance, or credit appears in SQLite metadata, TUI output, CLI output, or
  local errors.

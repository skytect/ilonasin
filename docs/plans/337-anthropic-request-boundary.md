# 337 Anthropic Request Boundary

## Context

`docs/ilonasin-architecture.md` expects strict local request parsing before
routing, with modular provider-specific request boundaries. Recent Anthropic
slices split affinity, content blocks, tools, options, messages, responses,
count-token DTOs, and Chat conversion out of `types.go`.

`internal/anthropic/types.go` still owns DTOs, error envelopes, top-level
request decode orchestration, and shared package-level parser helpers. The
top-level request decode orchestration is now the next cohesive boundary.

## Scope

1. Keep this slice limited to `internal/anthropic` and this plan.
   - Do not touch server, provider, storage, management, TUI, config, logging,
     routing, metadata, or app files.
2. Add `internal/anthropic/request.go`.
3. Move top-level Anthropic request decoding out of `types.go`:
   - `DecodeRequest`
   - `DecodeCountTokensRequest`
   - `decodeRequest`
4. Keep DTOs, error envelopes, and shared package-level parser helpers in
   `types.go`:
   - `Request`, `Message`, `ContentBlock`, `Tool`
   - `ErrorEnvelope`, `ErrorBody`, `CountTokensResponse`
   - `MaxOutputTokens`
   - `Error`, `ErrorForStatus`, `ErrorWithType`
   - `firstUnsupportedAnthropicField`
   - `blocksText`
   - `isJSONString`
   - `isJSONObject`
5. Preserve behavior exactly.
   - invalid JSON and multi-object body errors stay unchanged;
   - unsupported top-level fields remain deterministic;
   - Messages still require `max_tokens`;
   - Count Tokens still treats `max_tokens` as optional when absent and
     validates it when present;
   - all top-level field decoders still run in the same order and produce the
     same errors;
   - no prompts, request bodies, response bodies, tool payloads, provider
     payloads, or affinity keys are newly stored, logged, rendered, or exposed.
6. Do not add permanent tests or new abstractions.

## Out Of Scope

- Changing accepted Anthropic request fields.
- Changing Count Tokens behavior.
- Moving DTOs or error envelope helpers.
- Moving shared parser helpers used across content, tools, messages, affinity,
  and conversion.
- Server/provider/storage/management/TUI changes.

## Verification

Use a temporary focused Anthropic package test, then remove it before commit,
covering:

- invalid request JSON error;
- multiple JSON objects error;
- unsupported top-level field error is deterministic;
- `DecodeRequest` keeps malformed JSON, multi-object body, and missing
  `max_tokens` errors unchanged;
- error precedence remains unchanged when required-field, message, and later
  option errors are combined;
- `DecodeCountTokensRequest` keeps malformed JSON and multi-object body errors
  unchanged;
- `DecodeCountTokensRequest` keeps unsupported top-level field errors
  deterministic;
- `DecodeCountTokensRequest` accepts omitted `max_tokens`;
- `DecodeCountTokensRequest` validates present invalid `max_tokens`;
- representative valid `DecodeRequest` and `DecodeCountTokensRequest` calls
  still decode all top-level options:
  `system`, `stream`, `temperature`, `top_p`, `top_k`, `stop_sequences`,
  `tools`, `tool_choice`, `metadata`, `cache_control`, `thinking`,
  `context_management`, and `output_config`.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/anthropic
go test ./...
go vet ./...
```

The `find` output may include unrelated pre-existing permanent tests; this
slice must not add any.

Build `cmd/ilonasin`, start `ilonasin serve` with temporary `ILONASIN_HOME` and
`[server] bind = "127.0.0.1:0"`, check `/_ilonasin/manage/health` over the
management socket, run a short `ilonasin manage` TUI smoke, then clean up.

## Acceptance

- Anthropic top-level request decoding lives in a focused package file.
- `types.go` remains responsible for DTOs, error envelopes, and shared
  package-level parser helpers.
- Anthropic Messages validation, Count Tokens validation, conversion behavior,
  metadata privacy, and IO logging behavior are unchanged.

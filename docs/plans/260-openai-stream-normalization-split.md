# 260 OpenAI Stream Normalization Split

## Goal

Reduce `internal/openai/types.go` responsibility by moving OpenAI-compatible
stream chunk normalization into `internal/openai/stream.go` without changing
behavior.

The `internal/openai` package is the local OpenAI-compatible contract boundary.
After earlier splits, `types.go` still owns request DTOs, request decoding,
request validation, upstream request marshaling, safe model sanitization, stream
normalization, and low-level JSON helpers. Streaming normalization is a coherent
boundary because it is used by provider streaming transport to normalize
upstream SSE chunks and usage metadata.

## Scope

1. Add `internal/openai/stream.go`.
2. Move these declarations from `types.go` to `stream.go`:
   - `IsStreamError`
   - `NormalizedStreamChunk`
   - `NormalizeStreamChunk`
   - `streamUsageFromMap`
   - `normalizeStreamChoice`
   - `normalizeStreamToolCalls`
   - `normalizeStreamToolCall`
   - `normalizeStreamToolCallFunction`
   - `normalizeStreamLogprobs`
   - `validateStreamLogprobsObject`
   - `validateStreamLogprobToken`
   - `jsonNumberFinite`
   - `jsonNumberIntegerInRange`
   - `requiredString`
   - `optionalString`
   - `copyOptionalString`
   - `copyOptionalInt`
3. Keep shared helpers that are also used by non-stream response extraction in
   `types.go`:
   - `safeResolvedModelFromRaw`
   - `SafeResolvedModel`
   - `safeResolvedModelRune`
   - `firstPositive`
   - `positiveInt`
4. Keep request DTOs, request decoding, request validation, upstream request
   marshaling, `Usage`, `Error`, and general request validation helpers in
   `types.go`.
5. Preserve function names, exported API, JSON output shape, error strings,
   usage extraction, resolved-model sanitization, stream tool-call handling,
   logprobs validation, and output-token detection.
6. Do not change provider transport, server routes, metadata recording,
   storage, management, TUI, config, logging policy, schema, migrations, or
   tests.

## Boundaries

- No public behavior changes.
- No raw prompt, completion, request body, response body, SSE chunk, tool
  argument, tool result, bearer token, OAuth token, API key, or full account ID
  storage or rendering changes.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused OpenAI stream smoke, then remove it before commit:

- normalize a content chunk and assert output-token detection plus sanitized
  resolved model;
- normalize a reasoning-content chunk and assert output-token detection;
- normalize a final usage-only chunk and assert usage fields, reasoning tokens,
  cache-hit tokens, and cache-write tokens;
- normalize a tool-call delta chunk and assert tool-call fields are preserved;
- assert top-level stream error detection still works;
- assert provider-layer handling of top-level stream errors remains covered by
  the unchanged `openai.IsStreamError` API;
- assert unsafe model strings are rejected by `SafeResolvedModel`;
- assert non-stream chat response metadata still uses the same resolved-model
  sanitizer for unsafe upstream model values and still extracts cached and
  cache-write usage tokens;
- assert existing error strings for unsupported stream object, missing choices,
  empty choices without usage, invalid tool calls, and invalid logprobs are
  unchanged.
- assert `types.go` no longer contains the moved stream-specific declarations
  and provider/server files are unchanged by this slice.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify API/providers/usage/logs
   chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- OpenAI stream normalization code lives in `stream.go`.
- `types.go` no longer owns stream-specific normalization declarations; shared
  non-stream helpers remain in `types.go`.
- Provider streaming transport continues using the same `openai` API.
- JSON normalization behavior, usage extraction, model sanitization, and error
  messages are unchanged.
- Compile, vet, focused stream smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

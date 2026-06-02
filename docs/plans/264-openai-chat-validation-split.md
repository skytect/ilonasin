# 264 OpenAI Chat Validation Split

## Goal

Reduce `internal/openai/types.go` responsibility by moving OpenAI
Chat Completions request validation into a dedicated validation file without
changing behavior.

The `internal/openai` package is the local OpenAI-compatible contract boundary.
After response, stream, and upstream request marshaling splits, `types.go` still
owns DTOs, request decoding, request validation, safe resolved-model helpers,
and shared low-level JSON helpers. Validation is now the largest remaining
mixed responsibility.

## Scope

1. Add `internal/openai/chat_validation.go`.
2. Move `ChatCompletionRequest.Validate` and the OpenAI Chat Completions
   validation helper cluster from `types.go` to `chat_validation.go`:
   - `validateTopLevelKeys`
   - `validateRawPrediction`
   - `validateRawUser`
   - `validateRawServiceTier`
   - `validateRawSessionID`
   - `validateRawMetadata`
   - `validateRawParallelToolCalls`
   - `validateRawAdvancedSampling`
   - `validateRawLogprobs`
   - `validateRawLogitBias`
   - `validateRawTools`
   - `validateRawTool`
   - `validateRawToolChoice`
   - `parseRawLogprobs`
   - `parseRawTopLogprobs`
   - `validateLogitBiasTokenID`
   - `parseLogitBiasNumber`
   - `advancedSamplingSpec`
   - `advancedSamplingSpecs`
   - `advancedSamplingSpecFor`
   - `parseAdvancedSamplingNumber`
   - `validateRawPenalties`
   - `parsePenaltyNumber`
   - `validatePenaltyValue`
   - `validateAdvancedSamplingValues`
   - `validateLogprobsValues`
   - `validateLogitBiasValues`
   - `validateToolValues`
   - `validateRawStreamOptions`
   - `validateRawMessages`
   - `validateStop`
3. Keep shared low-level helpers in `types.go` for now because stream,
   response, content, and validation code all use them:
   - `isJSONString`
   - `rawJSONStringValue`
   - `requiredRawString`
   - `requireRawStringValue`
   - `isFunctionName`
   - `isJSONNull`
   - `parseJSONNumberToken`
   - `validateRawAssistantToolCalls`
   - `validateRawAssistantToolCall`
   - `firstPositive`
   - `positiveInt`
4. Keep `DecodeChatCompletion` in `types.go`, still calling the same validation
   functions.
5. Keep `ChatCompletionRequest`, `Message`, `ErrorEnvelope`, `ErrorBody`,
   `Usage`, `Error`, safe resolved-model helpers, stream normalization,
   response extraction, request marshaling, Responses conversion, provider
   adapters, server routes, metadata, storage, management, TUI, config, logging
   policy, schema, and migrations unchanged.

## Boundaries

- Behavior-preserving relocation only.
- Preserve accepted/rejected request shapes, validation order, and exact error
  strings.
- No request field additions or removals.
- No raw prompt, completion, request body, response body, SSE chunk, tool
  argument, tool result, bearer token, OAuth token, API key, request ID, or full
  account ID storage or rendering changes.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused OpenAI validation smoke, then remove it before commit:

- decode and validate a representative valid request containing messages,
  tools, tool choice, sampling controls, logprobs, logit bias, metadata,
  stream options, service tier, session ID, user, and prediction;
- assert existing error strings and representative ordering remain unchanged
  for unknown fields, invalid messages, invalid tools, invalid tool choice,
  invalid sampling, invalid logprobs, invalid logit bias, invalid metadata,
  invalid stream options, invalid service tier, and invalid token limits;
- assert `types.go` no longer contains the moved validation declarations;
- assert provider/server files are unchanged by this slice.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- OpenAI Chat Completions validation lives in `chat_validation.go`.
- `types.go` no longer owns request validation implementation beyond calling
  validation functions from `DecodeChatCompletion`.
- Public validation behavior, error strings, decode behavior, provider
  behavior, and server routes are unchanged.
- Compile, vet, focused validation smoke, serve smoke, manage smoke, senior
  plan review, and senior implementation review pass.

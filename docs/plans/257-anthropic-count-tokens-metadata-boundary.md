# 257 Anthropic Count Tokens Metadata Boundary

## Goal

Move Anthropic count-tokens metadata construction out of the route file and
into the request metadata helper boundary without changing behavior.

`internal/server/anthropic_count_tokens_route.go` currently handles route
orchestration and also owns count-token metadata construction, latency
normalization, and Anthropic image counting. Chat and Responses metadata are
already split into focused `request_metadata_*.go` helpers. Count-tokens should
match that structure.

## Scope

1. Add `internal/server/request_metadata_anthropic.go`.
2. Move these helpers out of `anthropic_count_tokens_route.go`:
   - `anthropicCountTokensMetadata`;
   - `countTokensLatencyMS`.
3. Move `countAnthropicImages` into `request_metadata_images.go`.
   - It must inspect only content block `Type` values, not source URLs, base64,
     or other raw image payload material.
4. Preserve exact metadata behavior:
   - endpoint remains `anthropic_count_tokens`;
   - `Stream` remains false;
   - provider type still uses `safeMetadataToken`;
   - message, tool, image, requested model, resolved model, max-output-token,
     status, error class, prompt token, total token, and latency fields remain
     equivalent;
   - latency remains at least 1 ms.
5. Preserve route behavior, IO logging, error handling, response shape, model
   resolution, and storage recording.

## Boundaries

- No public route, provider, Anthropic parser, storage, management, TUI,
  logging policy, config, schema, or DTO changes.
- No raw request body, image URL, prompt, completion, provider payload, or SSE
  chunk storage.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused in-package smoke, then remove it before commit:

- build an Anthropic count-tokens request with messages, tools, images, model,
  and max token fields;
- call `anthropicCountTokensMetadata` for success and error cases;
- assert all metadata fields match the pre-move semantics;
- assert image counting counts only Anthropic `image` content blocks;
- assert latency is never below 1 ms;
- assert no raw content strings are copied into metadata.

Run a tiny direct route smoke against an in-process server, then remove it
before commit:

- call `POST /v1/messages/count_tokens` with a local token and explicit model;
- assert the success response has `input_tokens`;
- assert one metadata row records endpoint `anthropic_count_tokens`, `Stream`
  false, and `PromptTokens == TotalTokens`.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify API/providers/usage/logs
   chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- Anthropic count-tokens metadata construction lives in the request metadata
  helper boundary.
- Anthropic image counting lives with other request image-counting helpers.
- The route file owns route orchestration only for this concern.
- Compile, vet, focused smoke, serve smoke, manage smoke, and implementation
  review pass.

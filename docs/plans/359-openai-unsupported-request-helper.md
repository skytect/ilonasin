# 359 OpenAI Unsupported Request Helper

## Context

`docs/ilonasin-architecture.md` separates OpenAI-compatible request parsing,
strict validation, route metadata recording, and response writing. The OpenAI
Chat and Responses routes already share helper boundaries for invalid models,
provider-not-configured failures, credential-unavailable failures, and provider
preflight failures.

One duplicated OpenAI route failure remains. Unsupported request handling
currently repeats the same response/log shape in:

- `internal/server/chat_route.go` after `ChatCompletionRequest.Validate`;
- `internal/server/responses_route.go` after `ResponsesRequest.ToChatCompletionRequest`;
- `internal/server/responses_route.go` after converted Chat request validation.

Each path records route-specific metadata, logs status `400` with class
`unsupported_request`, and writes an OpenAI-compatible error envelope with type
`invalid_request_error` and code `unsupported_request`.

## Goal

Centralize OpenAI-compatible unsupported-request response/log handling while
keeping route-owned metadata construction.

## Scope

1. Add a small OpenAI-style helper in the server route support area, for
   example `writeOpenAIUnsupportedRequest`.
   - Inputs: response writer, HTTP request, route event name, error message,
     and a callback that records route-specific metadata.
   - Behavior: record status `400` and class `unsupported_request`, log the
     route event with class `unsupported_request`, and write an OpenAI error
     envelope with type `invalid_request_error` and code
     `unsupported_request`.
2. Use the helper in:
   - Chat Completions request validation failure;
   - Responses request-to-chat conversion failure;
   - Responses converted Chat validation failure.
3. Preserve existing behavior exactly:
   - HTTP status remains `400`;
   - error type remains `invalid_request_error`;
   - error code/class remains `unsupported_request`;
   - error message remains `err.Error()`;
   - route log event names remain `chat_route` and `responses_route`;
   - metadata construction remains route-owned.
4. Keep adapter preflight validation failures using
   `writeOpenAIPreflightFailure`, because they carry the same class through the
   preflight result boundary.
5. Keep Anthropic routes separate because they use Anthropic-shaped response
   envelopes and different metadata builders.
6. Do not change request parsing, validation logic, provider adapters, storage,
   schema, management routes, TUI, config, IO logging policy, public routes, or
   metadata fields.

## Out Of Scope

- Changing any accepted or rejected request shape.
- Changing error messages.
- Changing adapter validation/preflight behavior.
- Combining Anthropic route error handling with OpenAI route helpers.
- Adding permanent tests.

## Implementation Steps

1. Add the helper near the existing OpenAI route helpers in
   `internal/server/route_preflight.go`.
2. Replace the three duplicated OpenAI unsupported-request blocks.
3. Review the diff for exact status, type, class, message, route event, and
   metadata callback behavior.
4. Verify with temporary focused route checks, then remove those checks before
   commit.

## Verification

Use temporary focused checks, then remove them before commit. They should assert:

- Chat validation unsupported-request path still records status `400` and
  class `unsupported_request`;
- Chat validation response body still has OpenAI type `invalid_request_error`,
  code `unsupported_request`, and the original validation message;
- Responses conversion unsupported-request path still records status `400` and
  class `unsupported_request`;
- Responses converted validation unsupported-request path still records status
  `400` and class `unsupported_request`;
- route logging still uses `chat_route` and `responses_route`;
- Anthropic route files are untouched by this slice.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- OpenAI unsupported-request handling has one shared route helper.
- Route-specific metadata construction remains route-owned.
- Chat and Responses public behavior is unchanged.
- Anthropic behavior is unchanged.
- Provider behavior, storage, management, TUI, config, IO logging, public route
  paths, and metadata fields are unchanged.

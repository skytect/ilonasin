# 360 OpenAI Invalid JSON Helper

## Context

`docs/ilonasin-architecture.md` separates OpenAI-compatible request parsing,
strict validation, route metadata recording, and response writing. Recent
slices centralized several OpenAI route failure helpers:

- invalid model;
- provider not configured;
- unsupported request;
- credential unavailable;
- provider preflight failure.

The OpenAI Chat and Responses routes still duplicate invalid JSON handling:

- log the route event with status `400` and class `invalid_json`;
- write an OpenAI-compatible error envelope with type `invalid_request_error`
  and code `invalid_json`;
- preserve the decoder or body-read error message.

Anthropic routes intentionally differ. They use Anthropic-shaped error
responses and map body-read failures to `413`.

## Goal

Centralize OpenAI-compatible invalid-JSON response/log handling while preserving
all public behavior.

## Scope

1. Add a small OpenAI-style helper in the server route support area, for
   example `writeOpenAIInvalidJSON`.
   - Inputs: response writer, HTTP request, route event name, and error
     message.
   - Behavior: log status `400` with class `invalid_json`, and write an
     OpenAI-compatible error envelope with type `invalid_request_error` and code
     `invalid_json`.
2. Use the helper in:
   - Chat Completions decode/body-read failure;
   - Responses decode/body-read failure.
3. Preserve existing behavior exactly:
   - HTTP status remains `400`;
   - error type remains `invalid_request_error`;
   - error code/class remains `invalid_json`;
   - error message remains `err.Error()`;
   - route log event names remain `chat_route` and `responses_route`;
   - these early invalid-JSON paths still do not record request metadata.
4. Keep Anthropic routes separate because their envelopes and body-read status
   behavior differ.
5. Do not change request parsing, body size limits, IO logging, validation
   logic, provider adapters, storage, schema, management routes, TUI, config,
   public routes, or metadata fields.

## Out Of Scope

- Changing malformed-body status codes.
- Recording metadata for invalid JSON.
- Combining Anthropic invalid-JSON handling with OpenAI helpers.
- Changing request body read or IO capture behavior.
- Adding permanent tests.

## Implementation Steps

1. Add the helper near the existing OpenAI route helpers in
   `internal/server/route_preflight.go`.
2. Replace the duplicated invalid-JSON blocks in `chat_route.go` and
   `responses_route.go`.
3. Review the diff for exact status, type, class, message, route event, and
   metadata behavior.
4. Verify with temporary focused route checks, then remove those checks before
   commit.

## Verification

Use temporary focused checks, then remove them before commit. They should assert:

- Chat invalid JSON response still has status `400`, OpenAI type
  `invalid_request_error`, code `invalid_json`, and the original decode error
  message;
- Responses invalid JSON response still has status `400`, OpenAI type
  `invalid_request_error`, code `invalid_json`, and the original decode error
  message;
- route logging still uses `chat_route` and `responses_route`;
- invalid JSON still does not record request metadata for Chat or Responses;
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

- OpenAI invalid-JSON handling has one shared route helper.
- Chat and Responses public behavior is unchanged.
- Anthropic invalid-JSON behavior is unchanged.
- Provider behavior, storage, management, TUI, config, IO logging, public route
  paths, and metadata fields are unchanged.

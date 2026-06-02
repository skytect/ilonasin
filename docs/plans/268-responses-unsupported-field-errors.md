# 268 Responses Unsupported Field Errors

## Goal

Make local `/responses` request validation identify unsupported top-level
fields by name.

`docs/ilonasin-architecture.md` requires the local API surface to be strict:
unsupported fields should return clear errors and should not be silently
forwarded or ignored. The current Responses decoder rejects unknown top-level
fields, but the error is generic: `request contains unsupported fields`.

## Current Evidence

- `internal/openai/responses.go` decodes local Responses requests through
  `DecodeResponses`.
- `rejectUnsupportedResponsesFields` already returns field-specific errors for
  known accepted-but-unimplemented fields such as `prompt_cache_key` and
  `client_metadata`.
- `validateResponsesTopLevelKeys` rejects other unsupported top-level fields
  with the generic error `request contains unsupported fields`.
- `internal/server/responses_route.go` forwards the decoder error into the
  local API error message with `type=invalid_request_error` and
  `code=invalid_json`.

## Scope

1. Change `validateResponsesTopLevelKeys` so every unsupported top-level
   Responses field returns a clear error naming the field, for example:
   `foo is unsupported`.
2. Keep `prompt_cache_key` and `client_metadata` rejection behavior unchanged.
   They should still be rejected even when present with `null`.
3. Keep the allowed Responses field set unchanged.
4. Keep valid Responses decoding behavior unchanged.
5. Keep error envelope type/code behavior unchanged unless existing route code
   already classifies it differently.
6. Keep chat completions, Anthropic routes, provider adapters, server
   execution, metadata, storage, management, TUI, config, and logging behavior
   unchanged.
7. Do not add permanent tests.

## Boundaries

- No new Responses feature support.
- No forwarding or storage of unsupported fields.
- No provider adapter changes.
- No route preflight refactor.
- No public architecture docs rewrite.
- No raw request bodies, response bodies, prompts, completions, tool arguments,
  tool results, bearer tokens, OAuth tokens, API keys, account IDs, or request
  IDs rendered or logged.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused decoder smoke, then remove it before commit:

- `unknown_field` present with a string is rejected and the error contains
  `unknown_field`;
- `unknown_field` present with `null` is rejected and the error contains
  `unknown_field`;
- `prompt_cache_key` present with `null` is still rejected and the error
  contains `prompt_cache_key`;
- `client_metadata` present with an object is still rejected and the error
  contains `client_metadata`;
- a minimal supported streaming Responses request still decodes.

Run a temporary focused route or handler smoke, then remove it before commit:

- exercise the Responses route error path with a verified token fixture or
  narrow handler setup;
- assert unsupported top-level fields return HTTP `400`;
- assert the JSON error message contains the rejected field name;
- assert the envelope keeps `type=invalid_request_error` and
  `code=invalid_json`.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least one provider instance.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- Unsupported local Responses top-level fields are rejected with field-specific
  messages.
- Existing supported and explicitly rejected Responses fields keep their
  behavior.
- Compile, vet, focused decoder smoke, HTTP smoke, serve smoke, manage smoke,
  senior plan review, and senior implementation review pass.

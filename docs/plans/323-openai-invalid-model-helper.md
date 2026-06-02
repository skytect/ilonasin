# 323 OpenAI Invalid Model Helper

## Context

`docs/ilonasin-architecture.md` separates model address resolution, request
metadata recording, HTTP response writing, and provider behavior.

The OpenAI-compatible Chat and Responses routes currently duplicate invalid
model handling after model address resolution fails:

- build route-specific early request metadata;
- record metadata;
- log the route event with status `400` and class `invalid_model`;
- write an OpenAI error response with type `invalid_request_error`, code
  `invalid_model`, and the resolver error message.

That duplication is another route-behavior drift point. Anthropic uses a
different model resolver and response envelope, so it should remain separate in
this slice.

## Plan

1. Add a small OpenAI-style invalid-model helper in the server route support
   area. It should accept:
   - response writer and request;
   - route event name;
   - error message;
   - a callback that records route-specific metadata.
2. Use the helper in:
   - `internal/server/chat_route.go`;
   - `internal/server/responses_route.go`.
3. Preserve existing behavior exactly:
   - status `400`;
   - OpenAI error type `invalid_request_error`;
   - error code `invalid_model`;
   - response message from the resolver error;
   - route log event names `chat_route` and `responses_route`;
   - route-owned metadata construction.
4. Keep Anthropic routes, invalid JSON handling, provider-not-configured
   handling, preflight handling, credential resolution, provider adapters,
   storage, config, management API, logging policy, and TUI behavior unchanged.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run a temporary focused route smoke, then remove it before commit. It should
exercise Chat and Responses invalid-model paths and assert:

- status, OpenAI error type, code, and message are unchanged;
- request metadata is recorded with status `400` and error class
  `invalid_model`;
- route logging still uses `chat_route` and `responses_route`;
- Anthropic route files are untouched.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temporary directory.

## Acceptance

- Chat and Responses invalid-model handling share one OpenAI-style helper.
- Route-specific metadata construction remains route-owned.
- Anthropic invalid-model behavior remains separate and unchanged.
- No public API behavior, storage schema, provider adapter behavior, config
  behavior, management API behavior, logging policy, or TUI behavior changes
  are introduced.

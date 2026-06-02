# 322 OpenAI Provider Not Configured Helper

## Context

`docs/ilonasin-architecture.md` separates model routing, metadata recording,
HTTP response writing, and provider behavior. The OpenAI-compatible Chat and
Responses routes currently duplicate provider-not-configured handling after
model address resolution succeeds but the provider instance is absent:

- build route-specific early request metadata;
- record metadata;
- log the route event with status `404` and class `provider_not_configured`;
- write an OpenAI error response with type `invalid_request_error`, code
  `provider_not_configured`, and message `provider instance is not configured`.

That duplicated route behavior is another drift point. Anthropic uses a
different response envelope and should remain separate in this slice.

## Plan

1. Add a small OpenAI-style provider-not-configured helper in the server route
   support area. It should accept:
   - response writer and request;
   - route event name;
   - a callback that records route-specific metadata.
2. Use the helper in:
   - `internal/server/chat_route.go`;
   - `internal/server/responses_route.go`.
3. Preserve existing behavior exactly:
   - status `404`;
   - OpenAI error type `invalid_request_error`;
   - error code `provider_not_configured`;
   - message `provider instance is not configured`;
   - route log event names `chat_route` and `responses_route`;
   - route-owned metadata construction.
4. Keep Anthropic routes, invalid model handling, preflight handling, credential
   resolution, provider adapters, storage, config, management API, logging
   policy, and TUI behavior unchanged.

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
exercise Chat and Responses provider-not-configured paths and assert:

- status, OpenAI error type, code, and message are unchanged;
- request metadata is recorded with status `404` and error class
  `provider_not_configured`;
- route logging still uses `chat_route` and `responses_route`;
- Anthropic route files are untouched.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temporary directory.

## Acceptance

- Chat and Responses provider-not-configured handling share one OpenAI-style
  helper.
- Route-specific metadata construction remains route-owned.
- Anthropic provider-not-configured behavior remains separate and unchanged.
- No public API behavior, storage schema, provider adapter behavior, config
  behavior, management API behavior, logging policy, or TUI behavior changes
  are introduced.

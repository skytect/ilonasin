# 321 OpenAI Credential Unavailable Helper

## Context

`docs/ilonasin-architecture.md` separates local API routing, upstream
credential resolution, request metadata recording, and response writing.

The OpenAI-compatible Chat and Responses routes currently duplicate the same
credential-unavailable failure handling after `resolveModelCredentials` fails:

- build route-specific request metadata;
- set HTTP status `401`;
- set error class `credential_unavailable`;
- record metadata;
- write an OpenAI error response with type `invalid_request_error` and code
  `credential_unavailable`.

That duplication is small, but it creates another route-behavior drift point.
Anthropic uses a different response envelope and currently logs this failure,
so it should remain separate in this slice.

## Plan

1. Add a small OpenAI-style credential-unavailable helper in the server route
   support area. It should accept:
   - response writer;
   - a callback that records route-specific metadata.
2. Use the helper in:
   - `internal/server/chat_route.go`;
   - `internal/server/responses_route.go`.
3. Preserve existing behavior exactly:
   - status `401`;
   - OpenAI error type `invalid_request_error`;
   - error code `credential_unavailable`;
   - message `no eligible upstream credential is available`;
   - no new route log for Chat or Responses credential failures;
   - route-owned metadata construction.
4. Keep Anthropic routes, credential resolution, provider adapters, storage,
   config, management API, logging policy, and TUI behavior unchanged.

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
exercise Chat and Responses credential-unavailable paths and assert:

- status, OpenAI error type, code, and message are unchanged;
- request metadata is recorded with status `401` and error class
  `credential_unavailable`;
- no new Chat/Responses route log is introduced for this path;
- Anthropic route files are untouched.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temporary directory.

## Acceptance

- Chat and Responses credential-unavailable handling share one OpenAI-style
  helper.
- Route-specific metadata construction remains route-owned.
- Anthropic credential-unavailable behavior remains separate and unchanged.
- No public API behavior, storage schema, provider adapter behavior, config
  behavior, management API behavior, logging policy, or TUI behavior changes
  are introduced.

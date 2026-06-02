# 320 OpenAI Route Preflight Helper

## Context

`docs/ilonasin-architecture.md` says the HTTP server/router stack should reject
unsupported features clearly and keep routing, metadata recording, provider
adapters, and response writing as separate concerns.

The OpenAI-compatible Chat and Responses routes currently repeat the same
preflight failure pattern:

- fill early request metadata with status and error class;
- record metadata;
- write the HTTP log event;
- write an OpenAI error response with the preflight message and error class.

That duplication makes preflight behavior easier to drift between Chat and
Responses. Anthropic uses a different response envelope and should remain
separate in this slice.

## Plan

1. Add a small server helper for OpenAI-style preflight failures that accepts:
   - response writer and request;
   - route event name;
   - `routePreflightResult`;
   - a callback that records the already route-specific metadata.
2. Use the helper in:
   - `internal/server/chat_route.go` for provider-adapter preflight failures
     and adapter request-validation preflight failures;
   - `internal/server/responses_route.go` for the same two preflight paths.
3. Keep unsupported request validation, invalid model handling, provider-not-
   configured handling, credential resolution errors, Anthropic routes,
   management routes, provider adapters, storage, config, and logging policy
   unchanged.
4. Review the diff for exact HTTP status, error type, error class, response
   body, log event, and metadata behavior preservation.

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
exercise Chat and Responses provider preflight failures and adapter request-
validation preflight failures, and assert:

- the OpenAI error response status, type, code, and message are unchanged;
- request metadata is still recorded with the same status and error class;
- route logging still uses `chat_route` and `responses_route`;
- Anthropic route files are untouched.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temporary directory.

## Acceptance

- Chat and Responses preflight failure handling share one OpenAI-style helper.
- Route-specific metadata construction remains route-owned.
- Anthropic behavior remains separate and unchanged.
- No public API behavior, storage schema, provider adapter behavior, config
  behavior, management API behavior, or TUI behavior changes are introduced.

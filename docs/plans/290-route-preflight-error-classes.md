# 290 Route Preflight Error Classes

## Context

`docs/ilonasin-architecture.md` describes provider adapters as a production
boundary that should reject unsupported provider behavior clearly. The live
generation routes now share provider preflight in `internal/server/route_preflight.go`,
but that helper still returns the implementation-era class
`provider_unimplemented` for:

- configured provider instances that cannot support the request capability;
- missing server adapter wiring;
- configured provider types with no chat adapter.

Plans 193, 227, and 244 intentionally preserved this class while removing stale
message text and centralizing preflight. That was a compatibility-preserving
step. The current architecture target is cleaner than that: API-facing error
classes should describe the request/provider boundary, not implementation
state.

## Goal

Replace live generation-route preflight error classes with explicit provider
boundary classes while preserving route response shapes, status codes, messages,
metadata timing, route ordering, credential resolution, and provider behavior.

## Scope

1. Touch only server route preflight live code and temporary route smoke files
   that are removed before commit.
2. Keep HTTP status `501` for provider capability and adapter availability
   preflight failures.
3. Keep client-facing messages unchanged:
   - unsupported capability: `provider does not support this request`;
   - unavailable adapter: `provider is not available for this request`.
4. Change only the error classes:
   - unsupported provider capability becomes `provider_unsupported`;
   - missing adapter registry or adapter lookup miss becomes
     `provider_unavailable`.
5. Keep adapter request validation failures as status `400`, class
   `unsupported_request`.
6. Preserve route-specific error envelopes:
   - Chat Completions and Responses use OpenAI-compatible `writeError`;
   - Anthropic Messages uses `writeAnthropicError`.
7. Preserve early metadata recording paths and log event names:
   - `chat_route`;
   - `responses_route`;
   - `anthropic_route`.
8. Preserve route ordering:
   - Responses still runs provider capability and adapter lookup before
     `ToChatCompletionRequest`;
   - Anthropic still translates and locally validates before provider preflight.
9. Do not change credential resolution, retry/fallback, provider adapters,
   model discovery, storage, management DTOs, TUI, config, IO logging, or public
   route paths.
10. Do not add permanent tests.

## Implementation

1. Add server-local constants for the two preflight classes, likely in
   `internal/server/provider_errors.go`:
   - `providerUnsupportedCapabilityClass`;
   - `providerUnavailableClass`.
2. Update `internal/server/route_preflight.go` to use those constants.
3. Leave all response-writing call sites as-is so existing route-specific
   envelopes consume the new `preflight.ErrorClass`.
4. Add a temporary in-package smoke under `internal/server`, run it, then remove
   it before commit. The smoke should cover:
   - unsupported provider capability;
   - nil adapters;
   - adapter lookup miss;
   - adapter validation failure;
   - a successful preflight path.
5. Add a route-level temporary smoke if needed to prove OpenAI and Anthropic
   envelopes carry the new class where applicable.

## Verification

Run a temporary focused smoke, then remove it before commit. It should assert:

- unsupported provider capability returns status `501`, class
  `provider_unsupported`, and the unsupported-capability message;
- nil adapters returns status `501`, class `provider_unavailable`, and the
  unavailable message;
- adapter lookup miss returns status `501`, class `provider_unavailable`, and
  the unavailable message;
- adapter validation errors still return status `400`, class
  `unsupported_request`, and the adapter error text;
- a successful preflight returns the adapter;
- Chat Completions and Responses error envelopes expose the new code in the
  OpenAI-compatible body;
- Anthropic Messages keeps its existing Anthropic-shaped error body, which is
  status/message based and does not expose the internal preflight class;
- Anthropic logs and early request metadata carry the new preflight class;
- early request metadata stores the new preflight classes.

Then run:

```sh
rg -n 'provider_unimplemented' internal/server
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

The `rg` command should find no live `internal/server` uses of
`provider_unimplemented` after the temporary smoke file is removed.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health and snapshot over the management socket.
4. Run `manage` under short timeouts at narrow and wide terminal sizes.
5. Verify API, providers, usage, and logs chrome renders.
6. Remove all temporary artifacts.

## Acceptance

- Live server code no longer reports `provider_unimplemented`.
- Provider preflight classes describe unsupported capability versus provider
  unavailability.
- Status codes, messages, response envelope types, route ordering, metadata
  recording, and logs stay otherwise unchanged.

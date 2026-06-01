# 227 Server Provider Unavailable Errors

## Goal

Normalize the remaining API-facing provider-unimplemented messages. The server
already has `providerUnsupportedCapabilityMessage` for configured providers
whose capabilities cannot serve a request, but missing adapter paths still return
`provider adapter is not implemented`. That phrase exposes implementation state
instead of a clean provider/request boundary.

## Current Evidence

- `internal/server/chat_route.go` has two adapter-missing branches returning
  `provider adapter is not implemented`.
- `internal/server/responses_route.go` has two adapter-missing branches returning
  `provider adapter is not implemented`.
- `internal/server/anthropic_route.go` has two adapter-missing branches returning
  `provider adapter is not implemented`.
- `internal/server/provider_errors.go` already defines
  `providerUnsupportedCapabilityMessage = "provider does not support this request"`.
- `docs/ilonasin-architecture.md` says unsupported fields and provider behavior
  should return clear errors, while provider quirks stay behind adapter
  boundaries.

## Implementation

1. Add a server-local message for configured provider types that cannot be
   served because no adapter exists:
   - `provider is not available for this request`
2. Replace user-facing `provider adapter is not implemented` responses in:
   - `internal/server/chat_route.go`
   - `internal/server/responses_route.go`
   - `internal/server/anthropic_route.go`
3. Preserve HTTP status `501`, error class/code `provider_unimplemented`,
   metadata recording, logging event names, adapter selection, and routing.
4. Do not touch retry/fallback logic in the currently dirty server files:
   - `internal/server/chat_nonstream.go`
   - `internal/server/chat_stream.go`
   - `internal/server/credentials.go`

## Verification

- Inspect the diff before running checks.
- Run `rg` to prove `provider adapter is not implemented` no longer appears in
  live `internal/server` code.
- Add a temporary in-package route smoke under `internal/server`, run it, and
  remove it before commit. The smoke should hit both no-adapter and
  adapter-not-found branches for:
  - `POST /v1/chat/completions`
  - `POST /v1/responses`
  - `POST /v1/messages`
- The temporary smoke must assert:
  - HTTP status remains `501`,
  - OpenAI-compatible responses keep `type=invalid_request_error` and
    `code=provider_unimplemented`,
  - Anthropic-compatible responses keep an Anthropic error envelope,
  - recorded metadata keeps `ErrorClass=provider_unimplemented` where the
    route records early metadata,
  - response bodies do not contain `provider adapter is not implemented`,
  - response bodies contain `provider is not available for this request`.
- Run `git diff --check`.
- Run `go test ./...`.
- Run `go vet ./...`.
- Build `cmd/ilonasin`.
- Start a temp daemon and smoke `ilonasin serve`.
- Smoke `ilonasin manage` against that daemon in a PTY.

## Boundaries

- Server route response language only.
- No provider adapter, credential, storage, management, config, TUI, metadata
  schema, or routing behavior changes.
- Do not add permanent tests.

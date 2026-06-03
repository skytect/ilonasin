# 409 Stream Error Exposure Policy

## Context

Whole-codebase senior reviews flagged
`internal/server/chat_stream.go` as still embedding provider-specific behavior:
`writeStreamingChatPreResponseError` takes a raw provider type string and
special-cases Codex when choosing the local error code returned before an SSE
stream starts.

`docs/ilonasin-architecture.md` says provider adapters own provider-specific
behavior, while the router/server should keep explicit routing policies rather
than hidden provider quirks.

## Goal

Replace the raw Codex branch in the streaming pre-response error writer with an
explicit server-local policy object, preserving exact response behavior.

## Scope

1. Add a small stream error exposure policy type in `internal/server`.
   - It should express whether a provider may expose any non-empty provider
     stream error class as the local error code before the stream starts.
   - It should keep the common public error classes exposed for every provider:
     `upstream_auth_failed`, `rate_limit_exceeded`, and `insufficient_quota`.
2. Add a helper that maps `provider.Instance` to the policy.
   - Preserve current behavior: Codex exposes any non-empty pre-stream
     `ErrorClass`; non-Codex providers expose only the common public classes.
   - Keep this as explicit routing policy, not buried inside the response
     writer.
3. Update `writeStreamingChatPreResponseError`.
   - Replace the `providerType string` parameter with the policy type.
   - Preserve local status normalization exactly:
     upstream statuses below 400 or 500+ become `502`, 4xx stays 4xx.
   - Preserve message/type/code output exactly for all current cases.
4. Do not change streaming execution, retry, quota, health, metadata,
   provider adapters, TUI, management, config, or storage.
5. No permanent tests.

## Verification

- Temporary focused checks must prove current behavior is preserved; remove
  them before commit.
- Include checks for:
  - no error with status below 400 and no sink start returns summary
    unchanged;
  - sink already started returns summary unchanged;
  - status normalization for 0/500 and 4xx statuses;
  - common public classes are exposed for non-Codex;
  - non-Codex non-public classes return `upstream_stream_error`;
  - Codex non-empty classes are exposed;
  - empty Codex class still returns `upstream_stream_error`.
- Run `git diff --check`.
- Run `find . -name '*_test.go' -type f -print`.
- Run `go test ./internal/server`.
- Run `go test ./...`.
- Run `go vet ./...`.
- Build `ilonasin`.
- Smoke `ilonasin serve` with an isolated `ILONASIN_HOME`.
- Smoke `ilonasin manage` against that daemon at narrow and wide terminal
  widths.

## Non-Goals

- Moving streaming error classification into provider adapters.
- Changing normalized SSE error payloads after a stream has started.
- Changing retry/fallback/quota behavior.
- Changing non-streaming error behavior.
- Changing the public error response envelope.

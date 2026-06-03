# 363 Refresh Failure Marker Coverage

## Context

`docs/ilonasin-architecture.md` allows OAuth refresh failure descriptions from
token endpoint error objects to be stored and rendered, but forbids full bearer
tokens, provider account IDs, provider request IDs, prompts, completions,
request bodies, response bodies, and raw provider payloads.

`privacy.RefreshFailureDescription` is the shared sanitizer used before
persistence and again before management/TUI exposure. Its current marker
coverage catches common token/body markers and some account/request markers,
but it is narrower than newer account-display and metadata safety filters for
separator variants such as `account.id`, `account:id`, `account/id`,
`account id`, `acct.`, `acct:`, `acct/`, `request.id`, `request:id`,
`request/id`, `req.`, `req:`, and `req/`.

## Goal

Broaden the shared refresh failure description sanitizer so account-id-shaped
and request-id-shaped refresh error descriptions redact consistently before
persistence and before management/TUI display.

## Scope

1. Update `internal/privacy/refresh_failure.go` marker coverage for:
   - account ID variants using `_`, `-`, `.`, `:`, `/`, or space separators;
   - account shorthand variants using `acct` plus `_`, `-`, `.`, `:`, or `/`;
   - request ID variants using `_`, `-`, `.`, `:`, `/`, or space separators;
   - request shorthand variants using `req` plus `_`, `-`, `.`, `:`, or `/`.
2. Preserve safe refresh-failure prose such as `invalid_grant`,
   `refresh token expired`, `authorization revoked`, and short human-readable
   upstream descriptions that do not contain forbidden markers.
3. Keep `RefreshFailureClass` behavior unchanged.
4. Do not change OAuth refresh flow, storage schema, management DTOs, TUI
   rendering, provider adapters, routing, or logging.

## Out Of Scope

- Adding permanent tests.
- Reworking all privacy regexes.
- Backfilling or rewriting existing SQLite rows.
- Changing snapshot display truncation.

## Implementation Steps

1. Broaden `unsafeRefreshFailureDescriptionPattern` in
   `internal/privacy/refresh_failure.go`.
2. Run temporary focused checks for allowed prose and the new account/request
   marker variants, then remove them before commit.
3. Review the diff for over-redaction of ordinary OAuth failure text.

## Verification

Use temporary focused checks, then remove them before commit. They must prove:

- `account.id`, `account:id`, `account/id`, `account id`, `acct.abc`,
  `acct:abc`, and `acct/abc` redact;
- `request.id`, `request:id`, `request/id`, `request id`, `req.abc`,
  `req:abc`, and `req/abc` redact;
- safe prose such as `invalid_grant`, `refresh token expired`, and
  `authorization revoked` remains visible.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/privacy
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide widths, and
cleaning up all temporary files and processes.

## Acceptance

- Refresh failure descriptions with account/request ID separator variants are
  redacted by the shared privacy helper.
- Ordinary safe OAuth failure prose is preserved.
- Persistence and management exposure continue to use the same shared helper.
- No permanent test files are added.

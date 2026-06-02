# 294 Refresh Failure Description Scrubber

## Goal

Prevent OAuth refresh failure descriptions from persisting or leaving the
management API with unsafe marker-shaped content.

The architecture allows storing OAuth token-endpoint error descriptions for
account visibility, but does not allow storing or exposing full bearer tokens,
provider request IDs, account IDs, raw payload/body markers, or similar sensitive
identifiers. Current refresh-failure description sanitizers normalize controls
and length only.

## Scope

1. Harden the credentials persistence boundary:
   - `recordOAuthRefreshFailure` must store only a marker-scrubbed refresh
     failure description.
   - If unsafe markers are present, replace the whole description with
     `[redacted]`; do not partially scrub substrings.
   - Preserve ordinary human-readable OAuth messages such as `invalid_grant`,
     `refresh token expired`, and `authorization revoked`.
2. Harden management response sanitization:
   - management snapshot and OAuth credential responses must redact stored unsafe
     descriptions even if legacy rows already contain unsafe text.
3. Cover unsafe marker classes including:
   - bearer/token/API-key markers such as `Bearer ...`, `sk-...`, `iln_...`,
   - raw/body/payload/prompt/completion markers,
   - account/request ID markers such as `acct_...`, `account_id`,
     `request_id`, `req_...`,
   - tool argument/result and SSE chunk markers,
   - JWT-like strings.
   - Treat token key/value markers such as `refresh_token=...`,
     `access_token`, and `Bearer ...` as unsafe, while preserving safe prose such
     as `refresh token expired`.
4. Do not change refresh failure class normalization, OAuth refresh behavior,
   SQLite schema, management DTO shape, TUI rendering, IO logging, or provider
   HTTP behavior.
5. Do not add permanent tests.

## Verification

1. Temporary focused smoke, removed before commit:
   - seed a refresh failure description through the credentials refresh-failure
     path and confirm SQLite stores `[redacted]` for unsafe marker text,
   - seed a legacy unsafe stored description directly and confirm the management
     snapshot and direct OAuth credential response conversion return
     `[redacted]`,
   - confirm ordinary safe descriptions remain readable and still trim controls
     and excess whitespace, including exact cases for `invalid_grant`,
     `refresh token expired`, and `authorization revoked`,
   - confirm unsafe cases for `refresh_token=...`, `access_token`, `Bearer ...`,
     `sk-...`, `iln_...`, `request_id`, `acct_...`, and JWT-like strings.
2. Source checks:
   - `rg -n 'func safeRefreshFailureDescription' internal/credentials internal/management`
     shows both boundaries keep explicit sanitizer functions.
   - `rg -n 'safeRefreshFailureDescription' internal/management` shows both the
     snapshot sanitizer and direct OAuth response conversion call the management
     sanitizer.
3. Standard checks:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
4. Daemon/manage smoke:
   - build a temporary `ilonasin`,
   - run `serve` with a temporary explicit config on a free port,
   - verify management health over the Unix socket,
   - run `manage --config "$tmp/config.toml"` under short wide and narrow PTY
     timeouts and confirm API, providers, usage, and logs render.

## Acceptance

- Unsafe refresh failure descriptions are scrubbed before persistence.
- Legacy unsafe refresh failure descriptions are scrubbed before management API
  exposure.
- Safe human-readable refresh descriptions remain useful.
- Compile, vet, serve, and manage smokes pass.

# 511 OAuth Device Log Privacy

## Context

Plan 510 found that OAuth device-login HTTP error handling can copy
provider-supplied `error_description`, `message`, or `detail` strings into
normal `provider_http` logs through `upstream_error_summary`. Those strings come
from an upstream response body and are outside the architecture's metadata-only
normal logging boundary.

`docs/ilonasin-architecture.md` forbids normal persistence of raw provider
payloads unless IO logging is enabled. `docs/plans/077-structured-application-logging.md`
limits provider HTTP logs to safe metadata such as endpoint label, method,
status, duration, response byte count, provider identifiers, error class, and
event ID.

## Goal

Remove upstream OAuth device-login response-body fields from normal provider
HTTP logs while preserving safe diagnostics and error classification.

## Scope

1. Update `internal/provider/oauth_device.go` so non-2xx OAuth device HTTP
   errors no longer append body-derived `upstream_error`,
   `upstream_error_kind`, or `upstream_error_summary` attributes to normal logs.
2. Preserve safe normal-log attributes: static endpoint label, method, provider
   instance ID, provider type, status, content type, duration, response byte
   count, error class, and response read error class.
3. Preserve response body size limiting and diagnostic read error handling.
4. Preserve OAuth device login behavior, returned `OAuthDeviceLoginError`
   classes, event IDs, management API behavior, storage, config, routing, TUI,
   provider auth, and IO logging behavior.
5. Remove helper code that becomes dead after the log privacy fix.
6. Do not add permanent tests.

## Out Of Scope

- Changing OAuth token refresh failure descriptions. Those have an explicit
  architecture carve-out.
- Changing OAuth device HTTP request paths, auth issuer validation, device-code
  parsing, or polling behavior.
- Adding new IO logging for OAuth device responses.
- Introducing new normal-log provider body summaries.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- OAuth device HTTP errors log status, response byte count, endpoint, and error
  class.
- OAuth device HTTP errors do not log upstream body-derived free-text fields
  such as `upstream_error`, `upstream_error_kind`, or
  `upstream_error_summary`.
- Returned `OAuthDeviceLoginError` class and event ID behavior remain intact.

Run:

```sh
rg -n 'oauthDeviceHTTPErrorAttrs|upstream_error|upstream_error_summary|safeOAuthLogValue|provider_http' internal/provider/oauth_device.go docs/plans/511-oauth-device-log-privacy.md
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/511-oauth-device-log-privacy.md
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, and keepalive disabled.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a pseudo-terminal.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- OAuth device-login HTTP error logs no longer contain provider response-body
  free text outside IO logging.
- Safe operational log metadata and returned error behavior are preserved.
- Dead body-summary helper code is removed.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Removed OAuth device HTTP error log enrichment from upstream response-body
  fields.
- Preserved endpoint label, method, provider instance ID, provider type, status,
  sanitized content type, duration, response byte count, error class, response
  read error class, and event ID.
- Removed the now-dead OAuth device HTTP error body parsing helpers.
- Preserved OAuth device login behavior, returned `OAuthDeviceLoginError`
  classes, event IDs, management API behavior, storage, config, routing, TUI,
  provider auth, and IO logging behavior.

## Verification Record

- Senior plan review: one reviewer reported no findings; two reviewers found
  that plain `git diff --check` would not cover the untracked plan file, so an
  explicit no-index check was added before implementation.
- Temporary focused harness: passed for OAuth device HTTP error log privacy,
  preserving safe log metadata and returned `OAuthDeviceLoginError` class plus
  event ID while excluding upstream body-derived fields and free text. Temporary
  harness was removed before commit.
- `rg -n 'oauthDeviceHTTPErrorAttrs|upstream_error|upstream_error_summary|safeOAuthLogValue|provider_http' internal/provider/oauth_device.go docs/plans/511-oauth-device-log-privacy.md`:
  passed. The removed body-derived helper names no longer appear in code, and
  remaining matches are provider HTTP calls, `safeOAuthLogValue`, and plan text.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty" docs/plans/511-oauth-device-log-privacy.md`:
  passed for the new untracked plan file before staging.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal.
- Senior implementation review: three reviewers reported no findings.
- Cleanup: temporary home, binary, config, harness, terminal captures, marker
  files, and daemon process were removed.

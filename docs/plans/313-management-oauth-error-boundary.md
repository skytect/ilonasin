# 313 Management OAuth Error Boundary

## Context

`docs/ilonasin-architecture.md` keeps provider adapters and daemon management as
separate boundaries. Recent slices removed provider registry and provider DTO
usage from management subscription usage paths, but management OAuth error
handling still imports provider error types:

- `internal/management/oauth.go` imports `internal/provider`;
- `safeOAuthErrorClass` inspects `provider.OAuthDeviceLoginError`;
- `safeOAuthErrorClass` inspects `provider.OAuthRefreshError`.

Credentials already owns the OAuth mutation interface used by management and
already inspects provider OAuth errors internally for logging and refresh
failure classification. Management should depend on credential-domain error
metadata, not provider adapter concrete errors.

## Plan

1. Add credential-domain helpers or interfaces that expose safe OAuth error
   metadata needed by management:
   - OAuth device-login class;
   - OAuth device-login event ID;
   - OAuth refresh class.
2. Keep provider error type inspection inside `internal/credentials`, where the
   provider adapters are already injected and credential services already know
   about provider OAuth errors.
3. Update `internal/management/oauth.go` to call credential-domain helpers and
   remove its `internal/provider` import.
4. Preserve current management error mapping:
   - credential not found -> `credential_not_found`, HTTP 404;
   - no eligible credential -> `oauth_login_expired`, HTTP 400;
   - unsupported credential -> `unsupported_credential`, HTTP 400;
   - invalid OAuth input -> `invalid_oauth_input`, HTTP 400;
   - OAuth refresh failed sentinel errors still map to `oauth_refresh_failed`,
     HTTP 502;
   - direct classified refresh errors, if surfaced to management without being
     wrapped as the sentinel, keep their sanitized refresh class as before;
   - provider login/refresh classes are sanitized through existing management
     sanitizer rules;
   - login event IDs are sanitized through existing event ID rules.
5. Do not change provider adapters, OAuth flows, storage, management routes,
   JSON shapes, TUI behavior, config, or local API behavior.
6. Review code before checks for lost event IDs, unsafe class propagation, and
   residual provider imports in `internal/management/oauth.go`.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/credentials
go test ./internal/management
go test ./...
go vet ./...
! rg -n '"ilonasin/internal/provider"|provider\.' internal/management/oauth.go
```

Run a temporary focused smoke, then remove it before commit. It must prove:

- a synthetic credential-domain device login error class and event ID become
  the same sanitized management class and event ID as before;
- unsafe login class or event ID values are redacted by management sanitizers;
- a synthetic credential-domain OAuth refresh class becomes the same sanitized
  management class as before;
- fallback sentinel errors still map to the same class/status pairs.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temporary directory.

## Acceptance

- `internal/management/oauth.go` no longer imports or refers to
  `internal/provider`.
- Management OAuth error responses preserve existing classes, event ID
  behavior, and HTTP status behavior.
- Provider-specific OAuth error inspection is isolated to the credentials
  domain.
- No route, storage, provider adapter, TUI, config, keepalive, or local API
  behavior changes are introduced.

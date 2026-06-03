# 382 Credentials Provider Boundary

## Context

`docs/ilonasin-architecture.md` keeps local API auth, upstream provider
credentials, provider adapters, routing, HTTP transport, TUI, config, and SQLite
storage as separate boundaries. Prior whole-codebase review found
`internal/credentials` importing `internal/provider`, which makes the credential
domain depend on provider adapter DTOs and concrete registry types.

The coupling is concentrated in `internal/credentials/upstream.go`:

- `UpstreamService.Registry` is `provider.Registry`;
- OAuth refresh and device-login collaborators use provider package
  interfaces and request/result DTOs;
- error-class helpers inspect provider error structs directly.

## Goal

Remove the `internal/credentials` dependency on `internal/provider` by defining
the small interfaces and DTOs the credential domain actually needs inside
`internal/credentials`, then adapt provider registry and OAuth adapters in
`internal/app`.

## Scope

1. Add credential-domain provider capability interfaces and DTOs in
   `internal/credentials`:
   - a provider instance lookup interface returning only ID, type, auth issuer,
     and credential capability flags needed by credential operations;
   - OAuth device-code request/challenge DTOs;
   - OAuth device-login request/result DTOs;
   - OAuth refresh request/result DTOs.
2. Change `UpstreamService` to depend on those credential-domain interfaces and
   DTOs instead of `internal/provider`.
3. Preserve existing credential behavior:
   - API-key providers must still require `APIKey`;
   - OAuth credential pooling and bearer lookup must still require Codex OAuth;
   - OAuth device login must still require Codex OAuth;
   - OAuth refresh must still require Codex OAuth refresh;
   - account hash inputs and labels must remain unchanged;
   - refresh failure class and description semantics must remain unchanged.
4. Add app-layer adapters from `provider.Registry`,
   `provider.OAuthTokenRefresher`, and `provider.OAuthDeviceLoginProvider` to
   the new credential interfaces.
5. Keep provider HTTP implementations, management DTOs, storage schema, routing,
   server request handling, keepalive, TUI, logging, IO capture, and config
   behavior unchanged.
6. Keep provider package error structs local to provider. Credentials should
   extract optional error metadata through local small interfaces, not concrete
   provider error types.
7. Preserve credential logging metadata currently derived from provider errors:
   - OAuth device-login errors with event IDs must keep those event IDs in
     credential logs;
   - OAuth device-login and refresh error classes must keep the same
     credential log `error_class` values.

## Out Of Scope

- No keepalive provider-boundary cleanup in this slice.
- No provider registry redesign outside the adapter needed for credentials.
- No public API, management route, JSON, SQLite, config, TUI layout, logging, or
  routing behavior changes.
- No new permanent tests.

## Verification

Run:

```sh
rg -n "\"ilonasin/internal/provider\"|provider\\." internal/credentials
rg -n "type ProviderInstanceLookup|type OAuthTokenRefresher|type OAuthDeviceLoginProvider" internal/credentials internal/app
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/credentials ./internal/app
go test ./...
go vet ./...
```

The first `rg` must produce no hits in `internal/credentials`.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

Also run a temporary focused compile-time or in-package smoke without keeping a
permanent test file proving:

- credential provider lookup accepts an API-key-capable provider for API keys;
- it rejects unsupported API-key, OAuth, OAuth-refresh, and missing-provider
  cases with the same credential-domain errors;
- provider OAuth refresh and device-login adapter errors still expose refresh
  classes, refresh descriptions, login error classes, and event IDs through the
  credential-domain helper interfaces.
- credential logging keeps provider-adapted OAuth device-login event IDs and
  refresh/login error classes unchanged.

Remove any temporary check before commit.

## Acceptance

- `internal/credentials` no longer imports or references `internal/provider`.
- Provider adapter details are adapted at the app boundary.
- Credential behavior, errors, refresh metadata, and OAuth/device-login flows
  are preserved.
- Credential log event IDs and error classes for provider-adapted OAuth errors
  are preserved.
- Compile/package checks, vet, direct serve/manage smokes, and the temporary
  focused boundary smoke pass.

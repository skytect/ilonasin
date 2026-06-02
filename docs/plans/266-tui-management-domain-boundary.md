# 266 TUI Management Domain Boundary

## Goal

Keep `ilonasin manage` as a client of the daemon-owned management API by
removing remaining TUI dependencies on provider and credential domain types.

The architecture says the TUI talks to the daemon-owned management API for
mutable operations, while the daemon performs SQLite/provider-domain work behind
that boundary. The TUI is mostly management-DTO based, but a few remaining paths
still leak lower-level domains into the control plane.

## Evidence

- `internal/tui/model.go` stores `*credentials.OAuthDeviceLoginChallenge`.
- `internal/tui/oauth_actions.go` converts management OAuth challenges back
  into `credentials.OAuthDeviceLoginChallenge` and interprets
  `provider.OAuthDeviceLoginError`.
- `internal/tui/helpers.go` interprets provider and credential domain errors.
- `internal/tui/provider_fallback_actions.go` imports `credentials` only for
  credential-kind constants.
- `internal/management/http_client.go` maps management HTTP errors back into
  `provider.OAuthDeviceLoginError`.

## Scope

1. Add a management-owned client error type, for example
   `management.ClientError`, carrying safe `Class`, `EventID`, and HTTP status.
2. Make `managementHTTPError` return that management-owned type for management
   error responses instead of returning provider-domain errors.
3. Keep management server-side error classification behavior unchanged.
   Server-side management may still translate domain errors into management
   response DTOs.
4. Store OAuth challenge state in TUI as `*management.OAuthDeviceLoginChallenge`.
5. Remove TUI conversion from management challenge DTOs to credential-domain
   challenge structs.
6. Update TUI error formatting/logging to understand `management.ClientError`
   and no longer import provider-domain errors.
7. Expose management-owned fallback visibility helpers or predicates so TUI
   fallback visibility no longer imports `internal/credentials` and no longer
   owns provider-kind eligibility policy.
8. Keep action routing, management routes, request/response JSON shape, OAuth
   flows, fallback mutation behavior, storage, provider adapters, server routes,
   config, logging policy, and TUI layout unchanged.

## Boundaries

- No provider adapter behavior changes.
- No management API path or JSON field changes.
- No storage, schema, config, or route changes.
- No direct SQLite or `config.toml` mutation from TUI.
- No raw API keys, OAuth tokens, bearer tokens, full account IDs, request IDs,
  prompts, completions, request bodies, response bodies, raw SSE chunks, tool
  arguments, or tool results rendered or logged.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused smoke, then remove it before commit:

- assert `internal/tui` no longer imports `internal/provider` or
  `internal/credentials`;
- assert `internal/management/http_client.go` no longer imports
  `internal/provider`;
- assert management HTTP error decoding returns `management.ClientError` with
  status, class, and event ID for an OAuth-style error response;
- assert management HTTP error decoding returns `management.ClientError` with
  status and class for a non-OAuth management error response;
- assert TUI OAuth error display includes safe class and event ID from
  `management.ClientError`;
- assert TUI OAuth challenge state uses the management DTO and still renders
  provider ID, verification URL, and user code;
- assert fallback visibility still includes API-key and OAuth fallback policies
  for providers with matching management DTO capabilities through management
  fallback visibility helpers;
- assert unsafe class/event/challenge values are sanitized by existing
  management/TUI display paths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- `internal/tui` imports only management-facing DTO/client packages for
  management state and no longer depends on `internal/provider` or
  `internal/credentials`.
- Management client errors are represented by a management-owned type.
- OAuth and fallback TUI behavior is unchanged from the user perspective.
- Compile, vet, focused smoke, serve smoke, manage smoke, senior plan review,
  and senior implementation review pass.

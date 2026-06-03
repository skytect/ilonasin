# 355 Refresh Failure Class Boundary

## Context

`docs/ilonasin-architecture.md` allows OAuth refresh failure classes and
descriptions as extracted metadata, while keeping raw token endpoint payloads,
tokens, account IDs, request IDs, prompts, completions, bodies, stream chunks,
tool arguments, and tool results out of normal metadata and display surfaces.

Plan 317 moved refresh-failure description sanitization into
`internal/privacy`. Refresh-failure class handling still has duplicated policy:

- `internal/credentials` normalizes classes before persistence;
- `internal/management` sanitizes classes before DTO exposure with a generic
  error-token helper;
- `internal/tui` keeps its own allowlist and falls back to generic display
  sanitization.

That means the class allowlist and fallback behavior are harder to audit than
the description path.

## Scope

1. Add a shared `privacy.RefreshFailureClass` helper.
   - It trims input.
   - It returns known safe refresh-failure classes unchanged.
   - It coerces unknown non-empty values to `refresh_unavailable`.
   - It returns empty for empty input so display boundaries can preserve
     absence.
   - It imports only the standard library.
2. Use the shared helper from `internal/credentials` for persistence and
   refresh-error wrapping.
   - Credentials must preserve the existing persistence behavior where an empty
     or unknown class normalizes to `refresh_unavailable`.
   - This can be done with a credentials-local wrapper around the shared helper.
3. Use the shared helper from `internal/management` for snapshot and direct
   OAuth response sanitization.
   - Management must preserve empty input as empty so absent refresh failures do
     not render as failures.
4. Use the shared helper from `internal/tui` for OAuth account display.
   - TUI must preserve empty input as empty so absent refresh failures remain
     visually absent.
5. Preserve the existing class vocabulary currently accepted by credentials and
   TUI:
   - `refresh_token_expired`
   - `refresh_token_invalidated`
   - `refresh_token_reused`
   - `refresh_invalid_grant`
   - `refresh_invalid_client`
   - `refresh_invalid_request`
   - `refresh_unauthorized_client`
   - `refresh_access_denied`
   - `refresh_unsupported_grant_type`
   - `refresh_invalid_scope`
   - `refresh_server_error`
   - `refresh_temporarily_unavailable`
   - `refresh_unauthorized`
   - `refresh_network_error`
   - `refresh_timeout`
   - `refresh_http_error`
   - `refresh_body_too_large`
   - `refresh_unavailable`
   - `refresh_invalid_response`
6. Do not change OAuth refresh behavior, storage schema, management JSON shape,
   TUI layout, provider adapters, request routing, logging, config, or
   refresh-failure descriptions.

## Out Of Scope

- Adding or removing refresh failure classes.
- Changing terminal refresh-failure resolution policy.
- Migrating stored legacy rows.
- Changing OAuth error descriptions or event IDs.
- Adding management or TUI surfaces.

## Implementation Steps

1. Add the shared helper beside `privacy.RefreshFailureDescription`.
2. Replace `credentials.normalizeRefreshFailureClass` internals with the shared
   helper, preserving the function as a local compatibility wrapper if useful.
3. Replace management and TUI `safeRefreshFailureClass` implementations with
   calls to the shared helper.
4. Review the diff for unchanged class vocabulary, empty-input behavior, and no
   new package dependency cycles.

## Verification

Use a temporary focused check, then remove it before commit:

- known classes remain unchanged through credentials, management, and TUI
  helpers;
- unknown non-empty classes become `refresh_unavailable`;
- empty classes still normalize to `refresh_unavailable` at the credentials
  persistence/error boundary;
- empty classes remain empty at management/TUI display boundaries;
- unsafe marker-shaped class strings do not render directly.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/privacy
go test ./internal/credentials
go test ./internal/management
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage`, and cleaning up all temporary
files and processes.

## Acceptance

- Refresh-failure class policy has one shared implementation in
  `internal/privacy`.
- Credentials, management, and TUI use the same class vocabulary and fallback
  behavior.
- Unknown or unsafe class values cannot leak through management or TUI display.
- No storage schema, API shape, provider, route, config, logging, or layout
  behavior changes are introduced.

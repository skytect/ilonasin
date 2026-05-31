# 083 Daemon Management OAuth

## Context

`docs/ilonasin-architecture.md` says `ilonasin manage` should be a TUI client
of a daemon-owned local management API, and that the daemon should own SQLite
mutation. Recent slices moved local-token mutations, read-only TUI snapshots,
upstream API-key mutations, and fallback-policy mutations behind that boundary.

Production `manage` still constructs a store-backed `credentials.UpstreamService`
for OAuth TUI actions:

- starting a Codex device OAuth login,
- completing a Codex device OAuth login,
- refreshing a selected OAuth credential.

Those actions can write OAuth tokens, provider account metadata, and refresh
failure state. They should therefore run through the daemon-owned management API
before adding more quota/account controls to the TUI.

## Goal

Move OAuth login and refresh mutations used by `ilonasin manage` through the
daemon-owned management API.

After this slice, production `manage` should no longer receive a direct
store-backed OAuth login or refresh controller for TUI OAuth actions. The TUI
should still read OAuth display state from the management snapshot, and the
daemon should perform all SQLite writes for login completion and refresh.

## Architecture Inputs

- `AGENTS.md`
- all Markdown files under `docs/**`
- especially `docs/ilonasin-architecture.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/plans/078-daemon-management-local-tokens.md`
- `docs/plans/079-daemon-management-snapshot.md`
- `docs/plans/080-codex-account-pooling.md`
- `docs/plans/081-quota-observability-foundation.md`
- `docs/plans/082-daemon-management-upstreams.md`

## Scope

1. Add management DTOs and interfaces for OAuth mutations.
   - Start OAuth device login.
   - Complete OAuth device login.
   - Refresh an OAuth credential by local credential ID.
   - Keep response payloads metadata-only except for device-login fields needed
     to continue the flow.
   - Provider instance ID, verification URL, and user code may be rendered by
     the TUI.
   - The opaque local login handle is a completion capability. It may be
     returned by the management response and held in transient TUI memory only;
     it must remain daemon-issued, TTL-bound, single-use, and not be rendered,
     logged, snapshotted, persisted, or copied into metadata.
2. Add local management HTTP routes under the internal management namespace.
   - Use bounded JSON decoding.
   - Keep routes off the public OpenAI-compatible API.
   - Return safe error statuses without embedding raw provider errors, token
     endpoint bodies, bearer tokens, account IDs, request IDs, balances, or
     credits.
3. Implement daemon service methods.
   - Delegate to the existing OAuth device login and refresh controller
     interfaces.
   - Preserve existing provider validation, device-login session behavior,
     token sanitization, account metadata sanitization, refresh failure
     tracking, and refresh error classification.
   - Do not add direct SQLite dependencies to management HTTP handlers.
4. Extend the Unix management client.
   - Add methods matching the new OAuth mutation interface.
   - Preserve OAuth login error class and safe event ID when available, because
     the TUI already surfaces these for debugging.
   - Add a structured safe error envelope for OAuth management failures, with
     normalized class and safe event ID only. The client should map that envelope
     back to the existing OAuth login error type where the TUI expects it.
   - Do not expose raw HTTP bodies or daemon internal errors.
   - Use an OAuth-completion transport path that does not inherit the generic
     short management timeout. Device login completion can legitimately poll for
     longer than ordinary management operations and must still honor context
     cancellation.
5. Update TUI wiring.
   - Replace direct OAuth metadata/login/refresh dependencies in production
     `Run` and `Check` with a narrow `management.OAuthClient`.
   - Keep snapshot-based reads as the production display source.
   - Keep direct fake/in-memory controllers only for isolated TUI exercise
     helpers that do not model production wiring.
6. Update app wiring.
   - Start the management service with OAuth login and refresh capability.
   - Pass the Unix management client to production `tui.Run` and production-like
     `tui.Check` for OAuth actions.
   - Stop constructing a direct TUI-facing `UpstreamService` in production
     `Manage` solely for OAuth mutations.
7. Extend smoke coverage.
   - `manage --check` should exercise OAuth device login and refresh through
     the local management client and daemon.
   - Include a pending/polling device-login case through the management client
     that takes longer than the generic short management timeout and still
     supports TUI cancellation.
   - Add source-level smoke guards proving production `app.Manage`,
     production `app.ManageCheck`, `tui.Run`, and `tui.Check` do not regain
     direct store-backed OAuth login or refresh controllers.
   - Add route-isolation checks proving new OAuth management routes are not
     available on the public OpenAI-compatible server.
   - Assert raw OAuth tokens, token endpoint body markers, raw provider payload
     markers, full account IDs, bearer tokens, request IDs, balances, and
     credits do not appear in management responses, snapshots, TUI output, logs,
     or metadata tables.

## Non-Goals

- Do not migrate telemetry pruning in this slice.
- Do not change serve-side OAuth credential resolution, 401 refresh, account
  pooling, fallback, quota tracking, request execution, or provider adapters.
- Do not add browser OAuth callback login in this slice.
- Do not add account deletion or token revocation.
- Do not change SQLite schema or migrations.
- Do not add permanent tests.
- Do not push.

## Design Constraints

1. The daemon remains the only production writer to SQLite for OAuth login
   completion and refresh.
2. The TUI remains a client of the daemon-owned local management API for OAuth
   mutations.
3. The management API may carry the local device-login handle only between
   start and complete calls. The handle must remain daemon-issued, TTL-bound,
   single-use, transient, and non-rendered.
   The management API may also carry safe user-facing device challenge fields,
   but not OAuth access tokens, refresh tokens, ID tokens,
   authorization codes, code verifiers, token endpoint bodies, raw provider
   payloads, bearer tokens, full account IDs, balances, credits, or full
   provider request IDs.
4. OAuth login error responses may preserve normalized error class and safe
   event ID only.
5. Management routes stay local-only on the management socket and do not appear
   on public `/v1/*` routes.
6. The TUI must not mutate `config.toml`.
7. Existing direct TUI exercise helpers may keep local fake controllers where
   that keeps targeted checks small, but production `Manage` and production-like
   `ManageCheck` should use the management client.

## Implementation Plan

1. Add management OAuth mutation API.
   - Add DTOs and a narrow `OAuthClient` interface.
   - Add an `OAuthMutationManager` interface around the existing credential
     service methods.
   - Add safe OAuth error DTO handling so HTTP responses can preserve only
     normalized class and safe event ID.
   - Extend `management.Service`, `HandlerService`, HTTP routes, and HTTP
     client methods.
2. Update daemon/app wiring.
   - Start the management service with an `UpstreamService` configured with the
     HTTP OAuth refresher and device-login provider.
   - Pass the Unix management client to production `tui.Run` and
     production-like `tui.Check` for OAuth operations.
   - Remove the direct OAuth `UpstreamService` construction from production
     `Manage`.
3. Update TUI wiring.
   - Store a management OAuth mutation client in the model.
   - Use it for `l` start/complete OAuth login and `r` refresh.
   - Keep snapshot reload after each successful mutation.
4. Update smoke exercises.
   - Route OAuth device-login and refresh lifecycle checks through the
     management HTTP client.
   - Add a long-polling device-login completion check through the management
     HTTP client that proves the OAuth completion request is not cut off by the
     ordinary management timeout and can be canceled.
   - Keep direct service checks only for credential-domain edge cases where
     those checks are not modeling production TUI wiring.
   - Add management response safety checks for OAuth login and refresh errors.
   - Assert device-login handles are not rendered, logged, snapshotted,
     persisted, or copied into metadata.
5. Add source and route guards.
   - Public `serve --check` must fail if OAuth management routes are reachable
     on the public API listener.
   - `manage --check` must fail if production TUI wiring regains broad direct
     OAuth refresh or device-login controller dependencies.
6. Cleanup.
   - Remove now-unused production TUI direct OAuth helpers.
   - Keep legacy direct dependencies only where clearly isolated smoke fakes
     still need them.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Acceptance:

- no permanent tests exist,
- compile/package, vet, build, `serve --check`, `manage --check`, and diff
  whitespace checks pass,
- production `manage` uses the Unix management client for OAuth login and
  refresh TUI actions,
- source-level smoke guards prove production `Manage`, production
  `ManageCheck`, `tui.Run`, and `tui.Check` do not retain direct store-backed
  OAuth login or refresh controllers,
- TUI display state still comes from the management snapshot,
- public OpenAI-compatible routes do not expose OAuth management endpoints,
- raw OAuth tokens and unsafe provider markers do not appear in management
  responses, TUI output, logs, metadata tables, or snapshots.

## Review Questions

1. Is OAuth login plus refresh the right next daemon-management slice, or should
   refresh move separately from device login?
2. Is preserving normalized OAuth error class and safe event ID in management
   client errors enough for debugging without leaking provider details?
3. Are direct fake controllers in targeted TUI exercises acceptable if
   production wiring and production-like checks are daemon-owned?

# 082 Daemon Management Upstreams

## Context

`docs/ilonasin-architecture.md` says the daemon owns SQLite mutation and
`ilonasin manage` is a client of a local daemon-owned management API. Recent
slices moved local token mutations and read-only TUI state through that
management boundary, but production `manage` still passes a store-backed
`credentials.UpstreamService` into the TUI for upstream API-key creation,
upstream credential disablement, and fallback-policy toggles.

Those direct TUI mutation paths are legacy. They should be moved behind the
local management API before adding more account, quota, or routing controls.

## Goal

Move upstream API-key credential mutations and fallback-policy mutations used by
`ilonasin manage` through the daemon-owned management API.

After this slice, production `manage` should no longer receive a direct
store-backed upstream credential manager for:

- adding an upstream API key,
- disabling an upstream credential,
- enabling a fallback group,
- disabling a fallback group.

The TUI should still read its display state from the management snapshot. The
daemon should perform all SQLite writes for these operations.

## Architecture Inputs

- `AGENTS.md`
- all Markdown files under `docs/**`
- especially `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/plans/078-daemon-management-local-tokens.md`
- `docs/plans/079-daemon-management-snapshot.md`
- `docs/plans/080-codex-account-pooling.md`
- `docs/plans/081-quota-observability-foundation.md`

## Scope

1. Add management DTOs and interfaces for upstream mutations.
   - Add request/response types for:
     - add upstream API key,
     - disable upstream credential,
     - enable fallback group,
     - disable fallback group.
   - Keep response payloads metadata-only and reuse safe credential/fallback
     metadata shapes where practical.
   - Do not expose secret material back to the TUI after API-key submission.
2. Add local management HTTP routes.
   - Add endpoints under the internal management namespace only.
   - Keep the OpenAI-compatible public API surface unchanged.
   - Use bounded JSON decoding.
   - Map validation and not-found errors to non-2xx HTTP statuses without
     embedding raw secrets or provider payload text.
3. Implement service methods on the management service.
   - Delegate mutations to the existing credential service interfaces.
   - Preserve existing provider/kind validation, fallback group validation,
     duplicate handling, and disabled-credential behavior.
   - Do not add direct SQLite dependencies to management HTTP handlers.
4. Extend the Unix management client.
   - Add client methods matching the new upstream mutation interface.
   - Return generic management-request errors, not raw daemon internals.
5. Wire production and check-mode TUI mutation paths through the management
   client.
   - Introduce a narrow TUI mutation interface instead of passing the full
     `credentials.UpstreamCredentialManager` for production.
   - Keep snapshot-based reads as the source of display state.
   - Keep direct fake/in-memory managers only for isolated TUI exercise helpers
     when they are not modeling production wiring.
   - Remove production `manage` construction of a store-backed
     `UpstreamService` solely for TUI upstream mutations.
6. Extend smoke coverage.
   - `manage --check` should exercise upstream add/disable and fallback
     enable/disable through the local management client and daemon.
   - Add negative source checks proving production `app.Manage`,
     production `app.ManageCheck`, `tui.Run`, and `tui.Check` are not wired to
     a store-backed `credentials.UpstreamService` or the broad
     `credentials.UpstreamCredentialManager` for the mutations in scope.
   - Add route-isolation checks proving new management routes are not available
     on the public OpenAI-compatible server.
   - Assert raw API keys, raw provider markers, bearer tokens, full account IDs,
     request bodies, response bodies, and provider payload markers do not appear
     in management responses, snapshots, TUI output, logs, or metadata tables.

## Non-Goals

- Do not migrate OAuth login or OAuth refresh mutations in this slice.
- Do not migrate telemetry pruning in this slice.
- Do not change provider routing, fallback eligibility, account pooling, quota
  tracking, or request execution behavior.
- Do not add new provider credential kinds.
- Do not add permanent tests.
- Do not push.

## Design Constraints

1. The daemon remains the only production writer to SQLite for the upstream and
   fallback operations in scope.
2. The TUI remains a client of the daemon-owned local management API for these
   mutations.
3. Management routes stay local-only on the management socket and do not appear
   on `GET /v1/models`, `POST /v1/chat/completions`, or any public server
   route.
4. The TUI must not mutate `config.toml`.
5. Management request and response types must not include raw provider payloads,
   raw SSE chunks, prompts, completions, tool data, full bearer tokens, full
   provider request IDs, full account IDs, balances, or credits.
6. API keys may be accepted in a management request body for insertion, but they
   must not be returned, logged, stored outside existing secret tables, rendered
   in the TUI, or copied into telemetry.
7. Existing direct TUI exercise helpers may keep local fake managers where that
   keeps checks small, but production `Manage` and production `ManageCheck`
   should use the management client for the operations in scope.

## Implementation Plan

1. Add management upstream mutation API.
   - Add DTOs and an `UpstreamCredentialClient` interface.
   - Extend `HandlerService` and `management.Service` methods.
   - Add HTTP routes and client methods.
2. Update app wiring.
   - Build the management service with a credential manager that can mutate
     upstream credentials and fallback policies.
   - Pass the Unix management client into production `tui.Run` and
     `tui.Check` for upstream mutations.
   - Stop constructing a direct TUI-facing `UpstreamService` in production
     `Manage`.
   - Add a small source-level smoke guard that fails if production `Manage`,
     production `ManageCheck`, `tui.Run`, or `tui.Check` regains the broad
     direct upstream mutation dependency.
3. Update TUI wiring.
   - Replace the TUI field that needs mutation with the narrow management
     upstream interface.
   - Adjust add, disable, enable fallback, and disable fallback methods to call
     the management client DTOs.
   - Keep snapshot reload after each mutation.
4. Update smoke checks.
   - Add management-route exercises for the new endpoints.
   - Make `manage --check` use the daemon client path for production-like
     upstream and fallback lifecycle checks.
   - Keep marker leak checks for secrets and raw payload markers.
5. Cleanup.
   - Remove now-unused production TUI direct upstream mutation wiring.
   - Keep legacy direct helpers only where they are clearly isolated check
     fakes.

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
- production `manage` uses the Unix management client for upstream API-key add,
  upstream credential disable, and fallback-policy toggles,
- source-level smoke guards prove production `Manage`, production
  `ManageCheck`, `tui.Run`, and `tui.Check` do not retain a direct
  store-backed `credentials.UpstreamService` or broad
  `credentials.UpstreamCredentialManager` mutation dependency for these
  operations,
- TUI display state still comes from the management snapshot,
- public OpenAI-compatible routes do not expose management mutation endpoints,
- raw API keys and unsafe provider markers do not appear in responses, TUI
  output, logs, metadata tables, or snapshots.

## Review Questions

1. Is grouping API-key upstream mutations and fallback-policy mutations in one
   slice narrow enough, or should fallback policy toggles be split out?
2. Are generic HTTP error responses sufficient for the local management API, or
   should the client receive safe normalized error classes now?
3. Does keeping isolated direct fake managers in TUI exercise helpers undermine
   the migration, or is it acceptable if production wiring is daemon-owned?

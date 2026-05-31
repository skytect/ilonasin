# 084 Daemon Management Pruning

## Context

`docs/ilonasin-architecture.md` says the daemon owns SQLite mutation and
`ilonasin manage` is a client of a daemon-owned local management API. Recent
slices moved local token mutations, read-only TUI snapshots, upstream
mutations, fallback-policy mutations, and OAuth login/refresh mutations through
that daemon boundary.

Production `manage` still passes `rt.Store` directly to the TUI as a telemetry
pruner. Pressing `p` in the TUI can therefore mutate SQLite without going
through the daemon management API.

That is now the last obvious production TUI mutation still using direct SQLite.

## Goal

Move telemetry pruning used by `ilonasin manage` through the daemon-owned
management API.

After this slice, production `manage` should no longer receive a direct
store-backed telemetry pruner. The TUI should request pruning through the Unix
management client, and the daemon should perform all SQLite writes for pruning.

## Architecture Inputs

- `AGENTS.md`
- all Markdown files under `docs/**`
- especially `docs/ilonasin-architecture.md`
- `docs/plans/009-telemetry-pruning.md`
- `docs/plans/078-daemon-management-local-tokens.md`
- `docs/plans/079-daemon-management-snapshot.md`
- `docs/plans/082-daemon-management-upstreams.md`
- `docs/plans/083-daemon-management-oauth.md`

## Scope

1. Add management DTOs and interfaces for telemetry pruning.
   - Request: cutoff timestamp.
   - Response: metadata-only prune result counts and cutoff.
   - Use explicit management DTOs, not broad storage structs in JSON.
2. Add a local management HTTP route under the internal management namespace.
   - Use bounded JSON decoding.
   - Keep the route off the public OpenAI-compatible API.
   - Return safe generic errors without raw SQLite errors, row contents,
     provider payload markers, request IDs, account IDs, balances, credits,
     prompts, completions, request bodies, response bodies, SSE chunks, tool
     arguments, or tool results.
3. Implement daemon service methods.
   - Delegate pruning to the existing metadata pruning interface.
   - Preserve the existing storage semantics from plan 009 plus the quota
     observability extensions added later:
     - strict `started_at < cutoff` and `occurred_at < cutoff`,
     - metadata tables only,
     - single transaction,
     - request, stream, fallback, health, and quota pruning counts,
     - returned counts only.
   - Do not add direct SQLite dependencies to management HTTP handlers.
4. Extend the Unix management client.
   - Add a pruning method matching the new management interface.
   - Return generic safe management errors only.
   - Use a pruning transport path that is appropriate for larger SQLite delete
     transactions rather than inheriting a short ordinary management timeout.
   - Preserve caller cancellation as `context.Canceled` or
     `context.DeadlineExceeded` where applicable. Cancellation must not be
     flattened into a generic daemon-unavailable error.
5. Update TUI wiring.
   - Replace production `TelemetryPruner` with a management pruning client.
   - Keep the TUI-owned 30-day manual cutoff policy unchanged.
   - Keep direct fake/in-memory pruners only for isolated TUI exercise helpers
     that do not model production wiring.
   - Keep snapshot-based reads as the production display source.
6. Update app wiring.
   - Build the management service with pruning capability.
   - Pass the Unix management client to production `tui.Run` and
     production-like `tui.Check` for pruning.
   - Stop passing `rt.Store` as the production TUI pruning dependency.
7. Extend smoke coverage.
   - `manage --check` should exercise telemetry pruning through the local
     management client and daemon.
   - Add source-level smoke guards proving production `app.Manage`,
     production `app.ManageCheck`, `tui.Run`, and `tui.Check` do not regain a
     direct store-backed telemetry pruner.
   - Add route-isolation checks proving the pruning management route is not
     available on the public OpenAI-compatible server.
   - Assert prune responses, TUI output, logs, snapshots, and metadata tables do
     not expose row contents or forbidden markers.
   - Keep plan-009 coverage that recent telemetry marker rows intentionally
     remain in telemetry tables after pruning, while protected tables and
     management outputs stay marker-free.

## Non-Goals

- Do not change pruning semantics from plan 009.
- Do not regress quota-event pruning or `PruneResult.Quotas`.
- Do not add scheduled pruning or configurable retention durations.
- Do not prune credentials, OAuth tokens, provider accounts, fallback policies,
  model cache, migrations, config, or logs.
- Do not change request routing, provider adapters, credential resolution,
  account pooling, quota recording, or observability snapshots.
- Do not change SQLite schema or migrations.
- Do not add permanent tests.
- Do not push.

## Design Constraints

1. The daemon remains the only production writer to SQLite for telemetry
   pruning.
2. The TUI remains a client of the daemon-owned local management API for
   pruning.
3. Management request and response types must not include raw row contents,
   model labels, credential labels, request IDs, provider request IDs, account
   IDs, prompts, completions, bodies, raw provider payloads, tool data, bearer
   tokens, balances, or credits.
4. The prune response may include only cutoff timestamp and counts.
   Counts must include requests, streams, fallbacks, health events, and quota
   observations.
5. Management routes stay local-only on the management socket and do not appear
   on public `/v1/*` routes.
6. The TUI must not mutate `config.toml`.
7. Recent telemetry rows that are newer than the cutoff may still contain
   seeded smoke-test markers in the isolated check database; that is intentional
   coverage that pruning did not over-delete. Protected tables, management
   responses, snapshots, logs, and TUI output must remain marker-free.
8. Existing direct TUI exercise helpers may keep local fake pruners where that
   keeps targeted checks small, but production `Manage` and production-like
   `ManageCheck` should use the management client.

## Implementation Plan

1. Add management pruning API.
   - Add `PruneTelemetryRequest`, `PruneTelemetryResponse`, and a narrow
     `TelemetryPruneClient` interface.
   - Add a daemon-side `TelemetryPruner` interface for the existing storage
     method.
   - Extend `management.Service`, `HandlerService`, HTTP routes, and HTTP
     client methods.
2. Update daemon/app wiring.
   - Start the management service with `store` as the daemon pruning backend.
   - Pass the Unix management client to production `tui.Run` and
     production-like `tui.Check` for pruning.
   - Remove `rt.Store` as a production TUI pruning dependency.
3. Update TUI wiring.
   - Keep the `p` key and 30-day cutoff behavior unchanged.
   - Call the management pruning client DTO instead of a direct storage-shaped
     interface.
   - Keep last-prune display as counts and cutoff only.
   - Preserve cancellation semantics from the management client.
4. Update smoke exercises.
   - Route the existing telemetry prune lifecycle check through a management
     HTTP client and daemon.
   - Keep the rich plan-009 storage invariants, quota-event pruning invariants,
     and forbidden-marker checks.
   - Add a cancellation/timeout smoke check for the management pruning client,
     proving canceled contexts surface as cancellation and do not leak raw
     storage/provider details.
   - Add direct management response safety checks for pruning errors.
5. Add source and route guards.
   - Public `serve --check` must fail if the pruning management route is
     reachable on the public API listener.
   - `manage --check` must fail if production TUI wiring regains a direct
     store-backed telemetry pruner.
6. Cleanup.
   - Remove now-unused production TUI direct pruning wiring.
   - Keep legacy direct dependencies only where isolated smoke fakes still need
     them.

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
- production `manage` uses the Unix management client for telemetry pruning,
- source-level smoke guards prove production `Manage`, production
  `ManageCheck`, `tui.Run`, and `tui.Check` do not retain direct store-backed
  telemetry pruning,
- TUI display state still comes from the management snapshot,
- public OpenAI-compatible routes do not expose the pruning management endpoint,
- prune results and error responses expose counts and cutoff only,
- quota pruning and `PruneResult.Quotas` are preserved through the management
  API, TUI display, and smoke checks,
- forbidden telemetry markers do not appear in management responses, TUI
  output, logs, snapshots, or protected tables, while recent telemetry marker
  rows remain when they are newer than the cutoff.

## Review Questions

1. Is telemetry pruning the right final production TUI mutation to move behind
   the daemon boundary?
2. Should the management API accept a caller-provided cutoff, or should the TUI
   send a fixed operation and let the daemon compute 30 days?
3. Are direct fake pruners in targeted TUI exercises acceptable if production
   wiring and production-like checks are daemon-owned?

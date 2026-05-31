# 086 TUI Snapshot Only Reads

## Context

`docs/ilonasin-architecture.md` says `ilonasin manage` is a client of the
daemon-owned management API and should not inspect SQLite directly. Plan 079
moved normal read-only TUI state loading to a daemon management snapshot, and
later slices moved production mutations through daemon management clients.

Production `app.Manage` now passes nil model-cache and observability readers to
`tui.Run` and `tui.Check`, so those direct read paths are not active in
production. However, the production TUI entrypoint signatures still expose
`ModelCacheReader` and `ObservabilityReader` slots, and `Model.reload` still
contains direct read fallback logic for model cache and observability.

Those slots preserve legacy architecture in the production TUI API even though
snapshot reads are now the intended boundary.

## Goal

Make production TUI read loading snapshot-only.

After this slice, `tui.Run` and `tui.Check` should not accept direct model-cache
or observability reader dependencies. The TUI should load read-only display
state from `management.SnapshotClient` only. Targeted smoke helpers may seed
SQLite and build a management snapshot through the daemon service boundary, but
they should not exercise direct TUI reads for model cache or observability.

## Architecture Inputs

- `AGENTS.md`
- all Markdown files under `docs/**`
- especially `docs/ilonasin-architecture.md`
- `docs/plans/079-daemon-management-snapshot.md`
- `docs/plans/081-quota-observability-foundation.md`
- `docs/plans/085-manage-client-bootstrap.md`

## Scope

1. Simplify production TUI entrypoints.
   - Remove `ModelCacheReader` and `ObservabilityReader` parameters from
     `tui.Run`.
   - Remove those parameters from `tui.Check`.
   - Keep `management.SnapshotClient` required.
2. Remove direct TUI read fallback for model cache and observability.
   - Delete TUI fields that hold model-cache and observability readers.
   - Delete the direct model-cache and observability branches from
     `reloadDirect`.
   - Keep direct reload only where targeted mutation smoke helpers still need
     direct token/upstream/OAuth metadata readers until those helpers are
     migrated or removed.
3. Convert model-cache and observability TUI smoke exercises.
   - Seed SQLite as before in app smoke helpers.
   - Build `management.Service.LoadManagementSnapshot` from the same store and
     registry to exercise the daemon-side snapshot mapping.
   - Feed the resulting snapshot into `tui.Check`.
   - Preserve current no-leak assertions and view assertions.
4. Update production wiring and smoke guards.
   - Update `app.Manage`, `app.ManageCheck`, and snapshot-related smoke helpers
     for the narrower TUI signatures.
   - Make `manage --check` fail if `tui.Run` or `tui.Check` reintroduces
     `ModelCacheReader` or `ObservabilityReader` parameters.
   - Make `manage --check` fail if `Model`, `NewModel`, `newCheckModel`, or
     `reloadDirect` reintroduce model-cache or observability reader fields,
     parameters, or direct reload branches.
   - Make `manage --check` fail if app model-cache or observability smoke
     helpers call TUI summary helpers with a direct store reader instead of a
     management snapshot.
   - Make `manage --check` fail if telemetry prune TUI exercises accept or are
     called with a direct observability/store reader for display rows.
   - Keep existing guards that production `tui.Run` and `tui.Check` receive the
     management client for mutation-capable slots.
5. Keep daemon snapshot contracts unchanged.

## Non-Goals

- Do not remove direct TUI helper paths for upstream, fallback policy, OAuth, or
  token mutation exercises in this slice.
- Do not change management snapshot DTOs, HTTP routes, storage schema,
  migrations, provider adapters, request routing, or TUI rendering text.
- Do not change quota recording, pruning semantics, fallback policy behavior,
  OAuth behavior, or account pooling behavior.
- Do not add permanent tests.
- Do not push.

## Design Constraints

1. Production read-only TUI state must come from `management.SnapshotClient`.
2. `tui.Run` and `tui.Check` must fail if no snapshot client is provided.
3. Snapshot failures must not fall back to direct readers.
4. App smoke helpers may still open SQLite to seed and verify isolated check
   data, but TUI view rendering checks for model cache and observability should
   consume management snapshots rather than direct storage readers.
5. No prompts, completions, request bodies, response bodies, raw provider
   payloads, raw SSE chunks, tool arguments, tool results, bearer tokens,
   provider request IDs, full account IDs, balances, or credits may appear in
   snapshots, TUI output, logs, or errors.
6. Source guards should cover both production app calls and TUI entrypoint
   signatures so legacy direct read slots cannot quietly return.

## Implementation Plan

1. Update TUI signatures and model fields.
   - Remove direct model-cache and observability reader fields from `Model`.
   - Remove model-cache and observability parameters from `NewModel`,
     `newCheckModel`, `Run`, and `Check`.
   - Remove direct model-cache and observability reload branches.
2. Update TUI exercises.
   - Change `ExerciseModelCacheSummary` and `ExerciseObservabilitySummary` to
     take a `management.SnapshotClient` or snapshot response.
   - Keep the existing view and leak checks.
   - Keep telemetry prune display exercising snapshot-loaded observability rows
     plus the management prune client.
   - Change `ExerciseTelemetryPrune` so display rows come from a snapshot
     client or snapshot response, not an `ObservabilityReader`.
3. Update app smoke helpers.
   - Use `management.Service{Registry: registry, ModelCache: store}` or
     `management.Service{Registry: registry, Observability: store}` to build
     snapshots from seeded stores.
   - Use the existing in-package snapshot check client for `tui.Check`.
4. Update app command wiring and guards.
   - Remove nil placeholders for model-cache and observability from all
     `tui.Run` and `tui.Check` call sites.
   - Extend source guards to reject `ModelCacheReader` and
     `ObservabilityReader` in production TUI entrypoint signatures.
   - Extend guards to reject `modelCache` and `observability` fields in
     `Model`, direct-reader parameters in TUI constructors, direct
     model-cache/observability reload calls, and direct store arguments passed
     from app smoke helpers into TUI model-cache or observability exercises.
   - Extend guards to reject direct observability/store arguments passed into
     `tui.ExerciseTelemetryPrune`.
5. Cleanup.
   - Remove now-unused helper types such as inert model-cache or observability
     fakes if they become dead.
   - Run `gofmt`.
   - Read through the diff before smoke checks.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
```

`go test ./...` is a compile/package check only. No permanent test files will be
added.

## Review Questions

1. Is removing direct model-cache and observability readers from TUI production
   entrypoints the right next architecture step?
2. Should model-cache and observability smoke helpers use a service-built
   management snapshot instead of direct TUI readers?
3. Are source guards sufficient to prevent the legacy direct read slots from
   returning to `tui.Run` and `tui.Check`?

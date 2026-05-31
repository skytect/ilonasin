# 089 TUI Management Observability Rows

## Context

`docs/ilonasin-architecture.md` says `ilonasin manage` should be a client of the
daemon-owned management API. Plans 079, 086, and 087 made TUI reads
snapshot-only. Plan 088 changed credential, fallback, OAuth, and provider-account
view rows to use management DTOs directly.

The TUI still converts other management snapshot rows back into provider and
metadata domain structs before rendering:

- `[]provider.ModelMetadata`
- `[]metadata.RequestSummary`
- `[]metadata.UsageSummary`
- `[]metadata.LatencySummary`
- `[]metadata.StreamSummary`
- `[]metadata.HealthSummary`
- `[]metadata.FallbackSummary`
- `[]metadata.QuotaSummary`
- `*metadata.PruneResult`

That preserves storage/domain row coupling inside `internal/tui` even though the
daemon management snapshot already exposes explicit allowlisted DTOs for these
views.

## Goal

Make TUI model-cache, observability, quota, fallback summary, and prune result
state use `internal/management` DTOs directly.

After this slice, `internal/tui` should not import `internal/metadata`, and
snapshot rows for display state should not be converted back into metadata or
provider model-cache structs inside `internal/tui`.

## Architecture Inputs

- `AGENTS.md`
- all Markdown files under `docs/**`
- especially `docs/ilonasin-architecture.md`
- `docs/plans/079-daemon-management-snapshot.md`
- `docs/plans/081-quota-observability-foundation.md`
- `docs/plans/086-tui-snapshot-only-reads.md`
- `docs/plans/087-tui-reload-snapshot-only.md`
- `docs/plans/088-tui-management-view-rows.md`

## Scope

1. Change TUI view row fields to management DTOs.
   - `Model.modelRows` becomes `[]management.ModelMetadata`.
   - `Model.requestRows` becomes `[]management.RequestSummary`.
   - `Model.usageRows` becomes `[]management.UsageSummary`.
   - `Model.latencyRows` becomes `[]management.LatencySummary`.
   - `Model.streamRows` becomes `[]management.StreamSummary`.
   - `Model.healthRows` becomes `[]management.HealthSummary`.
   - `Model.fallbackRows` becomes `[]management.FallbackSummary`.
   - `Model.quotaRows` becomes `[]management.QuotaSummary`.
   - `Model.pruneResult` becomes `*management.PruneResult`.
2. Apply snapshot rows directly.
   - Remove `modelMetadataFromSnapshot`.
   - Remove `requestSummariesFromSnapshot`.
   - Remove `usageSummariesFromSnapshot`.
   - Remove `latencySummariesFromSnapshot`.
   - Remove `streamSummariesFromSnapshot`.
   - Remove `healthSummariesFromSnapshot`.
   - Remove `fallbackSummariesFromSnapshot`.
   - Remove `quotaSummariesFromSnapshot`.
   - Use defensive copies when assigning snapshot slices to model state, so the
     TUI treats management API input as immutable.
3. Remove TUI metadata-domain prune adapters.
   - Delete the local `TelemetryPruner` interface.
   - Delete `directTelemetryPruner`.
   - Make the failure-only prune fake implement `management.TelemetryPruneClient`
     directly.
   - Make `ExerciseTelemetryPrune` compare a `management.PruneResult`.
4. Update TUI helpers to use management DTO types.
   - `requestModelDisplay`.
   - `modelCacheSummaries`.
   - observability rendering loops.
   - prune result rendering and logging.
5. Extend `manage --check` source guards so `internal/tui` cannot reintroduce
   metadata/provider model-cache view row types, the deleted conversion helpers,
   the local telemetry prune adapter, or an `internal/metadata` import.
6. Keep current user-visible rendering and behavior unchanged.
7. Do not add permanent tests.
8. Do not push.

## Non-Goals

- Do not change management snapshot JSON DTOs, daemon routes, HTTP clients,
  storage interfaces, storage schema, or snapshot sanitization.
- Do not convert provider instance rows away from `provider.Instance` in this
  slice because the TUI still uses `provider.Registry` and provider feature
  flags for actions.
- Do not move credential constants or sentinel errors in this slice.
- Do not change rendering text, quota recording, pruning semantics, fallback
  behavior, OAuth behavior, account pooling, migrations, or provider adapters.

## Design Constraints

1. TUI display state for model-cache and observability sections must be
   management API row types.
2. The TUI must not store or render raw provider payloads, bearer tokens, OAuth
   tokens, full account IDs, request bodies, response bodies, prompts,
   completions, SSE chunks, tool arguments, tool results, balances, or credits.
3. Management snapshot sanitization remains the daemon-side boundary. TUI
   rendering must keep its defensive display redaction.
4. App smoke helpers may keep using metadata/domain services for seeding and
   independent storage assertions, but not for TUI view rows.
5. The TUI should not define local adapters whose only purpose is to translate
   metadata-domain prune results into management responses.

## Implementation Plan

1. Update TUI row field types and snapshot application.
   - Assign model-cache and observability snapshot rows with `append([]T(nil),
     rows...)` defensive copies.
   - Delete the eight DTO-to-domain conversion helpers.
2. Update rendering helpers and prune logic.
   - Change helper signatures to management DTO types.
   - Store `resp.Result` directly in `m.pruneResult`.
   - Update the prune failure fake and `ExerciseTelemetryPrune` expected value
     to use `management.PruneResult`.
   - Remove `internal/metadata` from `internal/tui` imports.
3. Update app smoke helpers.
   - Change the telemetry prune expected value passed to
     `tui.ExerciseTelemetryPrune` to `management.PruneResult`.
   - Keep app-side storage fakes unchanged where they implement management
     service dependencies.
4. Add source guards.
   - Extend the positive AST guard for TUI `Model` fields to cover the
     management model-cache, observability, quota, fallback, and prune result
     field types.
   - Reject `internal/metadata` import and the old conversion helper names in
     `internal/tui/tui.go`.
   - Reject `provider.ModelMetadata` in `internal/tui/tui.go` while still
     allowing `provider.Instance` and `provider.Registry` for configured
     provider action semantics.
   - Reject `TelemetryPruner`, `directTelemetryPruner`, and
     `PruneTelemetryBefore` in `internal/tui/tui.go`.
5. Cleanup.
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

1. Is removing `internal/metadata` from `internal/tui` the right next
   architecture step after plan 088?
2. Should provider instance rows remain as `provider.Instance` for now, given
   TUI actions still depend on provider registry semantics?
3. Are the proposed guards enough to prevent metadata/provider model-cache DTO
   conversion from returning to TUI view state?

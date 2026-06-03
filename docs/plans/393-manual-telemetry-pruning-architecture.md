# 393 Manual Telemetry Pruning Architecture

## Context

`docs/ilonasin-architecture.md` still lists metadata pruning as an open question:
manual, scheduled, or both. The current code and earlier plans have already
settled a coherent production boundary:

- metadata retention defaults to keep forever until pruned;
- pruning is manual through the daemon-owned management API and TUI;
- the TUI sends a 30-day cutoff;
- SQLite pruning deletes metadata-only request, stream, fallback, health, and
  quota rows in one transaction;
- scheduled pruning and configurable retention durations are repeatedly out of
  scope in the prior pruning plans.

Leaving scheduled pruning as an open policy question makes the architecture less
accurate than the current implementation.

## Goal

Update `docs/ilonasin-architecture.md` so telemetry pruning is an explicit
manual-retention decision, not an unresolved open question.

## Scope

1. In the observability and logging section, replace "scheduled pruning remains a
   policy question" with the current decision:
   - default keep forever until pruned;
   - manual daemon-owned management API and TUI pruning;
   - current TUI cutoff is 30 days;
   - scheduled pruning is not part of the current architecture and would require
     a separate retention-policy design.
2. Remove "Should metadata pruning be manual, scheduled, or both?" from Open
   Questions.
3. Keep the Deferred Research list unchanged unless the architecture wording
   requires a narrow consistency edit.

## Out Of Scope

- Runtime behavior changes.
- TUI, management API, SQLite, config, provider, routing, or logging changes.
- Scheduled pruning implementation.
- Configurable retention duration.
- Permanent tests.

## Verification

Run:

```sh
rg -n "Telemetry retention|Manual pruning|Scheduled pruning|metadata pruning|prune" docs/ilonasin-architecture.md internal/management internal/storage/sqlite internal/tui
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke even though this is
docs-only, to keep the slice discipline consistent.

## Acceptance

- Architecture states manual telemetry pruning as the current decision.
- Architecture no longer lists pruning mode as an open question.
- The wording matches current code and prior pruning plans.
- No runtime behavior changes.

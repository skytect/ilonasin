# 493 Management Pruning Availability

## Context

Plan 490 found that management snapshots set `PruningAvailable = true`
unconditionally, even though `Service.PruneTelemetry` returns unavailable when
`Service.Pruner` is nil. Management snapshots should describe the daemon's
actual mutable operation surface, not a compile-time assumption.

The production daemon wires SQLite as the pruner, so normal `ilonasin serve`
behavior should remain unchanged.

## Goal

Make `ManagementSnapshotResponse.pruning_available` reflect whether the
management service has a telemetry pruner.

## Scope

1. Update `internal/management/snapshot.go` so `PruningAvailable` is true only
   when `s.Pruner != nil`.
2. Preserve `Service.PruneTelemetry` behavior and the existing unavailable error
   path.
3. Preserve production app wiring, where `internal/app/management.go` provides
   the SQLite store as `Pruner`.
4. Preserve TUI behavior except that the pruning pane can hide when both the
   TUI lacks a pruner client and the daemon snapshot reports pruning
   unavailable.
5. Do not change pruning semantics, retention windows, management routes, TUI
   keybindings, SQLite schema, or telemetry deletion behavior.
6. Do not add permanent tests.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run focused temporary checks that are removed before commit:

1. Call `management.Service{}.LoadManagementSnapshot` and confirm
   `PruningAvailable` is false.
2. Call `LoadManagementSnapshot` on a service with a stub pruner and confirm
   `PruningAvailable` is true.

Run direct CLI smokes with a temporary binary:

1. Start `ilonasin serve` with isolated home and a valid config.
2. Verify management health and snapshot over the Unix socket, including
   `pruning_available: true` for the production daemon.
3. Run bounded `ilonasin manage` through a PTY at narrow and wide widths.
4. Clean up all temporary files and processes.

## Acceptance

- Snapshot pruning availability matches `s.Pruner != nil`.
- Production daemon snapshots still report pruning available.
- TUI keeps rendering pruning state from daemon-owned snapshot data.
- Direct `serve` and `manage` smokes pass.

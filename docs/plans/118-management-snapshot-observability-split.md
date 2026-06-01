# 118 Management Snapshot Observability Split

## Context

Plans 114 through 117 split management snapshot sanitizers, DTOs, conversion
helpers, and visibility filters out of `internal/management/snapshot.go`. The
file now owns the snapshot route path, client interface, top-level snapshot
loading, and the observability read orchestration helper.

The architecture says `ilonasin manage` is a client of the daemon-owned local
management API, with SQLite-backed inspection performed by the daemon. Snapshot
loading should remain the top-level orchestration boundary, while observability
snapshot loading is a focused read cluster for metadata-only request, usage,
latency, stream, health, fallback, and quota rows.

This slice is behavior-preserving. It does not change management response JSON,
row limits, sanitizer policy, metadata queries, route registration, storage,
provider behavior, config, or TUI behavior.

## Goal

Move the management observability snapshot loading helper into a focused
same-package file without changing behavior.

After this slice, `snapshot.go` owns the snapshot route path, client interface,
and `LoadManagementSnapshot`. `snapshot_observability.go` owns loading the
observability section of the management snapshot.

## Scope

1. Create `internal/management/snapshot_observability.go`.
2. Move `loadObservabilitySnapshot` from `internal/management/snapshot.go` into
   the new file unchanged.
3. Keep `LoadManagementSnapshot`, `PathSnapshot`, and `SnapshotClient` in
   `snapshot.go`.
4. Preserve the observability read order and limits:
   - recent requests limited to 5,
   - usage by provider,
   - latency by provider,
   - stream summary,
   - latest health,
   - recent fallbacks limited to 5,
   - quota by provider.
5. Preserve conversion helper calls, error propagation, sanitizer behavior, and
   final response shape.
6. Do not change DTOs, visibility filters, conversion helpers, sanitizer policy,
   management routes, TUI behavior, storage, provider behavior, or config
   behavior.
7. Do not add permanent tests.
8. Do not push.

## Non-Goals

- No observability query changes.
- No row limit changes.
- No response shape or field changes.
- No subscription usage changes.
- No TUI layout work.
- No provider, routing, or credential changes.
- No broad refactor of `subscription_usage.go` or provider files in this slice.

## Implementation

1. Add `snapshot_observability.go` with `package management`.
2. Move `loadObservabilitySnapshot` intact into the new file.
3. Add only the imports needed by that helper.
4. Remove now-unused imports from `snapshot.go`.
5. Run `gofmt` on touched Go files.
6. Review the diff before smoke checks. The Go diff should be move-only except
   import cleanup.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
git diff --check
```

Then run the direct CLI smoke:

- build a fresh `ilonasin` binary,
- start `ilonasin serve --config "$cfg"` with a temporary `ILONASIN_HOME`,
- wait for the management socket,
- verify `/_ilonasin/manage/snapshot`,
- run `ilonasin manage --config "$cfg"` under a short PTY timeout,
- terminate the daemon and remove temporary files.

## Acceptance

- Compile/package check passes.
- Vet passes.
- `git diff --check` passes.
- Fresh binary builds.
- Direct `serve` smoke exposes the snapshot route.
- Direct `manage` smoke reaches the daemon-backed TUI path and exits cleanly or
  times out with status 124.
- Diff is move-only except import cleanup.

## Review Questions

1. Is `snapshot_observability.go` the right boundary after DTO, sanitizer,
   conversion, and visibility splits?
2. Should `LoadManagementSnapshot` remain in `snapshot.go` as the top-level
   orchestration point for this slice?
3. Are compile, vet, diff whitespace, and direct serve/manage smokes enough for
   this move-only extraction?

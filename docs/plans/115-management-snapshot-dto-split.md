# 115 Management Snapshot DTO Split

## Context

Plans 105 and 114 moved subscription usage DTOs and snapshot sanitizers out of
`internal/management/snapshot.go`. The file still owns three concerns:

- management snapshot DTO declarations,
- snapshot loading and observability loading,
- metadata/provider/credential DTO conversion helpers.

The architecture treats the daemon-owned management API as the boundary between
`ilonasin manage` and SQLite-backed state. Keeping route DTOs easy to audit
helps preserve that boundary and makes future management slices smaller.

## Goal

Move management snapshot DTO type declarations into a focused same-package file
without changing behavior.

After this slice, `snapshot.go` owns snapshot loading and conversion helpers.
`snapshot_dto.go` owns the JSON DTO declarations for the management snapshot
route.

## Scope

1. Create `internal/management/snapshot_dto.go`.
2. Move these type declarations from `snapshot.go` unchanged:
   - `ManagementSnapshotResponse`
   - `ProviderInstance`
   - `UpstreamCredential`
   - `FallbackPolicy`
   - `OAuthCredential`
   - `ProviderAccount`
   - `ModelMetadata`
   - `RequestSummary`
   - `UsageSummary`
   - `LatencySummary`
   - `StreamSummary`
   - `HealthSummary`
   - `FallbackSummary`
   - `QuotaSummary`
3. Keep `SnapshotClient`, `PathSnapshot`, and `LoadManagementSnapshot` in
   `snapshot.go`.
4. Preserve all field names, JSON tags, types, and ordering.
5. Do not change sanitization, route behavior, TUI behavior, storage, provider
   behavior, or config behavior.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No DTO reshaping.
- No new fields.
- No sanitizer policy changes.
- No management route changes.
- No broad conversion helper split in this slice.

## Implementation

1. Add `snapshot_dto.go` with `package management`.
2. Move the listed DTOs into the new file.
3. Add only the imports needed by DTO fields, currently `time`.
4. Remove now-unused imports from `snapshot.go`.
5. Run `gofmt`.
6. Review the diff to confirm it is move-only.

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
- Direct `manage` smoke reaches the daemon-backed TUI path and exits cleanly
  or times out with status 124.
- Diff is move-only except import cleanup.

## Review Questions

1. Is `snapshot_dto.go` a useful boundary after the sanitizer split?
2. Should `SnapshotClient` stay in `snapshot.go` with the loader, or move with
   DTOs later?
3. Is preserving DTO ordering worth doing in this move-only slice?

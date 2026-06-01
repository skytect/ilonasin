# 114 Management Snapshot Sanitizer Split

## Context

`internal/management/snapshot.go` now owns the management snapshot DTOs,
snapshot loading, visibility filters, DTO conversion helpers, observability
loading, and snapshot sanitization. The architecture requires management
snapshots to stay metadata-only and to avoid exposing prompts, completions,
request bodies, response bodies, raw provider payloads, tool data, full bearer
tokens, full provider request IDs, full account IDs, balances, or credits.

The sanitizer code is important enough to keep focused and easy to audit. It
currently lives in the middle of `snapshot.go`, separating snapshot loading
from DTO conversion helpers.

## Goal

Move management snapshot sanitization into a focused same-package file without
changing behavior.

After this slice, `snapshot.go` still owns snapshot DTOs, loading, visibility
filters, and DTO conversions. `snapshot_sanitize.go` owns the redaction pattern
and all snapshot sanitizer helpers.

## Scope

1. Create `internal/management/snapshot_sanitize.go`.
2. Move these items from `snapshot.go` unchanged:
   - `unsafeSnapshotStringPattern`
   - `sanitizeSnapshot`
   - `safeSnapshotString`
   - `safeEndpointString`
   - `safeRefreshFailureDescription`
   - `safeRefreshFailureClass`
   - `safeMachineString`
   - `safeTokenFragment`
   - `safeSecretFragment`
   - `hasAllowedUnsafePrefix`
   - `safeBaseURL`
3. Keep `sanitizeSubscriptionUsageResponse` in `subscription_usage.go` because
   it is shared by the standalone subscription usage route and the full
   management snapshot.
4. Keep behavior, exported JSON shape, redaction strings, and URL sanitization
   unchanged.
5. Do not add permanent tests.
6. Do not push.

## Non-Goals

- No sanitizer policy changes.
- No management route changes.
- No DTO reshaping.
- No SQLite, provider, server, config, or TUI changes.
- No broad snapshot loader split in this slice.

## Implementation

1. Add `snapshot_sanitize.go` with `package management`.
2. Move the listed sanitizer code and its imports into that file.
3. Remove now-unused imports from `snapshot.go`.
4. Run `gofmt`.
5. Review the diff to confirm it is move-only.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
git diff --check
tmpbin=$(mktemp -d)
tmp=$(mktemp -d)
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
```

Then run the direct daemon smoke:

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

1. Is moving sanitizer code out of `snapshot.go` the right next management
   boundary?
2. Should `safeBaseURL` move with sanitizer helpers even though it is used by a
   conversion helper?
3. Is keeping `sanitizeSubscriptionUsageResponse` in `subscription_usage.go`
   the correct sharing point for the standalone route?

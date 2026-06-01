# 116 Management Snapshot Conversion Split

## Context

Plans 114 and 115 moved management snapshot sanitizers and DTO declarations
out of `internal/management/snapshot.go`. The file now owns snapshot loading,
visibility filtering, and DTO conversion helpers.

The architecture treats the daemon-owned management API as the boundary between
the TUI and SQLite-backed state. Keeping conversion code separate from loading
logic makes that boundary easier to audit: loading should orchestrate daemon
reads, while conversion helpers should translate safe metadata/provider
records into management DTOs.

## Goal

Move management snapshot DTO conversion helpers into a focused same-package
file without changing behavior.

After this slice, `snapshot.go` owns the route path, client interface, snapshot
loading, observability loading, and visibility filters. `snapshot_convert.go`
owns metadata/provider/credential-to-management DTO conversions.

## Scope

1. Create `internal/management/snapshot_convert.go`.
2. Move these helper functions from `snapshot.go` unchanged:
   - `providerInstanceFromProvider`
   - `upstreamCredentialsFromCredentials`
   - `fallbackPoliciesFromCredentials`
   - `oauthCredentialsFromCredentials`
   - `providerAccountsFromCredentials`
   - `modelMetadataFromProvider`
   - `requestSummariesFromMetadata`
   - `usageSummariesFromMetadata`
   - `latencySummariesFromMetadata`
   - `streamSummariesFromMetadata`
   - `healthSummariesFromMetadata`
   - `fallbackSummariesFromMetadata`
   - `quotaSummariesFromMetadata`
3. Keep visibility filters in `snapshot.go` for this slice because they depend
   on provider registry policy, not DTO conversion.
   Keep `fallbackPolicyProviderKinds`, `apiKeyProviderIDs`, and
   `oauthProviderIDs` with those visibility filters.
4. `providerInstanceFromProvider` may continue calling `safeBaseURL` from
   `snapshot_sanitize.go`; this is an existing same-package sanitizer
   dependency used to keep provider instance DTOs metadata-safe. Moving the
   conversion helper does not move or change sanitizer policy.
5. Keep `loadObservabilitySnapshot` in `snapshot.go` for this slice because it
   orchestrates reads from the observability interface.
6. Preserve JSON shape, sanitizer behavior, row filtering, ordering, and all
   field assignments.
7. Do not add permanent tests.
8. Do not push.

## Non-Goals

- No conversion behavior changes.
- No sanitizer policy changes.
- No management route changes.
- No DTO reshaping.
- No storage, provider, server, config, or TUI changes.
- No broad snapshot loader split in this slice.

## Implementation

1. Add `snapshot_convert.go` with `package management`.
2. Move the listed conversion helpers into the new file.
3. Add only the imports needed by the moved helpers.
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

1. Is `snapshot_convert.go` the right boundary after DTO and sanitizer splits?
2. Should visibility filters and their provider-kind helper maps remain in
   `snapshot.go` for now?
3. Should `loadObservabilitySnapshot` remain with the loader until a separate
   observability-loader split?

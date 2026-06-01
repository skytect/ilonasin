# 117 Management Snapshot Visibility Split

## Context

Plans 114 through 116 split management snapshot sanitizers, DTOs, and
conversion helpers out of `internal/management/snapshot.go`. The file now owns
snapshot loading, observability loading, and visibility policy helpers for
filtering credentials/accounts/fallback policies by configured provider
capability.

The architecture says `ilonasin manage` is a client of the daemon-owned
management API and that the daemon performs SQLite reads and writes behind
that boundary. Snapshot loading should stay focused on orchestrating those
management reads. Visibility policy is a separate concern that decides which
daemon rows are relevant to the management snapshot.

## Goal

Move management snapshot visibility filtering helpers into a focused
same-package file without changing behavior.

After this slice, `snapshot.go` owns the snapshot route path, client
interface, snapshot loading, and observability loading. `snapshot_visibility.go`
owns registry-based visibility filters.

## Scope

1. Create `internal/management/snapshot_visibility.go`.
2. Move these helper functions from `snapshot.go` unchanged:
   - `visibleUpstreamCredentials`
   - `visibleFallbackPolicies`
   - `fallbackPolicyProviderKinds`
   - `apiKeyProviderIDs`
   - `visibleOAuthCredentials`
   - `visibleProviderAccounts`
   - `oauthProviderIDs`
3. Preserve provider filtering behavior, fallback policy visibility behavior,
   row ordering, and in-place slice reuse behavior.
4. Keep `LoadManagementSnapshot` and `loadObservabilitySnapshot` in
   `snapshot.go`.
5. Do not change DTOs, conversion helpers, sanitizer policy, route behavior,
   TUI behavior, storage, provider behavior, or config behavior.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visibility policy changes.
- No new filtering rules.
- No management route changes.
- No DTO or JSON shape changes.
- No observability-loader split in this slice.

## Implementation

1. Add `snapshot_visibility.go` with `package management`.
2. Move the listed visibility helpers into the new file.
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

1. Is `snapshot_visibility.go` the right boundary after conversion splitting?
2. Is keeping in-place slice reuse behavior important for move-only fidelity?
3. Should `loadObservabilitySnapshot` remain in `snapshot.go` until a separate
   observability-loader split?

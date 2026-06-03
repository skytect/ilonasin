# 347 TUI Snapshot Trust Boundary

## Context

`docs/ilonasin-architecture.md` says `ilonasin manage` is a client of the
daemon-owned management API. The daemon performs SQLite reads and writes behind
that management boundary, and the TUI should consume management DTOs rather than
re-owning storage/domain visibility rules.

Management snapshot construction already filters upstream credentials and
fallback policies before returning DTOs:

- `internal/management/snapshot.go` calls management visibility helpers before
  converting credential-domain rows to snapshot DTOs;
- `internal/management/snapshot_visibility.go` and `internal/management/upstreams.go`
  own those visibility rules.

The TUI still applies a second visibility pass in `applySnapshot`:

```go
m.credentials = m.visibleUpstreamCredentials(snapshot.UpstreamCredentials)
m.fallbackPolicies = m.visibleFallbackPolicies(snapshot.FallbackPolicies)
```

That duplicates daemon management API policy in the TUI and keeps the client
partly responsible for deciding which management DTOs are visible.

## Scope

1. Keep this slice limited to:
   - `internal/tui/snapshot.go`;
   - TUI action helper files only if helpers become unused;
   - this plan.
2. Change `applySnapshot` to trust the management snapshot for upstream
   credential and fallback-policy visibility:
   - copy `snapshot.UpstreamCredentials` into `m.credentials`;
   - copy `snapshot.FallbackPolicies` into `m.fallbackPolicies`.
3. Remove now-unused TUI duplicate visibility helpers if they have no remaining
   call sites:
   - `Model.visibleUpstreamCredentials`;
   - `Model.visibleFallbackPolicies`;
   - `Model.visibleProviderRows`.
4. Preserve all action behavior:
   - disabling an upstream credential still uses the first non-disabled
     management DTO in `m.credentials`;
   - fallback enable/disable still uses management DTOs in `m.fallbackPolicies`;
   - provider selection for adding API keys and OAuth login remains based on
     `m.providers`;
   - management-side `VisibleFallbackPolicies` remains the source of fallback
     visibility.
5. Do not change management DTOs, management routes, storage schema, provider
   behavior, credential mutation, config, logging, affinity, quota behavior,
   visual rendering, or permanent tests.

## Verification

Before implementation review:

1. Review the diff manually for scope and behavior.
2. Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/tui ./internal/management
go test ./...
go vet ./...
```

3. Run a temporary focused TUI smoke, removed before commit, that seeds a
   `ManagementSnapshotResponse` with upstream credential and fallback-policy
   rows and asserts `applySnapshot` copies those rows without applying a second
   provider-based visibility filter.
4. Build `ilonasin`, start `ilonasin serve` with an isolated temporary
   `ILONASIN_HOME`, verify the management health route over the Unix socket,
   run a short `ilonasin manage` TUI smoke, then terminate and clean up.

## Expected Outcome

- The TUI treats management snapshot DTOs as already-authoritative visibility
  output.
- Duplicate TUI-side credential and fallback visibility policy is removed.
- Snapshot JSON and management-side behavior are unchanged.

# 346 Management Snapshot Visibility Purity

## Context

`docs/ilonasin-architecture.md` treats `ilonasin manage` as a client of the
daemon-owned management API. Snapshot construction should be a management-boundary
projection from daemon-owned state into safe DTOs.

`internal/management/snapshot_visibility.go` currently filters credential-domain
rows by reusing the caller's slice backing array:

```go
out := rows[:0]
```

This does not change current visible behavior, but it makes the visibility
helpers mutate caller-owned slice storage as an implementation side effect. For
snapshot building, a pure projection is easier to audit and less likely to
create aliasing surprises as the management boundary keeps moving away from
legacy storage/domain details.

## Scope

1. Keep this slice limited to:
   - `internal/management/snapshot_visibility.go`;
   - `internal/management/upstreams.go`;
   - this plan.
2. Change the snapshot visibility helpers to allocate new result slices:
   - `visibleUpstreamCredentials`;
   - `visibleFallbackPolicyMetadata`, which currently lives with upstream
     fallback-policy helpers;
   - `visibleOAuthCredentials`;
   - `visibleProviderAccounts`.
3. Preserve exact filtering semantics:
   - upstream API key credentials remain visible only for API-key provider
     instances;
   - fallback policies remain filtered by management fallback-policy visibility;
   - OAuth credentials and provider accounts remain visible only for OAuth
     provider instances;
   - provider allowlists and credential counts are unchanged.
4. Do not change management DTO fields, route shapes, storage schema, provider
   behavior, credential mutation, TUI rendering, config, logging, affinity,
   quota behavior, or permanent tests.

## Verification

Before implementation review:

1. Review the diff manually for scope and behavior.
2. Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/management ./internal/tui
go test ./...
go vet ./...
```

3. Run a temporary focused management smoke, removed before commit, that seeds
   the four visibility helpers with mixed visible/hidden rows and asserts:
   - visible output behavior is unchanged;
   - the caller input slice length is unchanged;
   - caller input slice element order and contents are unchanged.
4. Build `ilonasin`, start `ilonasin serve` with an isolated temporary
   `ILONASIN_HOME`, verify the management health route over the Unix socket,
   run a short `ilonasin manage` TUI smoke, then terminate and clean up.

## Expected Outcome

- Management snapshot visibility filtering no longer mutates caller-owned slice
  storage.
- Snapshot JSON and TUI-visible behavior are unchanged.
- The management boundary is easier to audit as a pure DTO projection step.

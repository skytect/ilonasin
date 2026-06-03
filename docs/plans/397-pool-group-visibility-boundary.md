# 397 Pool Group Visibility Boundary

## Context

Credential pool groups replaced the old fallback-policy control surface, but a
small part of the management boundary is still awkward:

- `visibleCredentialPoolGroups` is a snapshot visibility helper in
  `snapshot_visibility.go`,
- the real filtering logic it delegates to, including provider/kind allowlist
  construction, still lives in `upstreams.go`,
- `upstreams.go` also owns upstream credential mutations and upstream DTO
  conversion.

This makes pool-group snapshot visibility look tied to upstream mutation code
instead of the management snapshot visibility boundary.

## Goal

Move credential pool group visibility policy into the management snapshot
visibility boundary without changing behavior.

## Scope

1. Move `poolGroupCredentialKinds`,
   `allowedPoolGroupCredentialKindsByProvider`, and
   `visibleCredentialPoolGroupMetadata` from `internal/management/upstreams.go`
   to `internal/management/snapshot_visibility.go`.
2. Collapse the wrapper so snapshot loading calls one local visibility helper
   with the same name and semantics.
3. Keep `credentialPoolGroupFromCredentials` in `upstreams.go` for this slice,
   because it is DTO conversion beside `upstreamCredentialFromCredentials`.
4. Do not change:
   - management snapshot JSON,
   - TUI rendering,
   - SQLite schema or migrations,
   - credential mutation behavior,
   - provider routing or credential pooling,
   - fallback, health, quota, logging, or subscription usage behavior.

## Verification

Run:

```sh
rg -n "visibleCredentialPoolGroups|visibleCredentialPoolGroupMetadata|allowedPoolGroupCredentialKindsByProvider|poolGroupCredentialKinds" internal/management
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/management
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke at narrow and wide
terminal widths.

## Acceptance

- Credential pool group visibility logic lives with snapshot visibility.
- The extra wrapper/delegate is gone.
- Pool groups are still visible only for allowed provider/kind pairs with at
  least two credentials.
- Runtime behavior and public management/TUI surfaces are unchanged.

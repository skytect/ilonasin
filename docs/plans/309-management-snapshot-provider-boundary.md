# 309 Management Snapshot Provider Boundary

## Context

`docs/ilonasin-architecture.md` treats the daemon management API as its own
boundary. The TUI should consume management DTOs, and management snapshot
construction should not need provider registry domain objects for display and
visibility filtering.

Current management snapshot code still depends directly on provider registry
types:

- `management.Service.Registry` is a `provider.Registry`;
- `LoadManagementSnapshot` calls `s.Registry.List()` to build provider DTOs;
- snapshot visibility helpers accept `provider.Registry`;
- snapshot conversion imports `provider.Instance` for provider rows.

This slice is snapshot-only. It must not change provider registry behavior,
subscription usage refresh, credential mutation policy, provider adapters,
storage, config loading, TUI rendering, or public management JSON shape.

## Plan

1. Add a management-owned provider catalog type or reuse the existing
   `management.ProviderInstance` DTO as the `Service` snapshot catalog.
2. Convert `provider.Registry.List()` to management provider rows once in
   `internal/app/management.go` when constructing `management.Service`.
   Snapshot construction must copy those rows before returning output so
   `sanitizeSnapshot` cannot mutate service-owned catalog state.
3. Add a separate `management.Service.Providers` snapshot catalog and keep the
   existing provider registry field for non-snapshot paths such as
   subscription usage refresh.
4. Update `snapshot.go`, `snapshot_convert.go`, and
   `snapshot_visibility.go` so snapshot construction and visibility helpers no
   longer import or depend on `provider.Registry` or `provider.Instance`.
5. Preserve existing visibility semantics:
   - upstream API key credentials visible only for API-key-capable providers;
   - OAuth credentials and provider accounts visible only for OAuth-capable
     providers;
   - fallback policies visible for API key providers and Codex OAuth providers
     when credential count is at least two.
6. Leave remaining management provider usage for subscription usage refresh and
   OAuth operations out of scope for this slice.
7. Review code before checks for JSON compatibility, visibility drift,
   accidental TUI changes, and unintended changes to credential mutations.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/management
go test ./internal/app
go test ./...
go vet ./...
! rg -n 'provider\.Registry|provider\.Instance|providerInstanceFromProvider' internal/management/snapshot.go internal/management/snapshot_convert.go internal/management/snapshot_visibility.go
```

Run a temporary focused smoke, then remove it before commit. It must:

- construct a management service with API-key, OAuth Codex, and non-OAuth
  provider rows;
- seed upstream credentials, OAuth credentials, provider accounts, and fallback
  policies through fake management readers;
- assert snapshot provider JSON is unchanged for provider fields;
- assert visibility filtering matches the pre-existing semantics listed above;
- assert provider row slices are copied so callers cannot mutate service-owned
  catalog state through snapshot output.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temp directory.

## Acceptance

- Management snapshot construction uses management-owned provider rows for
  display and visibility filtering, while non-snapshot management paths can
  continue using the provider registry.
- `snapshot.go`, `snapshot_convert.go`, and `snapshot_visibility.go` no longer
  depend on provider registry types.
- Existing management snapshot JSON and TUI-visible data remain compatible.
- No subscription usage refresh, credential mutation, storage, provider
  adapter, config, or TUI behavior changes are introduced.

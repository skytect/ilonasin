# 374 Remove Fallback Policies Snapshot Alias

## Context

Plan 372 introduced `credential_pool_groups` as the primary management snapshot
field and kept `fallback_policies` as a temporary compatibility alias. The
in-repo TUI now understands `CredentialPoolGroups`, and plan 373 renamed the
provider pane away from fallback terminology.

The remaining live alias keeps the removed fallback-policy model in the
management snapshot DTO and TUI compatibility path. The final architecture
should expose credential pool groups directly, without a fallback-policy alias.

## Goal

Remove `fallback_policies` from live management snapshots and TUI snapshot
application so `credential_pool_groups` is the only provider credential pool
group wire field.

## Scope

1. Remove `ManagementSnapshotResponse.FallbackPolicies`.
2. Stop populating the fallback alias in service snapshots.
3. Stop sanitizing the fallback alias.
4. Remove the TUI fallback-to-`FallbackPolicies` compatibility path.
5. Keep `CredentialPoolGroups []CredentialPoolGroup
   json:"credential_pool_groups"` unchanged.
6. Keep logs fallback event metadata unchanged.
7. Do not change `CredentialPoolGroup` row shape, SQLite schema, credential
   group listing, serving routing, credential resolution, quota handling,
   fallback event recording, provider adapters, config, logging, or
   subscription usage.

## Verification

Run a temporary focused smoke, then remove it before commit. It should prove:

- management snapshots marshal `credential_pool_groups`;
- management snapshots do not marshal `fallback_policies`;
- TUI snapshot application uses `CredentialPoolGroups`.

Then run:

```sh
rg -n "fallback_policies|FallbackPolicies" internal
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/management
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with isolated temporary home and config, checking management health over the
Unix socket, verifying management snapshot JSON contains `credential_pool_groups`
and not `fallback_policies`, running bounded `ilonasin manage` at narrow and
wide terminal widths, and cleaning up all temporary files and processes.

## Acceptance

- Live management snapshots expose only `credential_pool_groups` for provider
  credential pool groups.
- No live `fallback_policies` or `FallbackPolicies` references remain in
  `internal`.
- TUI uses only `CredentialPoolGroups` for provider pool groups.
- Fallback event log metadata remains unchanged.

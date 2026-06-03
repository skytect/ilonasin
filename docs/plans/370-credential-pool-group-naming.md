# 370 Credential Pool Group Naming

## Context

Plans 365 through 369 removed fallback-policy mutation semantics, stopped
reading stale `credential_fallback_policies` state into snapshots, and removed
dead exported management fallback helpers. The remaining live code still names
the snapshot DTO and model fields `FallbackPolicy`, `FallbackPolicies`, and
`fallbackPolicy...` even though they now represent active credential pool group
metadata.

`docs/ilonasin-architecture.md` says serving eligibility is the default
same-provider credential pool. Keeping policy terminology in live management
and TUI internals makes the code look like it still models a policy-control
surface.

## Goal

Rename the live management and TUI fallback-policy metadata path to credential
pool group terminology while preserving snapshot wire compatibility for this
slice.

## Scope

1. Rename credentials metadata type from `FallbackPolicyMetadata` to
   `CredentialPoolGroupMetadata`.
2. Rename management DTO type from `FallbackPolicy` to `CredentialPoolGroup`.
3. Keep `ManagementSnapshotResponse.FallbackPolicies` and JSON
   `fallback_policies` unchanged as a compatibility alias in this slice.
4. Rename conversion, visibility, storage listing, and TUI helper names to pool
   group terminology.
5. Keep SQLite method behavior unchanged: summarize active
   `provider_credentials` groups by provider instance, credential kind, and
   fallback group.
6. Keep the legacy `credential_fallback_policies` schema and historical rows
   untouched.
7. Do not change serving routing, credential resolution, quota handling,
   fallback event recording, health metadata, provider adapters, config,
   logging, or subscription usage.

## Verification

Review the diff before checks for:

- no live source type or helper still named `FallbackPolicy` except the
  `ManagementSnapshotResponse.FallbackPolicies` compatibility field and older
  plan/docs text;
- management snapshot JSON remains compatible;
- storage query and visibility behavior remain unchanged;
- TUI output remains credential pool group terminology.

Then run:

```sh
rg -n "FallbackPolicy|fallbackPolicy|fallbackPolicies" internal
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/credentials
go test ./internal/storage/sqlite
go test ./internal/management
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Live code uses credential pool group terminology for this metadata path.
- Snapshot wire compatibility is preserved intentionally.
- Legacy fallback-policy schema remains intact for compatibility.
- Serving behavior and credential-pool eligibility are unchanged.

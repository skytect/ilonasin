# 372 Credential Pool Group Snapshot Alias

## Context

Plans 365 through 371 removed fallback-policy mutation semantics, renamed live
metadata to credential pool groups, and dropped the unused SQLite policy table.
The remaining management snapshot wire field is still `fallback_policies`.

The only in-repo snapshot consumer is the local management client/TUI. The
final architecture should not retain legacy policy terminology as the primary
wire surface, but abruptly removing `fallback_policies` would be an avoidable
compatibility break.

## Goal

Expose `credential_pool_groups` as the primary management snapshot field while
keeping `fallback_policies` as a compatibility alias during migration.

## Scope

1. Add `CredentialPoolGroups []CredentialPoolGroup
   json:"credential_pool_groups"` to `ManagementSnapshotResponse`.
2. Keep `FallbackPolicies []CredentialPoolGroup json:"fallback_policies"` as a
   compatibility alias.
3. Populate both fields from the same sanitized credential pool group data in
   service snapshots.
4. Make the TUI prefer `CredentialPoolGroups`, falling back to
   `FallbackPolicies` for older daemons.
5. Update snapshot sanitization so both fields are sanitized consistently.
6. Do not change the `CredentialPoolGroup` row shape, SQLite schema,
   credential group listing, serving routing, credential resolution, quota
   handling, fallback event recording, provider adapters, config, logging, or
   subscription usage.

## Verification

Run a temporary focused management smoke, then remove it before commit. It
should prove:

- service snapshots populate both `credential_pool_groups` and
  `fallback_policies`;
- both aliases marshal to JSON with identical row content;
- TUI snapshot application prefers `CredentialPoolGroups`;
- TUI snapshot application still falls back to `FallbackPolicies` when the new
  field is empty.

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
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- New local management snapshots expose `credential_pool_groups`.
- Existing clients can still read `fallback_policies` during migration.
- The TUI uses the new field when available and remains compatible with older
  daemons.
- No serving, storage, provider, quota, logging, or subscription behavior
  changes.

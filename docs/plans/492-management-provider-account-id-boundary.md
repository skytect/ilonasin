# 492 Management Provider Account ID Boundary

## Context

Plan 490 found that management snapshots expose `ProviderAccount.ID` as a
second stable account identity next to `CredentialID`. The architecture says
observable account references should use local credential IDs, safe display
labels, or one-way account hashes, and the TUI currently uses OAuth credential
IDs for account actions.

Provider account row IDs are daemon-internal storage details. They should not
be part of the management snapshot unless a current management operation needs
them.

## Goal

Remove provider account row IDs from the management snapshot DTO while
preserving visible OAuth account rows, credential IDs, labels, plan labels, and
created timestamps.

## Scope

1. Remove `ProviderAccount.ID` from `internal/management/snapshot_dto.go`.
2. Stop copying `credentials.ProviderAccountMetadata.ID` into management
   snapshots in `internal/management/snapshot_convert.go`.
3. Confirm TUI provider account rendering and selection do not depend on the
   removed field.
4. Preserve `CredentialID`, `ProviderInstanceID`, display labels, plan labels,
   created timestamps, OAuth credential DTOs, subscription usage DTOs,
   credential storage, refresh actions, and management mutation routes.
5. Do not change SQLite schema or credential repository types in this slice.
6. Do not add permanent tests.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes with a temporary binary:

1. Start `ilonasin serve` with isolated home and a valid config.
2. Verify management health and snapshot over the Unix socket.
3. Run a temporary focused management-service or seeded-store harness that
   returns at least one provider account row, then confirm
   `provider_accounts[0]` does not contain an `id` field and still contains
   `credential_id`, safe labels, plan label, and created timestamp. Remove any
   temporary harness before committing.
4. Run a source-level compatibility check scoped to `ProviderAccount` usage so
   no management or TUI path still depends on the removed field.
5. Run bounded `ilonasin manage` through a PTY at narrow and wide widths.
6. Clean up all temporary files and processes.

## Acceptance

- Management provider account rows no longer expose storage row IDs.
- OAuth account rendering keeps using credential IDs and safe labels.
- No management mutation route loses the identifier it needs.
- Direct `serve` and `manage` smokes pass.

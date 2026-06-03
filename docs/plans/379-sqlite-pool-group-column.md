# 379 SQLite Pool Group Column

## Context

Slice 378 moved live domain, management, and TUI code from
`FallbackGroup` / `fallback_group` to `PoolGroup` / `pool_group`, leaving
SQLite as the only live legacy boundary. The remaining live internal references
to `fallback_group` are SQL statements and historical migration text.

The architecture describes credential grouping as credential pool groups.
Completing this naming boundary requires the active SQLite schema to use
`provider_credentials.pool_group` while keeping historical migrations intact.

## Scope

1. Add migration 12 that renames `provider_credentials.fallback_group` to
   `pool_group` with guarded `ALTER TABLE ... RENAME COLUMN` logic.
2. Do not rebuild or drop `provider_credentials`. It is a parent table for
   credential secrets, OAuth state, account metadata, request metadata, health,
   quota, subscription usage, and fallback events; dropping it under SQLite
   foreign-key enforcement can fail or cascade child rows.
3. Preserve `UNIQUE(provider_instance_id, label)`.
4. Preserve credential IDs and every child-table foreign-key reference.
5. Update SQLite storage queries and writes to use `pool_group`.
6. Keep historical migrations 3 and 4 unchanged. They may still mention
   fallback-policy and `fallback_group` for older upgrade paths.
7. Keep credential pool group behavior unchanged: group summaries are still
   derived from active `provider_credentials` rows by provider instance, kind,
   and group label.
8. Do not change serving credential selection, management DTOs, TUI layout,
   provider adapters, quota behavior, logging, request metadata, subscription
   usage semantics, or config.

## Verification

Review the diff before checks for:

- no live SQLite storage query or write still references `fallback_group`;
- historical `fallback_group` references remain only in migrations 3, 4, and
  the new migration 12 rename guard;
- fresh migrated databases contain `provider_credentials.pool_group` and not
  `fallback_group`;
- upgraded databases preserve provider credential IDs, group values, and child
  table rows;
- `PRAGMA foreign_key_check` returns no rows after migration;
- `UNIQUE(provider_instance_id, label)` still rejects duplicates after
  migration;
- the guarded rename helper handles old-only and new-only schema states, and
  errors on both-columns or neither-column states;
- credential pool group listing still returns the same group labels.

Then run:

```sh
rg -n "fallback_group" internal/storage/sqlite
rg -n "pool_group" internal/storage/sqlite
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/storage/sqlite ./internal/credentials ./internal/management ./internal/tui
go test ./...
go vet ./...
```

Use temporary smoke code or direct CLI/API checks, then remove any temporary
files before commit, to prove:

- fresh migration has `pool_group` and no `fallback_group`;
- upgrade from a database with version 11 and `fallback_group` preserves
  provider credential rows and child rows;
- rerunning migration code is idempotent when `pool_group` already exists;
- both-column and neither-column schema states fail fast instead of silently
  choosing one;
- API-key insertion writes `pool_group = "default"`;
- management add-credential and snapshot JSON still expose `pool_group`;
- `ListCredentialPoolGroups` still derives summaries from active credentials.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Active SQLite schema uses `provider_credentials.pool_group`.
- Live storage code no longer uses the `fallback_group` column.
- Historical migration compatibility remains intact.
- Serving behavior, metadata recording, and management/TUI display behavior are
  unchanged except for the active schema column name.

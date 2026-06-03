# 371 Drop Credential Fallback Policy Table

## Context

Plans 365 through 370 removed fallback-policy mutation semantics, stopped
reading stale policy state, renamed the live metadata path to credential pool
groups, and left only the legacy SQLite table `credential_fallback_policies`.

Current live code derives credential pool groups from active
`provider_credentials` rows and their `fallback_group` column. No live code
reads or writes `credential_fallback_policies`; only migrations create and
transform it.

`docs/ilonasin-architecture.md` says serving eligibility is the default
same-provider credential pool and completion requires legacy code to be cleaned
up. Keeping an unused policy table preserves a removed control model.

## Goal

Add a forward SQLite migration that drops the unused
`credential_fallback_policies` table while preserving active credential pool
group behavior.

## Scope

1. Add a new migration version after the current latest migration.
2. The migration should run `DROP TABLE IF EXISTS credential_fallback_policies`.
3. Keep historical migrations 3 and 4 unchanged for compatibility with older
   migration histories.
4. Keep `provider_credentials.fallback_group` unchanged, because credential pool
   groups still use it.
5. Keep live credential pool group listing derived only from
   `provider_credentials`.
6. Do not change management snapshot JSON, TUI rendering, serving routing,
   credential resolution, quota handling, fallback event recording, health
   metadata, provider adapters, config, or logging.

## Verification

Run a temporary schema smoke and remove it before commit. It should prove:

- a fresh migrated database does not contain `credential_fallback_policies`;
- a database with migrations up to version 9 and a populated
  `credential_fallback_policies` table drops that table after migration 10;
- `provider_credentials.fallback_group` remains present;
- `ListCredentialPoolGroups` still derives groups from active credentials after
  the table is dropped;
- rerunning migrations is idempotent.

Then run:

```sh
rg -n "credential_fallback_policies" internal
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/storage/sqlite
go test ./...
go vet ./...
```

The only remaining `credential_fallback_policies` source references should be
historical migrations 3 and 4 plus the new drop migration.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Fresh and upgraded databases no longer retain
  `credential_fallback_policies` after all migrations.
- Credential pool group listing still works from `provider_credentials`.
- Historical migration compatibility is preserved.
- Serving behavior and credential-pool eligibility are unchanged.

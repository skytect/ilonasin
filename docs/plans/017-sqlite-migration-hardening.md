# Plan 017: SQLite Migration Hardening

## Goal

Make SQLite migrations safe for fresh databases and for the concrete historical
schemas that exist in git history.

The architecture treats SQLite as the mutable source of truth for local client
tokens, upstream credentials, OAuth token metadata, provider accounts, model
cache, request metadata, health, fallback, and migration state. Git history
shows `migration001` already contained the broad initial schema in commit
`ef4f95c`. Later schema changes are versions 2 and 3 only. This slice hardens
those migrations and adds storage-owned smoke checks that prove older databases
catch up without changing runtime behavior.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- prior plans `001` through `016`

## Scope

1. Keep SQLite ownership narrow:
   - storage migrations stay in `internal/storage/sqlite`,
   - no HTTP, provider, TUI, config, or credential service behavior moves into
     migrations,
   - no secrets, prompts, completions, request bodies, response bodies, raw SSE,
     provider payloads, bearer tokens, request IDs, account IDs, tool arguments,
     or tool results are added to schema or smoke output.
2. Add migration support for idempotent schema repair:
   - retain the existing ordered `migrations` table,
   - add a small helper for conditional column additions using `PRAGMA
     table_info`,
   - convert existing migrations 2 and 3 from raw `ALTER TABLE` statements to
     structured conditional steps so already-drifted databases do not fail on
     duplicate columns,
   - use `CREATE TABLE IF NOT EXISTS` for tables that may be absent,
   - avoid `ALTER TABLE ... ADD COLUMN` when a fresh database already has the
     column,
   - keep migration application inside the existing transaction.
3. Treat the exact historical schema inventory as the compatibility target:
   - version 1 from commit `ef4f95c`: all current base tables except
     `provider_credentials.secret_prefix`,
     `provider_credentials.secret_last4`,
     `provider_credentials.fallback_group`, and
     `credential_fallback_policies`,
   - version 2 from commit `db1b9a8`: version 1 plus `secret_prefix` and
     `secret_last4`,
   - version 3 from commit `54f114c`: version 2 plus `fallback_group` and
     `credential_fallback_policies`,
   - fresh database from current `migration001`.
4. Freeze `migration001` as historical schema after this slice:
   - add a source comment saying existing migrations are historical
     compatibility contracts and future schema changes must use new migration
     versions,
   - do not move existing tables out of `migration001`,
   - do not renumber or rewrite existing migration rows.
5. Preserve current fresh-database behavior:
   - a brand new database still opens and migrates successfully,
   - databases do not fail because repair migrations try to add columns already
     present in some database states,
   - version rows remain ordered and auditable.
6. Preserve existing data:
   - repair migrations must not drop, recreate, or rewrite existing user tables,
   - existing credential secret material remains only in `credential_secrets`,
   - existing provider credential labels, disabled state, token prefixes, and
     metadata remain unchanged.
7. Add storage-owned smoke coverage without permanent tests:
   - add one narrow `internal/storage/sqlite` smoke helper that creates
     temporary historical schema fixtures and calls `sqlite.Open`,
   - call that helper from `serve --check`,
   - fixtures cover exact version 1, version 2, version 3, and fresh database
     migration paths,
   - assert required columns, defaults, unique constraints, foreign keys,
     cascade behavior, and migration rows after opening,
   - exercise storage behavior after migration: add/list/resolve API key,
     add/list OAuth, update OAuth refresh metadata, replace/list model cache,
     record request metadata, record stream metrics, record health and fallback
     events, set/list fallback policy,
   - assert existing sentinel rows survive migration,
   - keep synthetic secret markers only in `credential_secrets.secret_material`,
   - clean up all temporary databases.

## Out of Scope

- Replacing the migration system with goose, golang-migrate, or another
  dependency.
- Changing request, credential, provider, TUI, routing, OAuth, fallback, or
  telemetry runtime behavior.
- Adding permanent `*_test.go` files.
- Backfilling new metadata values for historical request rows.
- Renaming tables or compacting old schema history.
- Supporting guessed schemas that are not present in git history.

## Design Constraints

- No permanent tests.
- Do not push.
- Keep migrations append-only after this point.
- Storage performs no network operations.
- Schema repair must be deterministic and transaction-bound.
- Smoke checks must use isolated temporary homes and remove generated files.
- Existing selected user home must not be modified by smoke checks.

## Proposed Package Changes

```text
internal/storage/sqlite/
  migrations.go  # idempotent migrations and conditional helpers
  db.go          # migration runner calls structured migration steps
  smoke.go       # historical schema fixture and behavior smoke helper
internal/app/
  app.go         # call the storage smoke helper from serve --check
```

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Acceptance:

- no permanent test files exist,
- compile/package, vet, build, `serve --check`, `manage --check`, and diff
  whitespace checks pass,
- migration smoke cases prove exact historical schema versions catch up,
- fresh and migrated databases record ordered migration versions without
  rewriting historical rows,
- schema checks cover columns, defaults, unique constraints, foreign keys, and
  cascade behavior needed by runtime methods,
- behavior checks exercise the storage methods that depend on the repaired
  schema,
- no credential secrets or sensitive payloads appear in command output or
  metadata tables.

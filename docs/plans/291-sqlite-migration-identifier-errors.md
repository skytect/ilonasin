# 291 SQLite Migration Identifier Errors

## Context

`docs/ilonasin-architecture.md` treats SQLite as the daemon-owned mutable source
of truth. Plan 017 hardened migrations for historical schemas and made
migration steps idempotent, but `internal/storage/sqlite/migrations.go` still
uses `panic` in `migrationIdentifier` when a migration author passes an invalid
table or column identifier.

Those panics are not currently user-controlled because callers pass hardcoded
migration identifiers. Still, process crashes during storage initialization are
not the right failure mode for a production migration boundary. Invalid
migration definitions should return ordinary errors from `sqlite.Open` or
`Store.Migrate`, preserving transaction rollback and keeping application startup
failure controlled.

## Goal

Remove panic-based migration identifier validation and return migration errors
through the existing migration path instead.

## Scope

1. Touch only `internal/storage/sqlite` migration helper code and temporary
   storage smoke files that are removed before commit.
2. Replace `migrationIdentifier` panic behavior with error-returning validation.
3. Keep all current valid migration definitions, migration versions, schema SQL,
   transaction boundaries, and fresh/historical migration behavior unchanged.
4. Keep `addColumnIfMissing` as the safe helper used by migrations, but make it
   carry identifier validation errors into its returned `migrationStep`.
5. Do not change runtime storage methods, schema, management API, server,
   provider adapters, TUI, config, or logging.
6. Do not add permanent tests.

## Implementation

1. Change `migrationIdentifier` to return `(string, error)` or add an
   equivalent `validateMigrationIdentifier` helper.
2. Update `addColumnIfMissing` so invalid table or column names are returned as
   errors when the migration step executes.
3. Keep valid identifiers pre-normalized once per helper construction where
   practical.
4. Ensure valid migration behavior is unchanged by running normal package
   checks and daemon smokes.
5. Add a temporary in-package smoke under `internal/storage/sqlite`, run it, and
   remove it before commit. It should cover:
   - invalid empty identifier returns an error, not a panic;
   - invalid identifier with unsafe characters returns an error, not a panic;
   - valid `addColumnIfMissing` still adds a missing column and is idempotent.

## Verification

Run a temporary focused storage smoke, then remove it before commit:

- use `defer recover()` in the smoke to prove invalid identifiers do not panic;
- execute a valid migration step against an in-memory or temporary SQLite
  transaction;
- assert the target column exists after the step and the second execution is a
  no-op.

Then run:

```sh
rg -n 'panic\\(' internal/storage/sqlite
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

The `panic` search should find no live storage migration panics after temporary
smokes are removed.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health and snapshot over the management socket.
4. Run `manage` under short timeouts at narrow and wide terminal sizes.
5. Verify API, providers, usage, and logs chrome renders.
6. Remove all temporary artifacts.

## Acceptance

- Storage migrations no longer panic on invalid helper identifiers.
- Valid migrations and fresh database startup still work.
- No schema, runtime storage, server, management, provider, TUI, or config
  behavior changes.

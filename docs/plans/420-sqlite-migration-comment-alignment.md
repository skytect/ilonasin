# 420 SQLite Migration Comment Alignment

## Context

Recent architecture cleanup removed the fallback-policy control surface and live
schema names now use credential pool groups. The remaining SQLite migration code
still needs historical compatibility steps for upgrades, but one nearby comment
is misleading:

```go
// migration001, migration002, and migration003 are historical compatibility
// contracts. Future schema changes must use new migration versions.
var migration001 = []string{
```

There are no `migration002` or `migration003` base-schema arrays. Versions 2 and
3 are entries in the `migrations` list, and version 3 specifically contains old
fallback-policy compatibility work that is no longer a live architecture
contract. `docs/ilonasin-architecture.md` now describes credential pool-group
listing and metadata as operator display metadata, with serving eligibility
derived from same-provider/model eligible credentials.

## Goal

Clarify the SQLite migration comment so it describes the actual compatibility
boundary: `migration001` is the historical base schema, while later migrations
are ordinary versioned upgrade steps, some of which preserve compatibility with
older databases but are not live architecture contracts.

## Scope

1. Update only the comment immediately above `migration001` in
   `internal/storage/sqlite/migrations.go`.
2. Preserve every migration statement, version, name, order, and behavior.
3. Do not change schema, migration logic, storage queries, management JSON, TUI,
   serving behavior, routing, credentials, provider adapters, config, logging,
   or docs architecture text.
4. Do not add permanent tests.

## Verification

Run:

```sh
rg -n "migration001, migration002|migration002|migration003|historical compatibility" internal/storage/sqlite/migrations.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/storage/sqlite
go test ./...
go vet ./...
```

Finally build a temporary `ilonasin` binary and smoke `ilonasin serve` plus
bounded `ilonasin manage` runs at narrow and wide terminal widths against an
isolated temporary `ILONASIN_HOME`, then remove all temporary files.

## Non-Goals

- No SQLite schema changes.
- No migration version changes.
- No removal of historical upgrade compatibility steps.
- No changes to fallback event, credential pool group, or credential storage
  behavior.
- No permanent test files.

# 375 Remove Fallback Event Policy Column

## Context

The fallback-policy control surface has been removed. Credential fallback now
means retrying another eligible credential in the same provider/model pool.

Fallback event metadata still has a policy remnant:

- `metadata.FallbackEvent.AllowedByPolicy`;
- `fallback_events.allowed_by_policy`;
- `RecordFallbackEvent` logs `allowed`;
- `chatFallbackEvent` always sets `AllowedByPolicy: true`.

Recent fallback summaries do not read the column, and management/TUI do not
display it. Keeping it in live metadata and schema implies fallback decisions
are still policy-gated by an operator row.

## Goal

Remove stale allowed-by-policy fallback event state from live metadata and
SQLite while preserving fallback event history that is still displayed.

## Scope

1. Remove `AllowedByPolicy` from `metadata.FallbackEvent`.
2. Stop writing `allowed_by_policy` in `RecordFallbackEvent`.
3. Stop logging the stale `allowed` fallback attribute.
4. Add a new SQLite migration that rebuilds `fallback_events` without
   `allowed_by_policy`, copying all remaining columns through an explicit
   SQLite-safe table replacement:
   - create `fallback_events_new` with the current columns minus
     `allowed_by_policy`;
   - preserve the existing foreign key clauses:
     `request_metadata_id INTEGER REFERENCES request_metadata(id) ON DELETE CASCADE`,
     `from_credential_id INTEGER REFERENCES provider_credentials(id) ON DELETE SET NULL`,
     and
     `to_credential_id INTEGER REFERENCES provider_credentials(id) ON DELETE SET NULL`;
   - copy `id`, `request_metadata_id`, `occurred_at`, `provider_instance_id`,
     `model_id`, `from_credential_id`, `to_credential_id`, and `reason`;
   - drop the old `fallback_events` table;
   - rename `fallback_events_new` to `fallback_events`.
5. Keep historical migration 1 unchanged.
6. Keep fallback summary JSON, TUI fallback event logs, request fallback
   reason metadata, serving routing, credential resolution, quota handling,
   provider adapters, config, logging, and subscription usage otherwise
   unchanged.

## Verification

Run a temporary schema smoke and remove it before commit. It should prove:

- fresh migrated databases have no `fallback_events.allowed_by_policy`;
- an upgraded database with existing fallback event rows keeps those rows after
  migration;
- fresh final schema and upgraded final schema have the same remaining
  `fallback_events` column names and order;
- the copied fallback event columns still feed `RecentFallbacks`;
- rerunning migrations is idempotent.

Then run:

```sh
rg -n "AllowedByPolicy|allowed_by_policy|slog\\.Bool\\(\"allowed\"" internal
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/metadata
go test ./internal/storage/sqlite
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- No live `AllowedByPolicy` metadata field remains.
- Fully migrated SQLite schemas no longer have `fallback_events.allowed_by_policy`.
- Existing fallback event history is preserved across migration.
- Fallback event logs remain available as metadata-only retry/fallback records.

# 452 Architecture SQLite Table Inventory

## Context

Plan 450 recorded a whole-codebase review finding that
`docs/ilonasin-architecture.md` omits two live durable metadata tables from the
SQLite table inventory:

- `quota_events`;
- `subscription_usage_snapshots`.

Plan review also noted that the inventory omits another live durable table
created by migrations:

- `credential_secrets`;

These tables are created by migrations and used by runtime storage code. The
same architecture document already discusses provider credentials, fallback
policy, quota metadata, and subscription usage views, so the durable-state
boundary list should name them explicitly.

## Goal

Align the architecture's SQLite durable-state inventory with the current live
schema's durable table boundaries, without changing runtime behavior.

## Scope

1. Update only `docs/ilonasin-architecture.md` and this plan.
2. Add `credential_secrets` to the SQLite table inventory as local secret
   material storage for upstream credentials, referenced by credential metadata.
3. Add `quota_events` to the SQLite table inventory as quota/reset/cooldown
   metadata linked to request metadata where available.
4. Add `subscription_usage_snapshots` to the SQLite table inventory as
   metadata-only subscription quota snapshots for OAuth-capable accounts.
5. Do not add `credential_fallback_policies` to the active architecture
   inventory because it is historical migration compatibility and is dropped by
   migration 10.
6. Keep wording explicit that these are metadata tables and not raw prompt,
   completion, request body, response body, or provider payload storage.
7. Do not change migrations, storage queries, management DTOs, TUI rendering,
   config, provider behavior, routing, logging, or CLI behavior.
8. Do not add permanent tests.

## Verification

Run:

```sh
rg -n 'CREATE TABLE IF NOT EXISTS|DROP TABLE IF EXISTS credential_fallback_policies|credential_secrets|credential_fallback_policies|quota_events|subscription_usage_snapshots|request_metadata|stream_metrics' docs/ilonasin-architecture.md internal/storage/sqlite/migrations.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health and
snapshot over the Unix socket, running bounded `ilonasin manage` at narrow and
wide terminal widths, and cleaning up all temporary files and processes.

## Acceptance

- The active architecture names `credential_secrets`, `quota_events`, and
  `subscription_usage_snapshots` in the SQLite durable-state boundary list.
- The active architecture does not name `credential_fallback_policies` as a
  current durable-state boundary.
- The architecture remains clear that raw prompts, completions, bodies, and
  provider payloads are not normal durable SQLite tables.
- No runtime behavior changes are made.
- No permanent tests are added.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

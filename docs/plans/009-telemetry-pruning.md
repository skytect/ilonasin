# Plan 009: Telemetry Pruning

## Goal

Add explicit metadata-only telemetry pruning controls to `ilonasin manage`,
backed by narrow SQLite/service interfaces.

The architecture says telemetry defaults to "keep forever until pruned" and
that the TUI should provide pruning controls later. This slice implements the
first safe manual pruning path without touching credentials, config, model
cache, or provider adapters.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/plans/001-initial-go-scaffold.md`
- `docs/plans/002-local-api-tokens.md`
- `docs/plans/003-upstream-api-key-credentials.md`
- `docs/plans/004-nonstreaming-chat-adapters.md`
- `docs/plans/005-streaming-chat-adapters.md`
- `docs/plans/006-model-discovery-cache.md`
- `docs/plans/007-credential-fallback-health.md`
- `docs/plans/008-oauth-account-state.md`
- `AGENTS.md`

## Scope

1. Add a telemetry pruning boundary:
   - TUI receives a narrow `TelemetryPruner` interface,
   - storage implements pruning for metadata tables only,
   - server/provider/credential adapters do not receive pruning methods.
2. Implement manual pruning for metadata older than a cutoff:
   - `request_metadata.started_at < cutoff`,
   - `stream_metrics` rows attached to pruned request rows,
   - `fallback_events` attached to pruned request rows or
     `fallback_events.occurred_at < cutoff`,
   - `health_events.occurred_at < cutoff`.
   All target selection, per-table pre-counting, deletes, and commit/rollback
   must happen in one SQLite transaction. Timestamp comparisons must parse the
   stored UTC `RFC3339Nano` values instead of relying on lexicographic text
   ordering. Any error rolls back the entire prune.
3. Keep these tables out of pruning:
   - `client_tokens`,
   - `provider_credentials`,
   - `credential_secrets`,
   - `oauth_tokens`,
   - `provider_accounts`,
   - `credential_fallback_policies`,
   - `model_cache`,
   - `migrations`.
4. Add a retention summary to `manage`:
   - show telemetry retention as `keep forever until pruned`,
   - show the manual prune cutoff policy for this slice: older than 30 days,
   - show the last prune result during the current TUI session.
5. Add an interactive TUI prune command:
   - pressing `p` prunes metadata older than 30 days using the current time,
   - production uses `time.Now().UTC()`,
   - check mode uses an injected deterministic clock,
   - only SQLite metadata tables are mutated,
   - no config file mutation,
   - no prompts, completions, raw bodies, raw provider payloads, raw SSE chunks,
     bearer tokens, provider request IDs, account IDs, balances, or credit data
     are read or displayed.
6. Extend `manage --check`:
   - seed old and recent request/stream/health/fallback metadata in an isolated
     DB,
   - trigger the same TUI prune update path,
   - assert old metadata is removed,
   - assert a row exactly at the cutoff remains because pruning uses strict
     `< cutoff`,
   - assert recent metadata remains,
   - assert credentials, OAuth account state, model cache, and config are not
     mutated,
   - assert selected-home DB metadata and config snapshots are unchanged.
7. Extend `serve --check` only enough to verify current serving behavior still
   passes after pruning interfaces are added.

## Out of Scope

- Scheduled pruning.
- User-configurable retention durations.
- Pruning credentials, OAuth tokens, model cache, migrations, or config.
- Capturing or pruning raw prompt/body payloads, because such tables must not
  exist.
- Admin HTTP/socket pruning APIs.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` remains a compile/package check only.
- TUI may mutate SQLite but must not mutate `config.toml`.
- Pruning must use structured SQL against known metadata tables, not stringly
  table-name input.
- Request pruning must rely on SQLite foreign keys for child rows where schema
  defines cascade, and must explicitly delete orphan/time-based fallback and
  health metadata.
- Prune result count semantics are pre-delete target counts inside the same
  transaction:
  - `Requests` is the number of `request_metadata` rows with
    `started_at < cutoff`,
  - `Streams` is the number of `stream_metrics` rows attached to those request
    IDs,
  - `Fallbacks` is the number of distinct `fallback_events` rows attached to
    those request IDs or with `occurred_at < cutoff`,
  - `Health` is the number of `health_events` rows with `occurred_at < cutoff`.
  The returned counts must match the rows actually targeted/deleted.
- The prune result may include counts per table, but must not include row
  contents, model IDs, labels, errors, request IDs, account IDs, prompts,
  completions, raw bodies, raw provider payloads, tool data, token material, or
  balance/credit values.
- The TUI must show only counts and cutoff timestamps for pruning.
- `manage --check` must not leave check-created metadata in the selected home
  DB.

## Proposed Package Changes

```text
internal/metadata/
  metadata.go      # prune result type
internal/storage/sqlite/
  db.go            # prune metadata method
internal/tui/
  tui.go           # pruning view and key handling
internal/app/
  app.go           # isolated pruning smoke check
```

Interface shape:

```go
type TelemetryPruner interface {
    PruneTelemetryBefore(ctx context.Context, cutoff time.Time) (metadata.PruneResult, error)
}
```

The TUI owns the 30-day manual cutoff policy for this slice. Storage only
implements "before cutoff" semantics.

## Verification

Run:

```text
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Smoke checks must prove:

- no permanent test files exist,
- old request metadata is removed by the TUI prune path,
- old stream metrics attached to pruned requests are removed,
- recent fallback events attached to old requests are removed by request
  pruning/cascade,
- old fallback events attached to recent requests are removed by
  `occurred_at < cutoff`,
- old fallback events with `request_metadata_id NULL` are removed,
- old health events are removed,
- recent request, stream, fallback, and health metadata remains,
- a request, stream, fallback, and health row exactly at the cutoff remains,
- forbidden markers are seeded in both old and recent metadata; old marker rows
  are pruned, recent marker rows remain in SQLite, and prune output still shows
  only counts/cutoff metadata,
- credential, OAuth account, model cache, and config state remains unchanged,
- protected-state before/after snapshots in the isolated pruning DB and the
  selected home DB cover `client_tokens`,
  `provider_credentials`, `credential_secrets` using hashes only,
  `oauth_tokens`, `provider_accounts`, `credential_fallback_policies`,
  `model_cache`, `migrations`, and config bytes/mtime,
- old/recent telemetry is seeded with unsafe model IDs, labels, error classes,
  fallback reasons, prompt/body/provider-payload/secret/account/balance/credit
  markers, and prune result, TUI render, and failure messages show only counts
  and cutoff timestamps,
- selected-home DB and config snapshots remain unchanged across
  `manage --check`,
- check output contains only safe prune counts/cutoff metadata and no raw
  prompt/body/provider/secret/account markers.

## Review Questions

1. Is a fixed manual "older than 30 days" pruning control acceptable for the
   first pruning slice?
2. Are the pruning boundaries narrow enough to avoid deleting credential or
   model state?
3. Are the smoke checks strong enough without permanent tests?
4. Does the result metadata avoid leaking forbidden row contents?

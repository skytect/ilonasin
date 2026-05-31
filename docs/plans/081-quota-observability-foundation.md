# 081 Quota Observability Foundation

## Context

The architecture requires metadata-only usage, health, and account status views
in both the daemon and TUI. Existing code records request usage and health
`retry_after`, but quota state is still implicit. A user looking at
`ilonasin manage` cannot distinguish ordinary upstream failures from quota or
rate-limit pressure without reading recent request rows manually.

The previous Codex account-pooling slice deliberately refused account fallback
on 429 and `rate_limit_exceeded`. That is correct, but it leaves the next
architecture step unresolved: quota pressure should become visible state before
any future quota-pooling policy is designed.

This slice is the storage, daemon snapshot, and TUI foundation for quota
tracking. It must not make routing, pooling, or fallback decisions.

## Goal

Add first-class, metadata-only quota observations that are recorded by the
daemon, exposed through the management snapshot, and rendered by the TUI.

After this slice, HTTP 429 and normalized `rate_limit_exceeded` outcomes from
chat and streaming chat are visible as quota observations with provider,
credential, model, status, normalized error class, retry-after/reset timing
when already available, and safe aggregate counts. This makes quota pressure
auditable without storing balances, credits, provider request IDs, raw
provider payloads, prompts, completions, raw SSE chunks, full bearer tokens, or
full account IDs.

## Architecture Inputs

- `AGENTS.md`
- all Markdown files under `docs/**`
- especially `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/plans/019-usage-metadata-ledger.md`
- `docs/plans/020-health-retry-after.md`
- `docs/plans/079-daemon-management-snapshot.md`
- `docs/plans/080-codex-account-pooling.md`

## Scope

1. Add typed quota metadata.
   - Add `metadata.QuotaObservation` for daemon-side recording.
   - Add `metadata.QuotaSummary` for recent safe display rows.
   - Fields should be limited to safe scalar metadata:
     - observed time,
     - provider instance ID,
     - provider model ID,
     - local credential ID and safe credential label for summaries,
     - source, such as `chat` or `stream`,
     - HTTP status,
     - normalized error class,
     - retry-after timestamp when already known,
     - reset timestamp when it can be derived from retry-after,
     - observation count for grouped summaries.
   - Do not add raw provider JSON, raw headers, provider request IDs, account
     IDs, balances, credits, active-limit IDs, prompts, completions, tool data,
     or body text.
2. Add SQLite quota storage.
   - Add a `quota_events` table by migration.
   - Keep optional foreign keys to `request_metadata` and
     `provider_credentials`.
   - Use `ON DELETE CASCADE` for request-linked rows and `ON DELETE SET NULL`
     for credentials.
   - Add a compact reader returning latest/grouped quota summaries for the TUI.
   - Include `quota_events` in migration smoke checks and telemetry pruning.
   - Extend the typed prune result and TUI prune summary with a quota-event
     count so quota pruning is visible and smokeable.
3. Record quota observations in the server.
   - Record an observation after request metadata is persisted for
     non-streaming chat when:
     - upstream status is `429`,
     - upstream status is `402`,
     - normalized error class is `rate_limit_exceeded`,
     - or normalized error class is `insufficient_quota`.
   - Record an observation for streaming chat using the same rules after the
     stream summary/request metadata is available.
   - Normalize safe provider streaming quota errors before recording:
     - OpenRouter HTTP-200 SSE error frames that indicate rate-limit or
       payment-limit pressure should become a quota-class error rather than a
       generic `upstream_stream_error`,
     - no raw SSE event data, provider request IDs, balances, or credit values
       should be stored.
   - Use only already-normalized status, error class, retry-after, credential,
     provider instance, and model metadata.
   - Do not record quota events for successful requests or unrelated 5xx,
     auth, invalid-body, timeout, or cancellation cases.
   - If an availability fallback occurs first and the final attempt is quota
     limited, record the quota event for the final attempt while preserving the
     existing fallback metadata. The quota event itself must not trigger another
     fallback.
   - Do not change retry, fallback, pooling, or provider request behavior.
4. Expose quota state through the daemon management snapshot.
   - Extend the observability reader interface with
     `QuotaByProvider(ctx)` or an equivalent read-only method.
   - Add quota rows to `ManagementSnapshotResponse`.
   - Sanitize quota snapshot fields with the existing snapshot-safe string
     rules.
   - Keep the local management API read-only for this slice.
5. Render quota state in the TUI.
   - Add a `Quota` section near usage/health.
   - Show safe rows such as provider, model, status, error class, credential,
     retry-after/reset time, and count.
   - If no observations exist, render `No quota metadata.`
   - Do not show provider balances, credits, full account IDs, raw request IDs,
     raw headers, raw upstream bodies, or bearer tokens.
6. Extend smoke coverage without permanent tests.
   - `serve --check` should exercise at least one non-streaming quota
     observation and one streaming quota observation.
   - Existing fake Codex `rate_limit_exceeded` coverage should prove that a
     quota event is recorded while account fallback is not recorded.
   - API-key provider 429 coverage should also record quota metadata without
     leaking raw body markers, and must explicitly assert that no second
     credential is called and no fallback event is recorded for that 429.
   - Add a safe HTTP-200 SSE quota-error smoke for OpenRouter-style streaming
     errors.
   - Add a 402 or normalized `insufficient_quota` smoke row to prove
     payment-limit pressure is observable without querying balances or credits.
   - `manage --check` should render quota rows from both direct observability
     and daemon snapshot paths.
   - Smoke leak checks should include `quota_events`.

## Non-Goals

- Do not implement quota pooling, quota-aware routing, round-robin, or account
  selection.
- Do not switch accounts on 429.
- Do not query provider billing, balance, usage, credits, account settings, or
  key telemetry endpoints.
- Do not parse or persist raw Codex `codex.rate_limits` SSE payloads in this
  slice.
- Do not store provider active-limit IDs, full request IDs, full account IDs,
  balances, credits, or raw headers.
- Do not store actual balance or credit amounts even when the provider returns
  a payment-limit response.
- Do not estimate remaining quota from pricing tables.
- Do not add permanent tests.
- Do not push.

## Design Constraints

1. Quota tracking is observability only in this slice.
2. Server recording must depend on normalized local result metadata, not raw
   provider payloads.
3. Storage must not import provider, server, TUI, config, or HTTP packages.
4. Provider adapters must not import SQLite, management, TUI, or storage types.
5. The TUI must not mutate `config.toml`.
6. The daemon management API remains local-only and must not be exposed through
   the OpenAI-compatible API surface.
7. All quota labels shown to users must pass the existing safe-display and
   snapshot-sanitization rules.
8. Existing request, stream, health, fallback, and usage metadata behavior must
   remain compatible with older rows.

## Implementation Plan

1. Add metadata types and storage schema.
   - Add `QuotaObservation` and `QuotaSummary` to `internal/metadata`.
   - Add migration `quota_events`.
   - Add `RecordQuotaObservation` and `QuotaByProvider` methods to the SQLite
     store.
   - Include quota rows in pruning, typed prune counts, TUI prune summaries,
     and migration smoke checks.
2. Add server recording.
   - Extend the server metadata interface with quota recording.
   - Add small helpers that classify a chat/stream attempt as quota-related.
   - Record quota after request metadata exists, reusing request metadata ID.
   - Normalize safe streaming quota errors before the server sees the stream
     summary.
   - Keep health recording unchanged.
3. Add management and TUI read surfaces.
   - Extend management observability interfaces and snapshot DTOs.
   - Convert metadata quota rows into sanitized management rows.
   - Add TUI reload/snapshot wiring and a `Quota` display section.
4. Extend smoke checks.
   - Add quota metadata assertions to serve check.
   - Add TUI quota summary assertions to manage check.
   - Ensure the fake upstream raw quota markers do not appear in SQLite,
     snapshots, CLI output, or TUI output.

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

- no permanent tests exist,
- compile/package, vet, build, `serve --check`, `manage --check`, and diff
  whitespace checks pass,
- quota observations are recorded for HTTP 429 and `rate_limit_exceeded`,
- quota observations are recorded for HTTP 402 or normalized
  `insufficient_quota`,
- quota observations are not recorded for unrelated failures or successes,
- API-key and Codex quota observations do not trigger fallback or account
  cycling,
- quota metadata is visible through the management snapshot and TUI,
- quota pruning is covered with request/health/fallback pruning and reports a
  quota count,
- no prompts, completions, request bodies, response bodies, raw provider
  payloads, raw SSE chunks, tool arguments, tool results, full bearer tokens,
  provider request IDs, full account IDs, balances, credits, or raw quota
  payload markers appear in SQLite metadata, snapshots, TUI output, CLI output,
  logs, or local error envelopes.

## Review Questions

1. Is this slice narrow enough by keeping quota tracking read-only and avoiding
   quota-aware routing?
2. Is the proposed `quota_events` schema safe and useful enough for future
   quota pooling without storing sensitive provider payloads?
3. Should model discovery quota events be deferred until a request-linked
   representation exists, or should they be recorded as unlinked quota events
   in this slice?

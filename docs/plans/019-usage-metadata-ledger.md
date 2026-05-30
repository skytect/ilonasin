# Plan 019: Usage Metadata Ledger

## Goal

Complete the typed metadata path for safe usage fields that already exist in
the SQLite schema.

The architecture allows metadata-only telemetry for fallback reason, cache hit
and cache write counts, and estimated or actual cost. The schema already has
`fallback_reason`, `cache_write_tokens`, and `cost_microunits`, but the typed
request metadata path does not carry or insert them. This slice wires those
fields through the existing boundaries without storing request bodies,
response bodies, raw provider payloads, raw SSE chunks, balances, credits,
provider request IDs, or account identifiers.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `018`

## Scope

1. Extend typed metadata only:
   - add `FallbackReason`, `CacheWriteTokens`, and `CostMicrounits` to
     `metadata.Request`,
   - add safe display/summary fields only where the TUI already shows metadata
     aggregates,
   - do not add raw provider usage JSON, balances, credits, provider IDs,
     account IDs, request IDs, route traces, or generation IDs.
2. Persist the existing schema columns:
   - `RecordRequestMetadata` inserts `fallback_reason`,
     `cache_write_tokens`, and `cost_microunits`,
   - recent request and usage summary readers can return these safe values,
   - existing rows default to zero or empty string and remain valid.
3. Normalize safe cache telemetry:
   - keep `CacheHitTokens` as prompt cache-hit tokens,
   - add `CacheWriteTokens` only for explicitly documented cache-write token
     fields when they are safe scalar counts,
   - DeepSeek `prompt_cache_hit_tokens` maps to cache hit tokens,
   - DeepSeek `prompt_cache_miss_tokens` is not persisted in this slice because
     cache miss is not the same concept as cache write and the current schema
     has no cache-miss column,
   - OpenRouter `prompt_tokens_details.cached_tokens` maps to cache hit tokens,
   - OpenRouter `prompt_tokens_details.cache_write_tokens` maps to cache write
     tokens,
   - Codex `input_tokens_details.cached_tokens` continues to map to cache hit
     tokens; no Codex cache write field is added unless present as a safe
     scalar in the existing typed usage object.
4. Normalize safe cost telemetry conservatively:
   - add a typed `CostMicrounits` field to `openai.Usage`,
   - this slice does not extract cost from provider responses because the local
     provider docs do not define an exact stable JSON path and unit for
     request-level cost,
   - the current provider cost extraction allowlist is intentionally empty,
   - if a future slice adds cost extraction, it must name exact JSON paths,
     provider applicability, unit semantics, non-negative/finite validation,
     overflow behavior, and deterministic rounding,
   - until that future allowlist exists, provider responses record zero cost,
   - direct typed metadata callers can persist `CostMicrounits` when the value
     is already a safe integer microunit amount,
   - never query billing endpoints such as `/credits`, `/key`, `/activity`,
     Codex usage/settings endpoints, or balance APIs in this slice.
5. Record fallback reason:
   - when API-key credential fallback occurs, set request
     `fallback_reason` from the first recorded fallback event reason,
   - for non-fallback requests, keep the field empty,
   - do not store raw upstream error bodies or detailed provider failure text.
6. Preserve boundaries:
   - provider adapters extract safe scalar usage fields only,
   - server decides fallback reason from local fallback events,
   - storage persists typed metadata only,
   - TUI reads typed summaries only,
   - config, credentials, local API auth, provider adapters, routing, HTTP
     transport, storage, and TUI remain separate.
7. Extend smoke checks without permanent tests:
   - fake DeepSeek/OpenRouter responses include safe cache hit/write scalar
     fields,
   - fake responses include unknown nested cost/cache markers to prove
     unrecognized provider usage fields are ignored, not stored or printed,
   - `serve --check` asserts the metadata ledger stores hit/write/cost values,
   - fallback smoke asserts `fallback_reason` is stored for fallback requests,
   - TUI/summary smoke asserts usage output can include cache write and cost
     totals without leaking raw payload markers,
   - both non-streaming and streaming usage paths are covered,
   - privacy smoke asserts prompts, completions, raw bodies, raw provider
     payloads, balances, credits, request IDs, account IDs, and bearer tokens
     are not stored or printed.

## Out of Scope

- New SQLite columns or migrations.
- Raw `provider_usage` JSON.
- Persisting DeepSeek cache miss tokens.
- Provider response cost extraction without exact documented paths and units.
- Billing endpoint calls, credit/balance polling, account usage pages, or key
  telemetry endpoints.
- Pricing estimation from model price tables.
- Provider route tracing, BYOK usage, workspace IDs, concurrency buckets,
  response-cache headers, or detailed cache status.
- Changing fallback policy semantics.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- Do not push.
- Storage must not perform HTTP.
- Provider adapters must not import SQLite, TUI, config loaders, or credential
  storage.
- TUI must not mutate `config.toml`.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens, full provider request IDs, full account IDs, balances, or credits.
- Treat unrecognized provider usage fields as absent rather than storing them.

## Proposed Package Changes

```text
internal/openai/
  types.go       # typed usage extraction for cache write and cost
internal/provider/
  http_chat.go   # Codex typed usage extraction, if safe scalar present
internal/metadata/
  metadata.go    # request and summary safe metadata fields
internal/storage/sqlite/
  db.go          # insert/read/aggregate existing columns
internal/server/
  server.go      # set fallback_reason from local fallback events
internal/tui/
  tui.go         # display safe cache write/cost totals if present
internal/app/
  app.go         # serve/manage smoke assertions
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

- no permanent tests exist,
- compile/package, vet, build, `serve --check`, `manage --check`, and diff
  whitespace checks pass,
- request metadata stores fallback reason, cache hit tokens, cache write
  tokens, and direct typed cost microunits when safe typed values are
  available,
- provider response cost extraction remains zero because no allowlisted path
  exists in this slice,
- usage summaries include safe cache hit, cache write, and cost totals,
- fallback reason is derived from local fallback events only for both
  non-streaming and streaming fallback paths,
- no raw prompts, completions, request/response bodies, provider payloads,
  SSE chunks, bearer tokens, provider request IDs, account IDs, balances, or
  credits appear in SQLite metadata, TUI output, CLI output, or local errors.

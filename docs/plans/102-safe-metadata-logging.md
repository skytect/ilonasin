# 102 Safe Metadata Logging

## Context

The request ledger already records provider/model IDs, credential ID, status,
error class, retry/fallback counts, token counts, cache counts, cost, total
latency, TTFT, and one output TPS value. It does not yet record several safe
operational dimensions needed for Codex compatibility and performance analysis,
including service tier and separate TPS calculations.

The architecture allows metadata-only telemetry but forbids prompts,
completions, request/response bodies, raw provider payloads, tool arguments,
tool results, raw SSE chunks, full bearer tokens, provider request IDs, and full
account IDs.

## Goal

Make request metadata more exhaustive while staying metadata-only. The operator
should be able to answer what route shape was used, what safe Codex options were
requested, how many attempts happened, and how performance behaved without
inspecting content.

## Safe Fields To Add

1. Route and request shape.
   - `endpoint`: `chat_completions` or `responses`.
   - `stream`: boolean.
   - `provider_type`: safe provider type copied from config.
   - `message_count`: count only, no message text.
   - `tool_count`: count only, no tool names, arguments, or schemas.
   - `image_count`: count only, no image data or URLs.
2. Safe Codex/provider options.
   - `requested_service_tier`: normalized safe enum from the client request.
   - `effective_service_tier`: normalized safe enum sent upstream when it
     differs from the request, for example Codex `fast` mapping to `priority`.
   - `reasoning_effort`: normalized safe enum such as `minimal`, `low`,
     `medium`, `high`, or `xhigh`.
   - `reasoning_summary`: normalized safe enum when present.
   - `reasoning_max_tokens`: count only, for OpenRouter reasoning token caps.
   - `reasoning_enabled`: boolean only, for OpenRouter reasoning controls.
   - `reasoning_exclude`: boolean only, for OpenRouter reasoning controls.
   - `thinking_type`: normalized safe enum for DeepSeek thinking mode.
   - `max_output_tokens`: requested output token cap from `max_tokens` or
     `max_completion_tokens`.
   - Do not store `prompt_cache_key`, `client_metadata`, `metadata`, `user`,
     `session_id`, prompt text, tool schemas, or tool arguments.
   - Do not store OpenRouter provider routing objects, transforms, or
     performance preference request values.
3. Retry and attempt accounting.
   - `auth_retry_count`: OAuth refresh/auth recovery retries.
   - `attempt_count`: total upstream attempts including final attempt.
   - Preserve existing `retry_count`, `fallback_count`, and `fallback_reason`.
4. Performance metrics.
   - Preserve `total_latency_ms`.
   - Preserve `time_to_first_token_ms`.
   - Add `upstream_latency_ms` from the final provider attempt. Extend stream
     summaries so streaming and nonstreaming requests use the same semantics.
   - Preserve prompt, completion, total, reasoning, cache hit, cache write, and
     cost counters in request and usage summaries.
   - Add derived `reasoning_token_rate`, `cache_hit_rate`,
     `cache_miss_tokens`, `cache_miss_rate`, and `cache_write_rate` from token
     counters only.
   - Preserve current total output TPS semantics as
     `output_tokens_per_second_total`, counting full latency. Keep the existing
     `output_tokens_per_second` field and JSON name as a compatibility alias for
     this value.
   - Add `output_tokens_per_second_after_ttft`, counting
     `latency - ttft` when TTFT and completion tokens are available.
   - Preserve stream chunk count and stream completion status.

## Scope

1. Extend metadata structs.
   - Add fields to `metadata.Request`, `RequestSummary`, and `LatencySummary`.
   - Keep names explicit enough to avoid ambiguity between total TPS and
     post-TTFT TPS.
2. Extend SQLite schema and migrations.
   - Add a migration with `addColumnIfMissing` for new `request_metadata`
     columns.
   - Keep default zero or empty values for existing databases where the value
     cannot be inferred.
   - Preserve the existing `output_tokens_per_second` column and treat it as
     total-latency TPS for legacy readers. New summary JSON should keep this
     field and add explicit total and post-TTFT fields.
   - Backfill or query-coalesce `stream` from `stream_metrics` so existing
     streaming rows are not mislabeled as nonstreaming.
   - Update inserts and summary queries.
3. Populate metadata in server routes.
   - Build safe request-shape metadata after decoding and validation.
   - Record endpoint and stream for both Chat Completions and Responses routes.
   - Extract service tier and reasoning fields only through allowlisted safe
     option parsing.
   - Distinguish requested service tier from effective upstream service tier.
   - Extend provider result and stream summary structs with safe effective
     option metadata so adapter-only mappings, including Codex service tier
     mapping, can be recorded without duplicating provider-specific logic in the
     server.
   - Extract DeepSeek thinking type and OpenRouter reasoning controls only as
     safe enum, boolean, or numeric metadata. Do not extract provider routing or
     performance preference objects.
   - Record auth retry count and attempt count for nonstreaming and streaming
     execution.
   - Calculate both TPS metrics from safe counters and timings.
4. Keep provider adapter logs safe and useful.
   - Include safe option and performance metadata in structured logs where the
     data is already available and not content-derived.
   - Do not log request bodies, response bodies, tool arguments, prompt cache
     keys, client metadata, user/session identifiers, bearer tokens, full
     account IDs, or raw provider payloads.
5. Surface metadata through management and TUI.
   - Include new request and latency summary fields in management snapshots.
   - Update the existing TUI text summaries to distinguish total TPS from
     post-TTFT TPS and show safe route shape fields.

## Non-Goals

- Do not query provider balances, billing, account limits, or plan settings.
- Do not add body capture, prompt capture, or unsafe debug mode.
- Do not change provider routing, fallback policy, OAuth behavior, quota pooling,
  model discovery, or TUI layout.
- Do not add permanent test files or check harnesses.
- Do not push.

## Acceptance

- Request metadata rows record endpoint, stream, provider type, service tier,
  reasoning effort, DeepSeek thinking type, OpenRouter reasoning controls,
  attempt/auth retry counts, message/tool/image counts, upstream latency, total
  TPS, post-TTFT TPS, token count breakdowns, and derived cache/reasoning rates
  when available, including cache hit, cache miss, and cache write rates.
- Existing rows and existing databases migrate cleanly. Legacy
  `output_tokens_per_second` remains populated and is treated as total TPS;
  legacy streaming rows derive stream status from `stream_metrics`.
- Management snapshots and TUI summaries expose only safe metadata, including
  the compatibility TPS field and the explicit total/post-TTFT fields.
- Structured logs gain safe operational fields but no forbidden payloads or
  secret-shaped values.
- A disposable direct smoke records at least one request and inspects SQLite/log
  output for the new fields.
- The disposable smoke inspects management snapshot JSON and TUI output for the
  new fields and verifies that forbidden markers are absent.
- Privacy scan over disposable SQLite/log output finds no prompts, completions,
  request bodies, response bodies, raw provider payloads, tool arguments, raw
  tool results, raw SSE chunks, full bearer tokens, full provider request IDs,
  full account IDs, prompt cache keys, client metadata, OpenRouter provider
  routing objects, performance preference request values, `user`, or
  `session_id`.
- `find . -name '*_test.go' -type f -print` confirms no permanent tests were
  added.
- `go test ./...` passes as a compile/package check.
- `go vet ./...` passes.
- A fresh binary builds.
- Direct short-lived `ilonasin serve` and `ilonasin manage` smokes run against a
  disposable home.

# Codex Compatibility Audit

Date: 2026-07-13

Audited source base: `HEAD` `d155dd9` on the current branch, with branch
changes reviewed from `origin/main..HEAD`. This document refresh is separate
from the source evidence.

This checklist is evidence for compatibility decisions, not a runtime gate.
It must not become an approval switch, permission system, or permanent test
framework.

## Current Status

The stale Plan 529 blocker is no longer current. Current source no longer treats
Codex CLI boolean `strict` as a blocking unsupported field for the implemented
bounded function-tool paths, and native Responses terminal errors are not
swallowed: completed, failed, incomplete, and error events affect stream
summary/error metadata.

No fresh live Codex CLI switch smoke was run for this documentation refresh.
Items marked `SOURCE-READY-NOT-LIVE-VERIFIED` are implemented in current source
but still need a live Codex CLI/provider run before claiming runtime
compatibility.

## Concise Checklist

| Area | Status | Evidence |
| --- | --- | --- |
| Dynamic Codex client version and matching outbound User-Agent | SOURCE-READY-NOT-LIVE-VERIFIED | `internal/provider/codex_version.go`: `CachedCodexClientVersionResolver`, `HTTPChatAdapter.codexClientVersion`; `internal/provider/codex_headers.go`: `addCodexRequestHeaders`, `codexUserAgent`; `internal/provider/http_models.go`: `modelsURL` adds `client_version`. |
| Model metadata exact reasoning/default/max context/service tiers/modalities | SOURCE-READY-NOT-LIVE-VERIFIED | `internal/provider/http_models.go`: `normalizeCodexModels`, `codexCapabilityFlags`, `codexReasoningLevels`, `codexServiceTiers`, `codexInputModalities`; `internal/openai/models_response.go`: `ModelsResponseFromMetadata`, `codexModelInfoFromMetadata`; `internal/metadata/model_cache.go`: normalized persisted fields. |
| Unverified Codex metadata omitted | PASS | `internal/provider/http_models.go`: Codex metadata is copied only from upstream model fields; unknown service tiers/modalities are dropped by `safeCodexServiceTierID` and `codexInputModalities`. |
| Translated Chat output caps explicitly reject | PASS | `internal/provider/codex_responses_request.go`: `marshalCodexResponsesRequest` rejects Chat `max_tokens` and `max_completion_tokens` for Codex translation. |
| Native Responses `max_output_tokens` pass-through boundary | SOURCE-READY-NOT-LIVE-VERIFIED | `internal/server/responses_route.go`: native Codex providers use `DecodeNativeResponsesEnvelope` only for routing metadata, then pass the raw body to `StreamResponses`; `internal/provider/codex_responses_relay.go`: native relay preserves raw body fields and forces only model/stream/store plus narrow normalizations. This remains a deliberate boundary, not Chat translation. |
| Terminal completed/failed/incomplete/error events | PASS | `internal/provider/codex_responses_parse.go`: handles `response.completed`, `response.failed`, `response.incomplete`; `internal/provider/codex_responses_relay.go`: `handleCodexNativeResponsesData` maps `response.completed`, `response.failed`, `response.incomplete`, and `error` into summary status/error. |
| Bounded tools `strict` | SOURCE-READY-NOT-LIVE-VERIFIED | `internal/openai/responses_tools.go`: Codex-preserved function tools accept boolean `strict`; `internal/provider/codex_responses_request.go`: `codexResponsesTools` forwards boolean function `strict`; `internal/provider/codex_responses_relay.go`: `codexNormalizeNativeTools` removes only null `strict`; broader hosted/deferred/namespaced/MCP families remain separate work. |
| Root and `/v1` base routes | SOURCE-READY-NOT-LIVE-VERIFIED | `internal/server/handler.go` and `internal/server/responses_route.go` route `/responses` and `/v1/responses`; `internal/server/models.go` handles root and `/v1` model discovery. Earlier Plan 529 live root/`/v1` discovery passed, but no new live run was made here. |
| Auth, quota, retry, and server errors | PASS | `internal/server/auth.go`, `internal/server/chat_nonstream.go`, `internal/server/chat_stream.go`, `internal/server/metadata_quota.go`, `internal/provider/codex_responses.go`, `internal/provider/codex_responses_stream.go`, `internal/provider/codex_responses_relay.go`. |
| Privacy and metadata-only default | PASS | `internal/provider/codex_responses_request.go` sends `store:false`; `internal/provider/io_logging.go`, `internal/logging/io.go`, `internal/server/metadata.go`, and `docs/ilonasin-architecture.md` keep body/tool/SSE storage behind explicit IO logging. |
| Code Mode, namespaced tools, MCP, shell, tool-search, broader tool families | DEFERRED | Source supports selected function/custom/tool-search/web-search/image-generation and namespace declaration shapes in `internal/openai/responses.go`, `internal/openai/responses_tools.go`, and `internal/provider/codex_responses_parse.go`, but broad Codex tool-family parity is not live-proven. |
| Responses Lite | DEFERRED | Keep deferred until upstream request semantics are identified and verified. No current live evidence is recorded here. |
| Sol/Terra/Luna discovery aliases | OUT-OF-SCOPE | Do not invent aliases. Add only if a live upstream model source exposes those names. Current source copies upstream model slugs exactly. |

## Current Implementation Notes

- Codex native Responses preserves the implemented request boundary and relays
  provider-native SSE. Unsupported transcript/output families must fail locally
  instead of being lossy-converted.
- Chat-to-Codex translation is intentionally narrower than native Responses.
  It rejects unverified Chat output caps rather than silently dropping caller
  intent.
- Model discovery currently records provider model IDs returned by upstream.
  The served model is also captured from Codex response headers where available.
- `strict` is implemented only for the bounded function-tool surfaces above.
  This does not imply hosted, deferred, namespaced, MCP, shell, Code Mode, or
  broader custom tool parity.

## Compatibility Work Remaining

- Run a fresh isolated live Codex CLI smoke for root and `/v1` base URLs.
- Rerun Codex CLI through Codex, DeepSeek, and OpenRouter with the current CLI.
- Exercise workspace edit, images, service tiers, reasoning efforts, auth
  expiry/refresh, quota `429`, retryable `5xx`, and privacy scans.
- Audit Code Mode, namespaced tools, MCP, shell, tool-search, and Responses Lite
  only after upstream evidence identifies the exact live shapes.

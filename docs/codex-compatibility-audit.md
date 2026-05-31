# Codex Compatibility Audit

Date: 2026-06-01

Audited worktree commit: `9ea0ebf`

Current implementation note: plans 092 through 094 changed several findings
below. The original live audit remains useful historical evidence, but local
Responses now has stateless routes, explicit `input_image` decoding, function
tool definitions, function-call SSE output, and function-call-output follow-up
input in direct smoke coverage. These fixes still need a fresh full live Codex
switch-gate smoke before declaring normal Codex use ready.

Codex CLI version: `codex-cli 0.135.0`

## Summary

`ilonasin serve` is not ready as the default backend for normal Codex coding
agent use.

Text-only `codex exec` traffic can reach local Responses routes through both
root and `/v1` provider base URLs with the real credentials configured in
`~/.ilonasin`. Codex CLI text also works through the real DeepSeek and
OpenRouter provider instances when they are addressed through ilonasin.

The historical blocker was normal agent behavior: tool definitions,
function-call output, function-call-output follow-up input, and explicit
`input_image` were not supported at audited commit `9ea0ebf`. Plans 092 through
094 implemented direct local support for those families. Remaining switch risk
is now broader Codex parity: fresh full live smokes, hosted or deferred tool
families outside local function tools, developer/system ordering, reasoning
effort, service tier behavior, retry UX, and privacy scans.

Do not switch normal Codex use to `ilonasin` yet. Text-only and direct local
function-tool smokes are useful, but the full switch gate below is not complete.

## Safety

The audit used the real Codex OAuth, DeepSeek API-key, and OpenRouter API-key
credentials already configured in `~/.ilonasin`. It used a temporary
`CODEX_HOME`, temporary server log and cache directories, and fresh local client
tokens created through the management API. The audit tokens were disabled after
the smoke.

The smoke did not copy the user's real `~/.ilonasin`, Codex auth state, OAuth
token state, logs, cache, SQLite files, WAL/SHM files, or account state into a
temporary home. It pointed the worktree server at the existing real SQLite
database in place.

Raw terminal output, raw JSONL, raw HTTP bodies, provider payloads, prompts,
completions, image bytes, tool arguments, tool results, bearer tokens, account
IDs, and request IDs were not kept in this report. Probe captures were reduced
to allowlisted structural fields and then removed. Earlier fake-upstream
sentinel scans passed for the checked metadata and health tables.

Because the live smoke intentionally used the real SQLite database in place, it
left durable metadata-only audit rows behind: disabled local client token rows,
request metadata rows, and health event rows. Those rows contain provider/model
IDs, HTTP status, normalized error classes, retry/fallback counts, usage
counters, latency, and health classes. They do not include raw prompts,
completions, request bodies, response bodies, tool arguments, tool results,
bearer tokens, account IDs, or request IDs.

## Source Findings

Current Codex custom providers use the Responses API. For non-Azure custom
providers, Codex builds requests with `store: false`, `stream: true`, optional
`reasoning`, optional `text`, optional `service_tier`, `prompt_cache_key`,
`client_metadata`, and tool metadata.

Codex request input variants include message input, function-call output, MCP
tool-call output, custom tool-call output, and tool-search output. Content
items include text and image variants. This means local Responses compatibility
must handle both model input and agent tool-loop state, not just one text turn.

Direct recorder probes showed Codex includes tool definitions even for simple
tasks. A normal local Responses route cannot drop those tools and still be a
compatible coding-agent backend.

## Probe Matrix

| Area | Path | Result | Evidence | Switching impact | Next fix |
| --- | --- | --- | --- | --- | --- |
| Root base URL text | `POST /responses` | Pass | Live `codex exec` exited 0 against the real Codex provider instance. Local metadata recorded one 200 row. | Text-only root-base use works. | Keep covered in smokes. |
| `/v1` base URL text | `POST /v1/responses` | Pass | Live `codex exec` exited 0 against the real Codex provider instance. Local metadata recorded one 200 row. | Text-only `/v1` use works. | Keep covered in smokes. |
| Model discovery | `GET /models`, `GET /v1/models` | Pass for current smoke | Codex reached model discovery for both base URL variants without blocking text probes. | Good enough for text smoke, but not enough for richer Codex metadata. | Add Codex-aware model metadata when implementing image and service-tier support. |
| Developer/system instructions | Responses text route | Partial | Source and decoder review show developer messages are accepted and translated to internal system messages, but the live smoke did not isolate ordering or behavior. | Needs a targeted compatibility probe before switching. | Add a deterministic developer/system smoke. |
| Codex CLI through DeepSeek | `POST /v1/responses` | Pass | Live `codex exec` against `pragnition-deepseek/deepseek-v4-flash` exited 0 and recorded one 200 row. | Basic cross-provider text works. | Keep covered in smokes. |
| Codex CLI through OpenRouter | `POST /v1/responses` | Pass | Live `codex exec` against `pragnition-openrouter/openai/gpt-5.1-chat` exited 0 and recorded one 200 row. | Basic cross-provider text works. | Keep covered in smokes. |
| Reasoning minimal | Codex CLI config | Fail | Live `model_reasoning_effort="minimal"` timed out after repeated 502 `upstream_http_error` rows. | Blocking for reasoning-effort compatibility. | Inspect Codex option mapping and upstream request shape. |
| Reasoning high | Codex CLI config | Partial | Live `model_reasoning_effort="high"` exited 0 with one 200 row. Direct local Codex reasoning API probes still returned 502. | Inconsistent, not enough for switching. | Add deterministic option-shape smoke and fix failing effort values. |
| Fast or priority service tier | Codex CLI config | Fail | Live fast/service-tier probe timed out after repeated 502 `upstream_http_error` rows. | Blocking for fast-mode compatibility. | Fix service-tier mapping for Codex backend. |
| Image via `--image` | Responses input | Partial | Live `codex exec --image` exited 0 with one 200 row. Plan 094 added direct explicit `input_image` local smoke coverage after this audit. | Needs fresh full live smoke. | Keep covered in switch-gate smokes. |
| Explicit Responses image input | `POST /v1/responses` | Fixed locally after audit | At `9ea0ebf`, live direct API probe returned 400 before upstream dispatch. Plan 094 added local decoder and fake-upstream smoke coverage. | Needs fresh full live smoke. | Keep covered in switch-gate smokes. |
| Tool definitions | Responses request | Fixed locally after audit | Direct recorder observed Codex sending tool definitions. Plan 094 now preserves representable function tools and filters unsupported Codex-only tool families. | Function tools improved; hosted/deferred tool parity remains limited. | Audit hosted/deferred tool families separately. |
| Tool-call loop | Responses output and follow-up | Fixed locally after audit | At `9ea0ebf`, fake upstream returned a function-call shape and local route returned `501`. Plan 094 added function-call SSE output and function-call-output follow-up input. | Needs fresh full live smoke. | Keep covered in switch-gate smokes. |
| Workspace edit through Codex CLI | Local workspace | Fail | Live `codex exec` exited 0, but the target file was unchanged. | Blocking for normal coding-agent use. | Implement tool preservation and tool-loop handling. |
| Function-call output input | `POST /v1/responses` | Fixed locally after audit | At `9ea0ebf`, live direct API probe returned 400 unsupported for `function_call_output`. Plan 094 added direct local smoke coverage for function-call-output follow-up input. | Needs fresh full live smoke. | Keep covered in switch-gate smokes. |
| Direct Codex chat text | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with assistant message content. | Existing direct chat path still works. | Keep covered in smokes. |
| Direct Codex chat reasoning/service tier | `POST /v1/chat/completions` | Fail | Live direct API probe returned 502 `upstream_http_error`. | Relevant to Codex option compatibility. | Fix Codex provider option mapping. |
| DeepSeek JSON mode | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with assistant message content. | Not a Codex blocker. | Keep provider smokes separate. |
| DeepSeek forced tool | `POST /v1/chat/completions` | Fail | Live direct API probe returned upstream 400. | Not a Codex local Responses blocker, but provider capability handling needs review. | Validate DeepSeek forced-tool shape and support. |
| OpenRouter forced tool | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with tool-call shape. | Confirms at least one real provider tool path works in direct Chat Completions. | Keep as provider smoke coverage. |
| Upstream `5xx` | Fake upstream | Pass as error handling | Codex retried; local metadata recorded normalized upstream failure as `502 upstream_http_error`. | Not a switch blocker by itself, but retry behavior is noisy. | Improve UX and retry accounting later. |
| Upstream `429` and `Retry-After` | Fake upstream | Pass as error handling | Codex retried; local metadata recorded `502 rate_limit_exceeded`, and health events retained retry-after metadata. | Relevant to quota UX, not this worktree. | Leave quota work to the quota-tracking worktree. |
| Privacy | Metadata and health tables | Partial | Earlier sentinel scan found no prompt, completion, token, account, raw body, or raw payload markers in checked metadata tables. Outbound backend requests used `store: false`. The live smoke did not repeat a full temp-output and log sentinel scan. | Privacy evidence is good but not a complete switch gate pass. | Add full live-smoke privacy scans before switching. |

## Blockers

1. Full Codex tool parity is not proven.

   Function tool definitions, function-call SSE output, and
   function-call-output follow-up input are now covered locally. Hosted,
   deferred, namespaced, MCP, custom, tool-search, shell, and patch-style tool
   families are still not full parity and need separate compatibility work.

2. Multimodal support needs a fresh full live switch-gate smoke.

   The original explicit `input_image` failure is fixed locally, but this audit
   has not been rerun end to end after plans 092 through 094.

3. Reasoning and service tier behavior is unreliable.

   Live minimal reasoning and fast/service-tier Codex CLI probes retried into
   repeated 502 rows. High reasoning exited 0 once, but direct Codex reasoning
   API probes returned 502. This needs a provider-option mapping fix and a
   deterministic smoke before claiming compatibility.

4. Health semantics are upstream-centered.

   The historical route-level `501` from a function-call output is fixed
   locally, but the larger reporting issue remains: future UI should
   distinguish upstream health from local route compatibility for unsupported
   hosted, deferred, or namespaced Codex tool families.

## Next Plans

- Plan 094: implemented local Responses tool definitions, function-call event
  output, and tool-output follow-up input handling.
- Plan 095: implement and verify Codex-compatible model capability metadata.
- Plan 096: fix Codex reasoning-effort, verbosity, and fast or priority
  service-tier provider option mapping, with live Codex CLI smokes for each
  supported value.
- Plan 097: tighten Codex compatibility UX around retries, route-level errors,
  and upstream-health versus local-route failures.

## Switch Gate

Before switching normal Codex use to `ilonasin`, these must pass in an isolated
smoke:

- root and `/v1` base URL model discovery and text turns,
- developer/system instruction behavior,
- reasoning effort and fast or priority service-tier behavior for real target
  model IDs,
- image input through `codex exec --image`,
- tool definitions, function-call output, and tool-output follow-up loops,
- upstream `401`, retryable `5xx`, `429`, and `Retry-After` error paths,
- privacy scans showing no forbidden local storage or logs, with outbound
  backend `store: false`.

Partial results do not count as compatibility for switching.

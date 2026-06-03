# Codex Compatibility Audit

Date: 2026-06-01

Audited worktree: Plan 099 worktree

Current implementation note: plans 092 through 095 changed the local Responses
surface and Codex model metadata. Plan 096 reran real-credential switch-gate
smokes against the current worktree binary. Plan 098 added provider-aware
Responses tool filtering for non-Codex providers. Plan 099 added the Codex
custom-tool relay needed by the tested `apply_patch` workspace-edit path. Codex
text, images, reasoning efforts, service tiers, developer instructions, direct
function-call follow-up, direct Responses calls to all three configured
provider types, a real DeepSeek `codex exec` text smoke, and a real Codex
workspace edit smoke now pass. The tested OpenRouter `codex exec` route no
longer fails on the local unsupported Codex tool type, but still failed at the
provider-response layer.

Codex CLI version: `codex-cli 0.135.0`

## Summary

`ilonasin serve` is closer to being usable as a Codex backend, but broad
switching is still blocked by OpenRouter Codex CLI compatibility and unproven
broader Codex tool-family parity.

Text-only `codex exec` traffic reaches local Responses routes through both root
and `/v1` provider base URLs with the real credentials configured in
`~/.ilonasin`. `codex exec --image`, developer instructions, `minimal`, `low`,
`medium`, `high`, and `xhigh` reasoning efforts, and both `flex` and `priority`
service tiers exited 0 against the real Codex provider instance.

Direct local Responses calls using real credentials also passed for Codex,
DeepSeek, and OpenRouter. Plan 098 confirms the local Responses route now
filters non-chat-representable Codex tool definitions for non-Codex providers.
Plan 099 confirms the tested Codex `apply_patch` workspace edit path now works
through local Responses custom-tool relay. That is not the same as full Codex
CLI agent compatibility for non-Codex providers: OpenRouter still needs
provider/model behavior investigation, and hosted, deferred, namespaced, MCP,
shell, and tool-search families remain unproven.

Do not switch normal Codex use to `ilonasin` yet unless the use is limited to
the covered Codex provider paths. The remaining blockers are OpenRouter Codex
CLI behavior and broader tool-family parity.

## Safety

The audit used the real Codex OAuth, DeepSeek API-key, and OpenRouter API-key
credentials already configured in `~/.ilonasin`. It used a temporary
`CODEX_HOME`, temporary server log and cache directories, and fresh local client
tokens created through the management API. The audit tokens were disabled after
the smoke.

At audit time, the primary Codex OAuth credential in the real database returned
401 for model discovery and had `refresh_unauthorized` refresh state. The live
switch-gate smoke temporarily disabled that credential, restored it during
cleanup, and used the next real Codex credential. That was a real historical
deployment hazard in the audited worktree. Current code inspection shows model
discovery now resolves the currently eligible credential pool, attempts
materialized credentials in order, refreshes a Codex OAuth 401 attempt once
when possible, and records health per attempted credential. This code status
does not replace a future live switch-gate rerun against real credentials.

The smoke did not copy the user's real `~/.ilonasin`, Codex auth state, OAuth
token state, logs, cache, SQLite files, WAL/SHM files, or account state into a
temporary home. It pointed the worktree server at the existing real SQLite
database in place.

Raw terminal output, raw JSONL, raw HTTP bodies, provider payloads, prompts,
completions, image bytes, tool arguments, tool results, bearer tokens, account
IDs, and request IDs were not kept in this report. Probe captures were reduced
to allowlisted structural fields and then removed. The plan 096 scan found zero
sentinel hits in checked metadata tables, zero sentinel hits in server logs,
zero forbidden log key hits, and zero secret-shaped log hits.

The Plan 099 live workspace-edit smoke used the same privacy posture. It
created and then disabled a fresh local client token, used temp logs/cache and
temp Codex/workspace directories, and found zero smoke-marker hits in checked
server logs and metadata fields.

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

## Affinity Signal Map

Credential pooling should prefer fields clients already send out of the box.
The current source-backed map is:

| Client/API | Out-of-box or common fields | Pooling interpretation |
| --- | --- | --- |
| Codex CLI 0.135 Responses | `prompt_cache_key` is set from the Codex thread ID. `client_metadata` includes `x-codex-installation-id`. Headers include `session-id`, `thread-id`, `x-client-request-id`, and, in core request paths, `x-codex-window-id`. | Prefer body `prompt_cache_key`, because it is stable for the Codex thread and matches the cache/session goal. Use `session-id` or `thread-id` only as fallback. Treat `x-codex-window-id` as observed window metadata, not credential-affinity fallback. Do not treat request-id-shaped fields as generally stable for other clients. |
| Codex app-server Responses | App-server turn APIs can forward turn-scoped `responsesapi_client_metadata` into Responses `client_metadata`. | Use only selected safe `client_metadata` keys such as `prompt_cache_key`, `session_id`, `thread_id`, and `conversation_id` when the top-level cache key is absent. Ignore installation, account, device, token, and request-id-shaped values. |
| Claude Code Anthropic | Prior local captures against Claude Code 2.1.159 showed Anthropic `metadata.user_id` as a JSON string containing `session_id`, plus `X-Claude-Code-Session-Id`. | Prefer the nested `metadata.user_id.session_id` when present and safe. Use the session header only as fallback. |
| Generic OpenAI Chat | `model` and `messages` may be the only fields. OpenAI SDK source also exposes top-level `prompt_cache_key`, with `user`, `session_id`, and `metadata` optional. | Use safe `session_id`, then safe top-level `prompt_cache_key`, then safe `user`, then selected safe metadata keys. If none exist, route through no-affinity least-in-flight plus round-robin. |
| Generic Responses-compatible clients | `prompt_cache_key` and `client_metadata` are optional. Many clients may send neither. | Use safe `prompt_cache_key`, then selected safe `client_metadata` keys. If none exist, route through no-affinity least-in-flight plus round-robin. |

Codex source evidence from `/tmp/codex-src-0.135.0/codex-rs`:

- `core/src/client.rs` builds normal Responses requests with
  `prompt_cache_key = Some(self.state.thread_id.to_string())` and
  `client_metadata` containing `x-codex-installation-id`.
- `codex-api/src/endpoint/responses.rs` adds `x-client-request-id` from
  `thread_id` on the Responses stream path, then extends headers with
  `build_session_headers(session_id, thread_id)`.
- `codex-api/src/requests/headers.rs` maps those session values into
  `session-id` and `thread-id`.
- `core/src/client.rs` and related tests cover `x-codex-window-id` as a
  request header for window lineage.

OpenAI SDK source evidence from `/tmp/openai-node`:

- `src/resources/chat/completions/completions.ts` exposes Chat Completions
  `prompt_cache_key` and describes `user` as deprecated in favor of
  `prompt_cache_key` for caching.

The important distinction for pooling is stability. `prompt_cache_key`,
`session-id`, and `thread-id` are session or thread affinity candidates when
they pass the local safety filter. `x-codex-window-id` is observed transport
metadata but is not used as an ingress credential-affinity fallback. A header
named `x-client-request-id` is not a
general affinity source even when Codex currently fills it from the thread ID,
because other harnesses commonly use request IDs as per-request values.

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
| Model discovery | `GET /models`, `GET /v1/models` | Needs live rerun | Historical audit: with credential 1 temporarily disabled, live discovery returned Codex rows with `chat,parallel_tool_calls,reasoning,responses,service_tier,stream,tools,vision`; with credential 1 enabled, the audited worktree got 401 from the primary credential. Current code inspection shows pooled credential resolution and per-attempt health recording are implemented. | Model metadata is shaped correctly, and the old primary-credential-only discovery hazard is resolved in code. Live switch-gate evidence should be refreshed before broad use. | Rerun model discovery in an isolated switch-gate smoke with multiple real credentials and verify per-attempt health. |
| Developer/system instructions | Responses text route | Pass | Live `codex exec` with `developer_instructions` exited 0 and recorded a 200 row. | Basic developer instruction routing works. | Keep covered in smokes. |
| Codex CLI through DeepSeek | `POST /v1/responses` | Pass | Plan 098 live `codex exec` against `pragnition-deepseek/deepseek-v4-flash` exited 0. Fake-upstream smoke also proved mixed Codex-style tool definitions are filtered to representable Chat function tools and `parallel_tool_calls` is not forwarded to DeepSeek. | Basic Codex CLI text routing through DeepSeek now works. | Keep covered in switch-gate smokes. |
| Codex CLI through OpenRouter | `POST /v1/responses` | Partial | Plan 098 live `codex exec` against `pragnition-openrouter/anthropic/claude-3-haiku` exited nonzero, but the old local `tools[n].type is unsupported` blocker and DeepSeek-only `parallel_tool_calls` blocker were absent. Safe metadata showed upstream/provider response failure for the tested model. Fake-upstream smokes prove mixed tool filtering, OpenRouter `parallel_tool_calls` forwarding, and local rejection of Codex custom-tool transcript items before upstream. | Local tool-family validation no longer blocks the route, but the tested OpenRouter model path is not ready. | Investigate OpenRouter provider/model tool response behavior. |
| Direct Responses through all provider types | `POST /v1/responses` | Pass | Direct real-credential Responses calls returned 200 for Codex, DeepSeek, and OpenRouter. | Provider routing works outside Codex CLI's default tool set. | Keep as provider smoke coverage. |
| Reasoning efforts | Codex CLI config | Pass | Live `minimal`, `low`, `medium`, `high`, and `xhigh` Codex CLI probes exited 0 and each recorded a 200 row. | Reasoning-effort mapping is no longer a switch blocker for the tested Codex model. | Keep covered in smokes. |
| Fast or priority service tier | Codex CLI config | Pass | Live `service_tier="flex"` and `service_tier="priority"` Codex CLI probes exited 0 and recorded 200 rows. | Service-tier mapping is no longer a switch blocker for the tested Codex model. | Keep covered in smokes. |
| Image via `--image` | Responses input | Pass | Live `codex exec --image` exited 0 and recorded 200 rows. | Basic multimodal Codex input works. | Keep covered in switch-gate smokes. |
| Explicit Responses image input | `POST /v1/responses` | Pass | Plan 096 live image smoke and earlier direct decoder coverage both passed. | Basic explicit image support works. | Keep covered in switch-gate smokes. |
| Tool definitions | Responses request | Fixed locally for current Chat-adapter routes | Direct recorder observed Codex sending tool definitions. Plan 094 preserved representable Codex function tools. Plan 098 filters non-chat-representable Responses tool families for DeepSeek/OpenRouter, skips deferred and strict-only tools for those providers, keeps duplicate checks among forwarded tools, and handles all-filtered tool sets without forwarding `tools` or `tool_choice`. | Function tools improved; hosted/deferred namespace/freeform parity remains limited. | Audit hosted/deferred tool families separately. |
| Tool-call loop | Responses output and follow-up | Partial | Direct live function-call-output follow-up returned 200 with SSE bytes. Plan 099 fake smokes pass final-only and added-plus-delta `custom_tool_call` output, accept `custom_tool_call_output` follow-up for Codex, and reject custom-tool transcript items for DeepSeek/OpenRouter before upstream. | Function and tested custom apply-patch transcripts work; full Codex tool-family parity remains unproven. | Audit hosted, deferred, namespaced, MCP, shell, and tool-search families separately. |
| Workspace edit through Codex CLI | Local workspace | Pass for tested Codex path | Plan 099 live `codex exec` exited 0, recorded 200 rows, and changed the target file in a temporary workspace. | The tested Codex provider apply-patch edit path is no longer a switch blocker. | Keep in switch-gate smokes; do not infer full tool parity. |
| Function-call output input | `POST /v1/responses` | Pass | Direct live follow-up with a synthetic prior function call and function-call output returned 200. | The local route can carry function-call output back to the model. | Keep covered in switch-gate smokes. |
| Direct Codex chat text | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with assistant message content. | Existing direct chat path still works. | Keep covered in smokes. |
| Direct Codex chat service tier | `POST /v1/chat/completions` | Fixed in code | Plan 278 fake-upstream smoke proves top-level `service_tier` accepts `priority`, `flex`, and `default`, omits `default` upstream, maps provider-options `fast` to upstream `priority`, and rejects Codex top-level `auto`/`scale`; DeepSeek still rejects top-level `service_tier`. | Direct Codex Chat service-tier mapping is no longer the option-compatibility blocker described by the historical live failure. | Keep covered in fake-upstream and future live provider smokes. |
| Direct Codex chat reasoning | `POST /v1/chat/completions` | Fixed locally via provider options | Codex reasoning remains represented as `provider_options.codex.reasoning`, not as a top-level Chat Completions field. Plan 281 temporary local smoke proved Chat request validation and Codex provider validation accept supported reasoning options, Codex Responses request shaping serializes upstream `reasoning` plus `include: ["reasoning.encrypted_content"]`, model-unsupported but locally allowed efforts map to a supported model fallback, no-reasoning requests omit reasoning/include, top-level Chat `reasoning` is rejected, and unsupported Codex reasoning values fail locally. | Direct Codex Chat reasoning request shaping is no longer a local option-compatibility blocker; broad switch-gate readiness still depends on live smokes and tool-family parity. | Keep provider-options reasoning covered in fake-upstream and future live provider smokes. |
| DeepSeek JSON mode | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with assistant message content. | Not a Codex blocker. | Keep provider smokes separate. |
| DeepSeek forced tool | `POST /v1/chat/completions` | Pass for non-strict forced tool | Later DeepSeek comparison live probe returned 200. Plan 279 temporary local smoke proved OpenAI Chat validation accepts a matching named `tool_choice`, rejects a mismatched named `tool_choice`, preserves `tools` and `tool_choice` in the DeepSeek upstream body, and still rejects strict tool mode locally. | Direct DeepSeek non-strict forced tool is no longer a current local request-shaping blocker; strict tool mode remains a separate beta/provider boundary. | Keep strict DeepSeek tool mode separate and only route it through beta behavior if explicitly implemented. |
| OpenRouter forced tool | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with tool-call shape. | Confirms at least one real provider tool path works in direct Chat Completions. | Keep as provider smoke coverage. |
| Upstream `5xx` | Fake upstream | Pass as error handling | Codex retried; local metadata recorded normalized upstream failure as `502 upstream_http_error`. | Not a switch blocker by itself, but retry behavior is noisy. | Improve UX and retry accounting later. |
| Upstream `429` and `Retry-After` | Fake upstream | Pass as error handling | Codex retried; local metadata recorded `502 rate_limit_exceeded`, and health events retained retry-after metadata. | Relevant to quota UX, not this worktree. | Leave quota work to the quota-tracking worktree. |
| Privacy | Metadata and server logs | Pass for checked surfaces | Plan 096 sentinel scan found zero marker hits in checked metadata tables, zero marker hits in server logs, zero forbidden log keys, and zero secret-shaped log hits. Code scan found the outbound Codex `store:false` path. | Checked ilonasin storage and logs stayed metadata-only. | Keep privacy scans in live switch-gate runs. |

## Blockers

1. Full Codex tool parity is not proven.

   Function tool definitions, function-call SSE output, and
   function-call-output follow-up input are now covered locally. The tested
   custom `apply_patch` path is also covered. Hosted, deferred, namespaced, MCP,
   tool-search, shell, and other custom tool families are still not full parity
   and need separate compatibility work.

2. Codex CLI through OpenRouter is still partial.

   DeepSeek text now passes through real `codex exec`, and OpenRouter is no
   longer blocked by local tool-family validation. The tested OpenRouter model
   still failed at the provider-response layer, so OpenRouter is not ready for
   broad Codex CLI use.

3. Model discovery needs refreshed live switch-gate evidence.

   The historical audit found a primary-credential-only discovery hazard. Current
   code inspection shows pooled credential resolution is implemented for model
   discovery, with per-attempt health rows. A future isolated live smoke should
   prove the current behavior against the real multi-credential Codex set.

4. Health semantics are upstream-centered.

   The historical route-level `501` from a function-call output is fixed
   locally, but the larger reporting issue remains: future UI should
   distinguish upstream health from local route compatibility for unsupported
   hosted, deferred, or namespaced Codex tool families.

## Next Plans

- Plan 094: implemented local Responses tool definitions, function-call event
  output, and tool-output follow-up input handling.
- Plan 095: implement and verify Codex-compatible model capability metadata.
- Plan 096: ran real switch-gate smokes and updated this audit.
- Plan 097: write a quota and usage pooling policy plan. Quota pooling is
  separate from current availability fallback pooling.
- Plan 099: relay Codex custom tool calls needed by the tested apply-patch
  workspace edit path.
- Later compatibility work: fix OpenRouter provider/model tool response
  behavior and audit broader Codex tool families.

## Switch Gate

Before switching normal Codex use to `ilonasin`, these must pass in an isolated
smoke:

- root and `/v1` base URL model discovery and text turns,
- workspace edit through `codex exec`,
- Codex CLI routing through DeepSeek and OpenRouter, including Codex's default
  tool family list and provider-specific tool response behavior,
- historical primary-credential discovery regression behavior,
- upstream `401`, retryable `5xx`, `429`, and `Retry-After` error paths,
- privacy scans showing no forbidden local storage or logs.

Partial results do not count as compatibility for switching.

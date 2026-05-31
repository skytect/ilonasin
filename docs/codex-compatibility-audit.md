# Codex Compatibility Audit

Date: 2026-06-01

Audited worktree commit: `e5a4f96`

Current implementation note: plans 092 through 095 changed the local Responses
surface and Codex model metadata. Plan 096 reran real-credential switch-gate
smokes against the current worktree binary. Plan 098 added provider-aware
Responses tool filtering for non-Codex providers. Codex text, images, reasoning
efforts, service tiers, developer instructions, direct function-call follow-up,
direct Responses calls to all three configured provider types, and a real
DeepSeek `codex exec` text smoke now pass. Normal workspace editing through
`codex exec` still does not modify the target file. The tested OpenRouter
`codex exec` route no longer fails on the local unsupported Codex tool type,
but still failed at the provider-response layer.

Codex CLI version: `codex-cli 0.135.0`

## Summary

`ilonasin serve` is not ready as the default backend for normal Codex coding
agent use.

Text-only `codex exec` traffic reaches local Responses routes through both root
and `/v1` provider base URLs with the real credentials configured in
`~/.ilonasin`. `codex exec --image`, developer instructions, `minimal`, `low`,
`medium`, `high`, and `xhigh` reasoning efforts, and both `flex` and `priority`
service tiers exited 0 against the real Codex provider instance.

Direct local Responses calls using real credentials also passed for Codex,
DeepSeek, and OpenRouter. Plan 098 confirms the local Responses route now
filters non-chat-representable Codex tool definitions for non-Codex providers.
That is not the same as full Codex CLI agent compatibility for non-Codex
providers: OpenRouter still needs provider/model behavior investigation, and
workspace edits remain unproven.

Do not switch normal Codex use to `ilonasin` yet. The remaining blocker is
normal coding-agent behavior: `codex exec` exited 0 for a workspace edit task
but left the target file unchanged.

## Safety

The audit used the real Codex OAuth, DeepSeek API-key, and OpenRouter API-key
credentials already configured in `~/.ilonasin`. It used a temporary
`CODEX_HOME`, temporary server log and cache directories, and fresh local client
tokens created through the management API. The audit tokens were disabled after
the smoke.

The primary Codex OAuth credential in the real database currently returns 401
for model discovery and has `refresh_unauthorized` refresh state. The live
switch-gate smoke temporarily disabled that credential, restored it during
cleanup, and used the next real Codex credential. This is a real deployment
hazard: non-pooled model discovery still uses the primary credential only.

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
| Model discovery | `GET /models`, `GET /v1/models` | Partial | With credential 1 temporarily disabled, live discovery returned Codex rows with `chat,parallel_tool_calls,reasoning,responses,service_tier,stream,tools,vision`. With credential 1 enabled, Codex discovery gets 401 from the primary credential. | Model metadata is now shaped correctly, but primary credential health can hide valid secondary accounts. | Make model discovery health and credential selection explicit before broad use. |
| Developer/system instructions | Responses text route | Pass | Live `codex exec` with `developer_instructions` exited 0 and recorded a 200 row. | Basic developer instruction routing works. | Keep covered in smokes. |
| Codex CLI through DeepSeek | `POST /v1/responses` | Pass | Plan 098 live `codex exec` against `pragnition-deepseek/deepseek-v4-flash` exited 0. Fake-upstream smoke also proved mixed Codex-style tool definitions are filtered to representable Chat function tools and `parallel_tool_calls` is not forwarded to DeepSeek. | Basic Codex CLI text routing through DeepSeek now works. | Keep covered in switch-gate smokes. |
| Codex CLI through OpenRouter | `POST /v1/responses` | Partial | Plan 098 live `codex exec` against `pragnition-openrouter/anthropic/claude-3-haiku` exited nonzero, but the old local `tools[n].type is unsupported` blocker and DeepSeek-only `parallel_tool_calls` blocker were absent. Safe metadata showed upstream/provider response failure for the tested model. Fake-upstream smoke proved mixed tool filtering and OpenRouter `parallel_tool_calls` forwarding. | Local tool-family validation no longer blocks the route, but the tested OpenRouter model path is not ready. | Investigate OpenRouter provider/model tool response behavior. |
| Direct Responses through all provider types | `POST /v1/responses` | Pass | Direct real-credential Responses calls returned 200 for Codex, DeepSeek, and OpenRouter. | Provider routing works outside Codex CLI's default tool set. | Keep as provider smoke coverage. |
| Reasoning efforts | Codex CLI config | Pass | Live `minimal`, `low`, `medium`, `high`, and `xhigh` Codex CLI probes exited 0 and each recorded a 200 row. | Reasoning-effort mapping is no longer a switch blocker for the tested Codex model. | Keep covered in smokes. |
| Fast or priority service tier | Codex CLI config | Pass | Live `service_tier="flex"` and `service_tier="priority"` Codex CLI probes exited 0 and recorded 200 rows. | Service-tier mapping is no longer a switch blocker for the tested Codex model. | Keep covered in smokes. |
| Image via `--image` | Responses input | Pass | Live `codex exec --image` exited 0 and recorded 200 rows. | Basic multimodal Codex input works. | Keep covered in switch-gate smokes. |
| Explicit Responses image input | `POST /v1/responses` | Pass | Plan 096 live image smoke and earlier direct decoder coverage both passed. | Basic explicit image support works. | Keep covered in switch-gate smokes. |
| Tool definitions | Responses request | Fixed locally for current Chat-adapter routes | Direct recorder observed Codex sending tool definitions. Plan 094 preserved representable Codex function tools. Plan 098 filters non-chat-representable Responses tool families for DeepSeek/OpenRouter, skips deferred and strict-only tools for those providers, keeps duplicate checks among forwarded tools, and handles all-filtered tool sets without forwarding `tools` or `tool_choice`. | Function tools improved; hosted/deferred namespace/freeform parity remains limited. | Audit hosted/deferred tool families separately. |
| Tool-call loop | Responses output and follow-up | Partial | Direct live function-call-output follow-up returned 200 with SSE bytes. Workspace edit still did not mutate the file. | Direct transcript support works, but full Codex agent tool-loop behavior is still not reliable. | Inspect Codex tool families and patch/apply flows from real `codex exec`. |
| Workspace edit through Codex CLI | Local workspace | Fail | Live `codex exec` exited 0 and recorded 200 rows, but the target file was unchanged. | Blocking for normal coding-agent use. | Preserve enough Codex tool-loop behavior for file edits, or expose a clear unsupported state. |
| Function-call output input | `POST /v1/responses` | Pass | Direct live follow-up with a synthetic prior function call and function-call output returned 200. | The local route can carry function-call output back to the model. | Keep covered in switch-gate smokes. |
| Direct Codex chat text | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with assistant message content. | Existing direct chat path still works. | Keep covered in smokes. |
| Direct Codex chat reasoning/service tier | `POST /v1/chat/completions` | Fail | Live direct API probe returned 502 `upstream_http_error`. | Relevant to Codex option compatibility. | Fix Codex provider option mapping. |
| DeepSeek JSON mode | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with assistant message content. | Not a Codex blocker. | Keep provider smokes separate. |
| DeepSeek forced tool | `POST /v1/chat/completions` | Fail | Live direct API probe returned upstream 400. | Not a Codex local Responses blocker, but provider capability handling needs review. | Validate DeepSeek forced-tool shape and support. |
| OpenRouter forced tool | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with tool-call shape. | Confirms at least one real provider tool path works in direct Chat Completions. | Keep as provider smoke coverage. |
| Upstream `5xx` | Fake upstream | Pass as error handling | Codex retried; local metadata recorded normalized upstream failure as `502 upstream_http_error`. | Not a switch blocker by itself, but retry behavior is noisy. | Improve UX and retry accounting later. |
| Upstream `429` and `Retry-After` | Fake upstream | Pass as error handling | Codex retried; local metadata recorded `502 rate_limit_exceeded`, and health events retained retry-after metadata. | Relevant to quota UX, not this worktree. | Leave quota work to the quota-tracking worktree. |
| Privacy | Metadata and server logs | Pass for checked surfaces | Plan 096 sentinel scan found zero marker hits in checked metadata tables, zero marker hits in server logs, zero forbidden log keys, and zero secret-shaped log hits. Code scan found the outbound Codex `store:false` path. | Checked ilonasin storage and logs stayed metadata-only. | Keep privacy scans in live switch-gate runs. |

## Blockers

1. Workspace edits through Codex CLI still do not work.

   The live workspace edit probe exited 0 and recorded successful upstream rows,
   but the target file was unchanged. This blocks normal coding-agent use.

2. Full Codex tool parity is not proven.

   Function tool definitions, function-call SSE output, and
   function-call-output follow-up input are now covered locally. Hosted,
   deferred, namespaced, MCP, custom, tool-search, shell, and patch-style tool
   families are still not full parity and need separate compatibility work.

3. Codex CLI through OpenRouter is still partial.

   DeepSeek text now passes through real `codex exec`, and OpenRouter is no
   longer blocked by local tool-family validation. The tested OpenRouter model
   still failed at the provider-response layer, so OpenRouter is not ready for
   broad Codex CLI use.

4. Primary Codex credential health can hide valid secondary accounts.

   The real primary Codex OAuth credential returned 401 and had
   `refresh_unauthorized`. The smoke temporarily disabled it and restored it
   afterward to test the remaining real Codex credential set.

5. Health semantics are upstream-centered.

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
- Later compatibility work: fix Codex CLI workspace edit/tool-loop behavior and
  OpenRouter provider/model tool response behavior.

## Switch Gate

Before switching normal Codex use to `ilonasin`, these must pass in an isolated
smoke:

- root and `/v1` base URL model discovery and text turns,
- workspace edit through `codex exec`,
- Codex CLI routing through DeepSeek and OpenRouter, including Codex's default
  tool family list and provider-specific tool response behavior,
- primary credential health and model discovery behavior,
- upstream `401`, retryable `5xx`, `429`, and `Retry-After` error paths,
- privacy scans showing no forbidden local storage or logs.

Partial results do not count as compatibility for switching.

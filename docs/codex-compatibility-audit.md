# Codex Compatibility Audit

Date: 2026-06-07

Audited worktree: Plan 529 evidence refresh after commit `9d16c81`.

Current implementation note: plan 529 reran the switch-gate evidence refresh
against the current worktree binary with `codex-cli 0.137.0`. Root and `/v1`
model discovery now pass against the real configured credential store, with six
active Codex OAuth credentials visible through safe management metadata. Direct
streaming Responses calls returned HTTP 200 for Codex, DeepSeek, and
OpenRouter. A real `codex exec` text probe against the Codex provider reached
the local Responses route but failed locally because Codex CLI 0.137.0 sends
`tools[0].strict`, and the current Codex-preserved Responses function-tool
allowlist rejects that field.

Codex CLI version: `codex-cli 0.137.0`

## Summary

`ilonasin serve` is closer to being usable as a Codex backend, but broad
switching is still blocked. The current first blocker is local rejection of
Codex CLI 0.137.0 function tool declarations containing `strict`. After that is
fixed, OpenRouter Codex CLI behavior and broader Codex tool-family parity still
need refreshed evidence.

Root and `/v1` model discovery now pass against the real configured credential
store. Direct streaming Responses calls using real credentials returned HTTP
200 for Codex, DeepSeek, and OpenRouter. The current real `codex exec` text
probe did not reach model inference successfully because the local Responses
tool declaration validator rejected `tools[0].strict`.

Do not switch normal Codex use to `ilonasin` yet. The remaining blockers are
the Codex CLI `strict` tool-declaration compatibility regression, OpenRouter
Codex CLI behavior after that fix, and broader hosted, deferred, namespaced,
MCP, shell, and tool-search parity.

## Safety

The audit used the real Codex OAuth, DeepSeek API-key, and OpenRouter API-key
credentials already configured in `~/.ilonasin`. It used a temporary
`CODEX_HOME`, temporary workspace, temporary server log and cache directories,
and a fresh local client token created through the management API. The audit
token was disabled during cleanup.

The smoke did not copy the user's real `~/.ilonasin`, Codex auth state, OAuth
token state, logs, cache, SQLite files, WAL/SHM files, or account state into a
temporary home. It pointed the worktree server at the existing real SQLite
database in place because Codex OAuth credentials are stored there. That left
durable metadata-only rows for the temporary local token and probe requests.

Raw terminal output, raw JSONL, raw HTTP bodies, provider payloads, prompts,
completions, image bytes, tool arguments, tool results, bearer tokens, account
IDs, and request IDs were not kept in this report. Probe captures were reduced
to allowlisted structural fields and then removed. The plan 529 temp-log scan
checked one temporary server log file and found zero probe-marker hits, zero
bearer-shaped hits, and zero secret-key-name hits.

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

Credential pooling should prefer fields clients already send out of the box,
then fall back to local inputs ilonasin always has. The current source-backed
map separates observed named-client fields from optional API fields:

| Client/API | Out-of-box or common fields | Pooling interpretation |
| --- | --- | --- |
| Codex CLI 0.135 Responses | `prompt_cache_key` is set from the Codex thread ID. `client_metadata` includes `x-codex-installation-id`. Headers include `session-id`, `thread-id`, `x-client-request-id`, and, in core request paths, `x-codex-window-id`. | Prefer body `prompt_cache_key`, because it is stable for the Codex thread and matches the cache/session goal. Use `session-id` or `thread-id` only as fallback. Treat `x-codex-window-id` as observed window metadata, not credential-affinity fallback. Do not treat request-id-shaped fields as generally stable for other clients. |
| Codex app-server Responses | App-server turn APIs can forward turn-scoped `responsesapi_client_metadata` into Responses `client_metadata`. | Use only selected safe `client_metadata` keys such as `prompt_cache_key`, `session_id`, `thread_id`, and `conversation_id` when the top-level cache key is absent. Ignore installation, account, device, token, and request-id-shaped values. |
| Claude Code Anthropic | Prior local captures against Claude Code 2.1.159 showed Anthropic `metadata.user_id` as a JSON string containing `session_id`, plus `X-Claude-Code-Session-Id`. | Prefer the nested `metadata.user_id.session_id` when present and safe. Use the session header only as fallback. |
| Generic OpenAI Chat | `model` and `messages` may be the only fields. OpenAI SDK source also exposes top-level `prompt_cache_key`, with `user`, `session_id`, and `metadata` optional. | Use safe `session_id`, then safe top-level `prompt_cache_key`, then safe `user`, then selected safe metadata keys. If none exist, route through the verified local token identity, provider/model route, least-in-flight pressure, and token-scoped cursor. |
| Generic Responses-compatible clients | `model` and `input` may be the only fields. `prompt_cache_key`, `client_metadata`, and top-level `metadata` are optional. Many clients may send none of them. | Use safe `prompt_cache_key`, then selected safe `client_metadata` keys, then selected safe top-level `metadata` keys. If none exist, route through the verified local token identity, provider/model route, least-in-flight pressure, and token-scoped cursor. |

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

The important distinction for pooling is provenance and stability.
`prompt_cache_key` is a preferred signal when present because Codex sends it in
the audited Responses path, not because every OpenAI-compatible harness sends
it. `session-id` and `thread-id` are session or thread affinity candidates only
when they pass the local safety filter and body affinity is absent.
`x-codex-window-id` is observed transport metadata but is not used as an ingress
credential-affinity fallback. A header named `x-client-request-id` is not a
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
| Root base URL text | `POST /responses` | Blocked for Codex CLI 0.137.0 | Plan 529 did not complete a root-base `codex exec` text pass because the `/v1` custom-provider text probe already exposed a local `tools[0].strict is unsupported` request-validation blocker. | Text-only Codex CLI switching is blocked until the strict function-tool declaration shape is handled. | Implement bounded Codex Responses function-tool `strict` handling, then rerun root and `/v1` text smokes. |
| `/v1` base URL text | `POST /v1/responses` | Blocked for Codex CLI 0.137.0 | Live `codex exec` against `pragnition-codex/gpt-5.4-mini` reached the local Responses route but exited nonzero with sanitized terminal output showing `tools[0].strict is unsupported`. | Current Codex CLI text routing is locally blocked before inference. | Implement bounded Codex Responses function-tool `strict` handling, then rerun. |
| Model discovery | `GET /models`, `GET /v1/models` | Pass | Plan 529 live evidence returned HTTP 200 for both routes with `object: "list"` and 348 model rows while six active Codex OAuth credentials were visible through safe management metadata. | The historical primary-credential-only discovery hazard is no longer reproduced in this live refresh. | Keep covered in switch-gate smokes and inspect per-attempt health in future provider-health slices. |
| Developer/system instructions | Responses text route | Needs rerun | Older live `codex exec` evidence passed, but Plan 529 did not rerun this path because the current Codex CLI 0.137.0 function-tool `strict` shape blocks the basic text probe first. | Developer-instruction compatibility is stale until the strict declaration blocker is fixed. | Rerun after bounded strict handling. |
| Codex CLI through DeepSeek | `POST /v1/responses` | Needs rerun | Plan 529 did not rerun DeepSeek `codex exec` because the current Codex CLI function-tool `strict` shape blocks the Codex-provider text smoke first. Plan 098's older DeepSeek pass is stale for Codex CLI 0.137.0. | DeepSeek Codex CLI switching evidence is stale until the strict declaration blocker is fixed. | Rerun after bounded strict handling. |
| Codex CLI through OpenRouter | `POST /v1/responses` | Needs rerun | Plan 529 did not rerun OpenRouter `codex exec` because the current Codex CLI function-tool `strict` shape blocks the Codex-provider text smoke first. The prior OpenRouter provider-response failure remains stale evidence. | OpenRouter Codex CLI readiness is still unknown on the current CLI and current code. | Rerun after bounded strict handling, then investigate provider/model behavior if it still fails upstream. |
| Direct Responses through all provider types | `POST /v1/responses` | Pass for streaming | Plan 529 direct streaming Responses probes returned HTTP 200 for `pragnition-codex/gpt-5.4-mini`, `pragnition-deepseek/deepseek-v4-flash`, and `pragnition-openrouter/openai/gpt-3.5-turbo`. Non-stream direct probes were invalid for this surface because local Responses validation requires `stream: true`. | Provider routing works for direct streaming Responses outside Codex CLI's default tool set. | Keep as provider smoke coverage. |
| Reasoning efforts | Codex CLI config | Needs rerun | Older `minimal`, `low`, `medium`, `high`, and `xhigh` Codex CLI probes passed, but Plan 529 stopped at the current function-tool `strict` blocker before option smokes. | Reasoning-effort compatibility is stale for Codex CLI 0.137.0 until the strict declaration blocker is fixed. | Rerun after bounded strict handling. |
| Fast or priority service tier | Codex CLI config | Needs rerun | Older `service_tier="flex"` and `service_tier="priority"` Codex CLI probes passed, but Plan 529 stopped at the current function-tool `strict` blocker before option smokes. | Service-tier compatibility is stale for Codex CLI 0.137.0 until the strict declaration blocker is fixed. | Rerun after bounded strict handling. |
| Image via `--image` | Responses input | Needs rerun | Older `codex exec --image` evidence passed, but Plan 529 stopped at the current function-tool `strict` blocker before image smokes. | Basic multimodal Codex CLI switching evidence is stale until the strict declaration blocker is fixed. | Rerun after bounded strict handling. |
| Explicit Responses image input | `POST /v1/responses` | Pass | Plan 096 live image smoke and earlier direct decoder coverage both passed. | Basic explicit image support works. | Keep covered in switch-gate smokes. |
| Tool definitions | Responses request | Blocked for current Codex CLI | Direct recorder observed Codex sending tool definitions. Plan 529 shows Codex CLI 0.137.0 sends a `strict` field on a function tool declaration, and the current Codex-preserved function allowlist rejects it with `tools[0].strict is unsupported`. | Current Codex CLI text routing is blocked at local request validation. | Add bounded Codex-preserved function-tool `strict` handling without broad hosted/deferred/tool-family parity. |
| Tool-call loop | Responses output and follow-up | Partial | Direct live function-call-output follow-up returned 200 with SSE bytes. Plan 099 fake smokes pass final-only and added-plus-delta `custom_tool_call` output, accept `custom_tool_call_output` follow-up for Codex, and reject custom-tool transcript items for DeepSeek/OpenRouter before upstream. | Function and tested custom apply-patch transcripts work; full Codex tool-family parity remains unproven. | Audit hosted, deferred, namespaced, MCP, shell, and tool-search families separately. |
| Workspace edit through Codex CLI | Local workspace | Needs rerun | Plan 099 live `codex exec` exited 0, recorded 200 rows, and changed the target file in a temporary workspace. Plan 529 did not rerun this path because Codex CLI 0.137.0 now hits the local function-tool `strict` blocker first. | The tested Codex provider apply-patch evidence is stale for the current CLI and current code. | Rerun after bounded strict handling; do not infer full tool parity. |
| Function-call output input | `POST /v1/responses` | Pass | Direct live follow-up with a synthetic prior function call and function-call output returned 200. | The local route can carry function-call output back to the model. | Keep covered in switch-gate smokes. |
| Direct Codex chat text | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with assistant message content. | Existing direct chat path still works. | Keep covered in smokes. |
| Direct Codex chat service tier | `POST /v1/chat/completions` | Fixed in code | Plan 278 fake-upstream smoke proves top-level `service_tier` accepts `priority`, `flex`, and `default`, omits `default` upstream, maps provider-options `fast` to upstream `priority`, and rejects Codex top-level `auto`/`scale`; DeepSeek still rejects top-level `service_tier`. | Direct Codex Chat service-tier mapping is no longer the option-compatibility blocker described by the historical live failure. | Keep covered in fake-upstream and future live provider smokes. |
| Direct Codex chat reasoning | `POST /v1/chat/completions` | Fixed locally via provider options | Codex reasoning remains represented as `provider_options.codex.reasoning`, not as a top-level Chat Completions field. Plan 281 temporary local smoke proved Chat request validation and Codex provider validation accept supported reasoning options, Codex Responses request shaping serializes upstream `reasoning` plus `include: ["reasoning.encrypted_content"]`, model-unsupported but locally allowed efforts map to a supported model fallback, no-reasoning requests omit reasoning/include, top-level Chat `reasoning` is rejected, and unsupported Codex reasoning values fail locally. | Direct Codex Chat reasoning request shaping is no longer a local option-compatibility blocker; broad switch-gate readiness still depends on live smokes and tool-family parity. | Keep provider-options reasoning covered in fake-upstream and future live provider smokes. |
| DeepSeek JSON mode | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with assistant message content. | Not a Codex blocker. | Keep provider smokes separate. |
| DeepSeek forced tool | `POST /v1/chat/completions` | Pass for non-strict forced tool | Later DeepSeek comparison live probe returned 200. Plan 279 temporary local smoke proved OpenAI Chat validation accepts a matching named `tool_choice`, rejects a mismatched named `tool_choice`, preserves `tools` and `tool_choice` in the DeepSeek upstream body, and still rejects strict tool mode locally. | Direct DeepSeek non-strict forced tool is no longer a current local request-shaping blocker; strict tool mode remains a separate beta/provider boundary. | Keep strict DeepSeek tool mode separate and only route it through beta behavior if explicitly implemented. |
| OpenRouter forced tool | `POST /v1/chat/completions` | Pass | Live direct API probe returned 200 with tool-call shape. | Confirms at least one real provider tool path works in direct Chat Completions. | Keep as provider smoke coverage. |
| Upstream `5xx` | Fake upstream | Pass as error handling | Codex retried; local metadata recorded normalized upstream failure as `502 upstream_http_error`. | Not a switch blocker by itself, but retry behavior is noisy. | Improve UX and retry accounting later. |
| Upstream `429` and `Retry-After` | Fake upstream | Pass as error handling | Codex retried; local metadata recorded `502 rate_limit_exceeded`, and health events retained retry-after metadata. | Relevant to quota UX, not this worktree. | Leave quota work to the quota-tracking worktree. |
| Privacy | Metadata and server logs | Pass for checked temp logs | Plan 529 temp-log scan found zero probe-marker hits, zero bearer-shaped hits, and zero secret-key-name hits in the checked temporary server log file. Code still sends Codex `store:false`. | Checked temp logs stayed metadata-only. Durable metadata rows were left in the real SQLite because the live OAuth credential store was used in place. | Keep privacy scans in live switch-gate runs and extend metadata scans after the strict-tool fix. |

## Blockers

1. Current Codex CLI function-tool declarations include unsupported `strict`.

   Codex CLI 0.137.0 sends a `strict` field on at least one Responses function
   tool declaration. The Codex-preserved function tool allowlist currently
   accepts `type`, `name`, `description`, and `parameters`, so the local route
   rejects the request before inference with `tools[0].strict is unsupported`.
   This is the next implementation slice.

2. Full Codex tool parity is not proven.

   Function tool definitions, function-call SSE output, and
   function-call-output follow-up input are now covered locally. The tested
   custom `apply_patch` path is also covered. Hosted, deferred, namespaced, MCP,
   tool-search, shell, and other custom tool families are still not full parity
   and need separate compatibility work.

3. Codex CLI through OpenRouter needs a current rerun.

   The previous OpenRouter probe failed at the provider-response layer after
   older local tool-shape blockers were removed. Plan 529 could not rerun that
   path because the current Codex CLI `strict` declaration now blocks local
   validation first.

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
- Plan 529: refreshed switch-gate evidence for Codex CLI 0.137.0, model
  discovery, and direct streaming Responses.
- Next compatibility work: accept or intentionally translate bounded
  `strict:false` function-tool declarations on Codex-preserved Responses tools,
  then rerun Codex CLI text, DeepSeek, and OpenRouter switch-gate smokes.
- Later compatibility work: fix OpenRouter provider/model tool response
  behavior if still present after the strict-tool fix, and audit broader Codex
  tool families.

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

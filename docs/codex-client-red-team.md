# Codex CLI client red team

Accessed: 2026-05-31. Updated: 2026-06-01. This records safe compatibility findings only. Raw
Codex JSONL, HTTP request bodies, response bodies, prompts, completions, image
bytes, tool arguments, tool results, tokens, and account identifiers are not
included.

## Summary

Current `codex exec` compatibility is no longer blocked at the basic local API
shape. Local Responses routes exist and real Codex CLI probes now reach
ilonasin for Codex text, image input, reasoning effort, service tier, and
developer-instruction turns.

Codex can be configured to use `ilonasin` as a custom local provider with
environment-variable bearer auth, but current Codex sends Responses API traffic.
That means the compatibility surface is Codex Responses plus Codex's default
tool-family list, not Chat Completions.

The current blockers are narrower:

- `codex exec` workspace edit now passes for the tested Codex apply-patch path.
- `codex exec` routed to DeepSeek now passes a text smoke through ilonasin.
- `codex exec` routed to OpenRouter no longer fails with the local unsupported
  tool-type blocker, but the tested OpenRouter model still failed at the
  provider-response layer.
- Direct Responses calls using real credentials passed for Codex, DeepSeek, and
  OpenRouter.

## Confirmed

- `codex exec` is the non-interactive Codex CLI entry point.
- Current Codex model providers support `wire_api = "responses"` only.
- A custom provider can target a local base URL with:
  - `model_provider = "ilonasin"`
  - `model_providers.ilonasin.base_url = "http://127.0.0.1:<port>/v1"`
  - `model_providers.ilonasin.env_key = "ILONASIN_CLIENT_TOKEN"`
  - `model_providers.ilonasin.wire_api = "responses"`
- Auth is attached when using the env-key path.
- Text, reasoning-effort, service-tier, developer-instruction, and image probes
  reach the local Responses endpoint and pass for the Codex provider.
- A real `codex exec` workspace-edit smoke reached local Responses, relayed a
  Codex custom `apply_patch` tool turn, and modified the temporary file.
- Direct Responses probes pass for all three configured provider types.
- `--image <FILE>...` consumes following positional arguments unless the prompt
  is separated with `--`.
- `--ignore-user-config` also ignores the temporary custom provider config. For
  live smokes, write a temporary `CODEX_HOME/config.toml` instead of relying
  only on nested `-c` provider overrides.

## Current Failure Modes

| Feature | Route | Result | Failure class | Required next fix |
| --- | --- | --- | --- | --- |
| Text prompt | custom local provider | Pass for Codex | None in latest smoke | Keep covered |
| Image prompt | custom local provider | Pass for Codex | None in latest smoke | Keep covered |
| Reasoning effort | custom local provider | Pass for `minimal`, `low`, `medium`, `high`, `xhigh` | None in latest smoke | Keep covered |
| Fast or priority tier | custom local provider | Pass for `flex` and `priority` | None in latest smoke | Keep covered |
| Workspace edit | custom local provider | Pass for tested Codex apply-patch path | None in latest smoke | Keep covered; broader tool parity remains separate |
| Codex CLI through DeepSeek | custom local provider | Pass | None in Plan 098 smoke | Keep covered |
| Codex CLI through OpenRouter | custom local provider | Partial | Old local tool-type blocker absent; latest safe metadata showed upstream/provider response failure for the tested model. | Investigate OpenRouter model/tool response behavior |
| Model discovery | `GET /v1/models` | Needs live rerun | Historical audit found a primary-credential-only failure mode. Current code inspection shows pooled credential resolution and per-attempt health recording are implemented. | Rerun isolated multi-credential model-discovery smoke before broad switching |

## Endpoint Evidence

Recorder preflight:

- bare base URL variant reached the Responses endpoint family with auth present,
- `/v1` base URL variant reached the Responses endpoint family with auth
  present,
- text, tool-like, reasoning-effort, fast-mode, and corrected image probes all
  reached the Responses endpoint family.

Latest direct probes:

- bare and `/v1` local Responses text turns passed for the Codex provider,
- direct local Responses calls passed for Codex, DeepSeek, and OpenRouter,
- local model cache exposes Codex capability flags including `responses`,
  `tools`, `vision`, `reasoning`, and `service_tier`.
- Plan 099 fake-upstream smokes pass Codex `custom_tool_call` output through
  local Responses SSE, accept `custom_tool_call_output` follow-up for Codex,
  reject custom tool transcripts for DeepSeek and OpenRouter before upstream,
  and keep custom tool sentinels out of checked logs and metadata.
- Plan 099 live `codex exec` workspace-edit smoke exited 0, modified the
  temporary workspace file, disabled its fresh local token, and found zero
  sentinel hits in checked logs and metadata fields.
- Plan 098 local fake-upstream smokes pass mixed Codex-style Responses tools
  through DeepSeek/OpenRouter by forwarding only representable flat Chat
  function tools.
- Plan 098 live `codex exec` text smoke passed through DeepSeek. The OpenRouter
  smoke no longer hit the local `tools[n].type is unsupported` or
  `parallel_tool_calls` blocker, but exited nonzero with safe metadata showing
  an upstream/provider response failure for the tested model.

## Interpretation

Plans `092` through `095` added the local Responses entrypoint, tool transcript
handling, image decoding, and Codex capability metadata. Plan 098 removed the
local blocker for non-Codex Codex CLI tool definitions by filtering
non-chat-representable Responses tools and DeepSeek-unsupported
`parallel_tool_calls`. Plan 099 relays the Codex custom-tool subset used by the
tested `apply_patch` workspace edit. Remaining compatibility work is now
provider-specific model/tool response behavior and broader Codex hosted,
deferred, namespaced, MCP, shell, and tool-search parity.

## Next Plan Direction

- Investigate OpenRouter model/tool response behavior under real `codex exec`.
- Audit broader Codex tool families beyond the tested custom `apply_patch`
  path.
- Keep the live privacy/log scan in every switch-gate smoke.
- Treat quota and usage pooling as a separate policy plan from current
  availability fallback pooling.

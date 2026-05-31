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

- `codex exec` workspace edit exited 0 but left the target file unchanged.
- `codex exec` routed to DeepSeek/OpenRouter through ilonasin failed with
  `tools[5].type is unsupported`.
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
| Workspace edit | custom local provider | Fails behaviorally | File unchanged after exit 0 | Fix Codex tool-loop/edit behavior |
| Codex CLI through DeepSeek/OpenRouter | custom local provider | Fails before inference | Unsupported tool type | Support or filter Codex tool families for non-Codex providers |
| Model discovery | `GET /v1/models` | Partial | Primary Codex credential 401 can hide secondary credentials | Make primary credential health explicit |

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

## Interpretation

Plans `092` through `095` added the local Responses entrypoint, tool transcript
handling, image decoding, and Codex capability metadata. The remaining
compatibility problem is now agent behavior, especially edit/tool loops and
non-Codex provider handling for Codex tool families.

## Next Plan Direction

- Fix workspace edit/tool-loop behavior under real `codex exec`.
- Support or explicitly filter Codex's unsupported tool families when routing
  Codex CLI to DeepSeek and OpenRouter.
- Keep the live privacy/log scan in every switch-gate smoke.
- Treat quota and usage pooling as a separate policy plan from current
  availability fallback pooling.

# Codex CLI client red team

Accessed: 2026-05-31. This records safe compatibility findings only. Raw
Codex JSONL, HTTP request bodies, response bodies, prompts, completions, image
bytes, tool arguments, tool results, tokens, and account identifiers are not
included.

## Summary

Current `codex exec` compatibility is blocked at the local API shape.

Codex can be configured to use `ilonasin` as a custom local provider with
environment-variable bearer auth, but current Codex sends Responses API traffic.
`ilonasin` currently exposes Chat Completions, not a local Responses API.

## Confirmed

- `codex exec` is the non-interactive Codex CLI entry point.
- Current Codex model providers support `wire_api = "responses"` only.
- A custom provider can target a local base URL with:
  - `model_provider = "ilonasin"`
  - `model_providers.ilonasin.base_url = "http://127.0.0.1:<port>/v1"`
  - `model_providers.ilonasin.env_key = "ILONASIN_CLIENT_TOKEN"`
  - `model_providers.ilonasin.wire_api = "responses"`
- Auth is attached when using the env-key path.
- Text, tool-like, reasoning-effort, fast-mode, and image probes all reach the
  Responses endpoint family before any provider-specific behavior is tested.
- `--image <FILE>...` consumes following positional arguments unless the prompt
  is separated with `--`.

## Current Failure Modes

| Feature | Route | Result | Failure class | Required next fix |
| --- | --- | --- | --- | --- |
| Text prompt | custom local provider | Fails before model output | Endpoint mismatch | Add local Responses endpoint |
| Image prompt | custom local provider | Fails before model output | Endpoint mismatch | Add local Responses image input support |
| Tool-like task | custom local provider | Fails before tool execution | Endpoint mismatch | Add Responses tools and local tool-call event handling |
| Reasoning effort | custom local provider | Fails before inference | Endpoint mismatch | Add Responses request parsing and option translation |
| Fast mode | custom local provider | Fails before inference | Endpoint mismatch | Add Responses request parsing and service tier translation |
| Model discovery | `GET /v1/models` | Not Codex-compatible | Model metadata mismatch | Add Codex-compatible model catalog or bypass path |

## Endpoint Evidence

Recorder preflight:

- bare base URL variant reached the Responses endpoint family with auth present,
- `/v1` base URL variant reached the Responses endpoint family with auth
  present,
- text, tool-like, reasoning-effort, fast-mode, and corrected image probes all
  reached the Responses endpoint family.

`ilonasin` direct probe:

- bare Responses endpoint returned `404`,
- `/v1` Responses endpoint returned `404`,
- `/v1/models` returned an OpenAI-style model response path, not the
  Codex-compatible model catalog expected by the current Codex client.

## Interpretation

Plan `090` improved the Chat Completions surface and the upstream Codex
Responses bridge. It did not make `ilonasin` a local Responses API server for
Codex CLI.

The next compatibility slice should implement a local Responses API entrypoint.
That should be separate from the existing Chat Completions route and should
parse Codex Responses request shapes directly instead of trying to force them
through the existing chat decoder.

## Next Plan Direction

Plan `092` should implement the smallest useful local Responses API support:

- `POST /v1/responses`, and probably bare `POST /responses` if Codex custom
  provider base URLs without `/v1` are supported,
- SSE event output in the shape expected by Codex CLI,
- text input and output first,
- Codex-compatible model metadata or an explicit model-catalog compatibility
  path,
- safe translation from Responses request fields to existing provider adapters,
- no raw body, prompt, completion, image, tool-argument, or provider-payload
  persistence.

Images, complex tool calls, reasoning effort, and fast mode should then be
enabled incrementally on top of the local Responses route, because right now all
of those probes fail at the same endpoint mismatch.

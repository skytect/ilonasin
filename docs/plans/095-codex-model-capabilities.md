# 095 Codex Model Capabilities

## Context

Plans 092 through 094 added a local Responses route, multimodal input decoding,
and function tool-loop handling. The remaining Codex compatibility audit still
calls out weak model discovery metadata. Current code already reads Codex model
catalog fields for request-time option mapping, but `GET /models` normalization
stores Codex cache rows as only `chat,reasoning,stream`.

OpenAI's current model docs say GPT-5.5 is the recommended frontier model for
complex coding and professional work, with image input, function calling, and
reasoning efforts `none`, `low`, `medium`, `high`, and `xhigh`. They also show
that `minimal` exists for GPT-5, not GPT-5.5. That means ilonasin should keep
model IDs as provider-returned data, not hardcode old defaults, and should
derive capabilities from the Codex model catalog where possible.

## Goal

Make Codex model discovery preserve safe capability metadata so the model cache
and management surfaces reflect what a Codex model can actually do.

## Scope

1. Extend Codex model normalization in `internal/provider/http_models.go` to
   derive sanitized capability flags from Codex model catalog fields:
   - always include `chat`, `responses`, and `stream` for accepted Codex models,
   - include `reasoning` when reasoning levels are present or a default
     reasoning level is present,
   - include `tools` as a coarse local function-tool capability for Codex
     Responses models, not as hosted Codex tool support,
   - include `parallel_tool_calls` only when the catalog advertises
     `supports_parallel_tool_calls`,
   - include `service_tier` when Codex advertises `priority` or `flex`,
   - include `vision` only from Codex `input_modalities`, treating an omitted
     field as Codex source default text plus image input.
2. Keep privacy constraints intact:
   - do not store raw model payloads,
   - do not store prompts, responses, request bodies, response bodies, raw SSE,
     bearer tokens, provider request IDs, or account IDs,
   - do not expose base instructions.
3. Update direct smoke coverage in `serve --check` to prove Codex model cache
   rows preserve the new flags from the fake Codex catalog and reject unsafe
   payload leakage.
4. Refresh the Codex compatibility audit note for stale findings already fixed
   by plans 092 through 094, especially explicit `input_image`, function tool
   definitions, function-call output, and function-call-output follow-up input.
5. Keep capability flags observational only. They must not affect routing,
   credential selection, fallback policy, or request validation in this slice.

## Non-Goals

- Do not change quota tracking.
- Do not add permanent test files.
- Do not change model IDs or default configured provider instances.
- Do not add direct storage for reasoning level names, service tier names, base
  instructions, or raw Codex catalog JSON.
- Do not implement hosted Codex tools beyond the function tool-loop already
  supported.
- Do not derive `tools` from hosted search, patch, shell, or experimental tool
  catalog fields.

## Verification

- Review the code before running checks.
- `git diff --check`
- `find . -name '*_test.go' -type f -print`
- `go test ./...`
- `go vet ./...`
- `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
- `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
- `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Review Questions

1. Is deriving only coarse capability flags the right privacy boundary, instead
   of storing exact reasoning levels or service tier names?
2. Should `responses` be a first-class capability flag for Codex models now that
   ilonasin exposes local `/responses` routes?
3. Are the proposed recognized multimodal fields conservative enough to avoid
   storing raw provider payload data while still detecting image input support?

# 041 OpenRouter Capability Flags

## Goal

Refresh OpenRouter model-cache capability flags so the metadata summary matches
the OpenRouter request features already implemented by ilonasin, without adding
new request fields, routing behavior, or permanent tests.

## Sources

- `AGENTS.md`
- all markdown files under `docs/**`
- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- current OpenRouter OpenAPI `ChatRequest.supported_parameters` context from
  `/models`
- current `internal/provider/http_chat.go` model normalization
- current `internal/app/app.go` direct smoke checks

## Scope

1. Keep model-cache capabilities as a safe allowlisted metadata summary.
2. Continue deriving OpenRouter flags only from `/models`
   `supported_parameters`.
3. Add flags only for OpenRouter request features already implemented locally:
   sampling controls, advanced sampling controls, logit bias, JSON response
   format, logprobs, reasoning, tools, tool choice, parallel tool calls,
   prediction, user, service tier, session id, and metadata.
4. Keep `chat` as the baseline flag for valid OpenRouter model rows.
5. Keep deterministic sorted comma-separated flags.
6. Do not persist raw `supported_parameters`, pricing, descriptions, provider
   payloads, request metadata, user identifiers, session identifiers, or
   provider routing objects.
7. Do not advertise unsupported OpenRouter fields such as `models`, `route`,
   `plugins`, `cache_control`, `modalities`, `image_config`,
   `stop_server_tools_when`, `trace`, `debug`, or any future field not
   explicitly implemented.
8. Do not change request validation, routing, fallback policy, credential
   selection, provider privacy routing, TUI storage, or SQLite schema.
9. Update direct smoke fixtures so OpenRouter model discovery proves the new
   implemented flags are stored, sorted, and sanitized.
10. Add leak checks proving unsupported and raw provider metadata are not copied
    into capability flags or local model output.

## Non-Goals

- Do not add support for OpenRouter top-level `models`, `route`, `plugins`,
  `cache_control`, `modalities`, `image_config`,
  `stop_server_tools_when`, `trace`, or `debug`.
- Do not turn capability flags into routing requirements or eligibility
  filters.
- Do not infer capabilities from request history, account state, config, local
  API tokens, or provider credentials.
- Do not add permanent tests.

## Implementation Plan

1. Add an explicit OpenRouter `supported_parameters` to capability flag mapping
   near `openRouterCapabilityFlags`.
2. Map implemented top-level OpenRouter parameters to stable internal flag
   names:
   - `temperature`, `top_p`, `frequency_penalty`, `presence_penalty`, and
     `stop` to `sampling`
   - `top_k`, `min_p`, `top_a`, `repetition_penalty`, and `seed` to
     `advanced_sampling`
   - `response_format` to `json_object`
   - `tools` and `tool_choice` to `tools`
   - `parallel_tool_calls` to `parallel_tool_calls`
   - `prediction` to `prediction`
   - `user` to `user`
   - `service_tier` to `service_tier`
   - `session_id` to `session_id`
   - `metadata` to `metadata`
   - existing `logprobs`, `top_logprobs`, `logit_bias`, `reasoning`, and
     `stream` mappings unchanged
3. Keep unsupported or unimplemented parameters ignored by default.
4. Update the fake OpenRouter `/models` fixture to include implemented,
   unsupported, and marker-bearing raw metadata fields.
5. Update model-cache smoke expectations for the expanded OpenRouter capability
   string.
6. Add smoke assertions that unsupported field names and raw metadata markers do
   not appear in cached capability flags or local model responses.
7. Review the diff manually before running commands.
8. Run:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
   - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Review Questions

1. Are the flag names specific enough for TUI and local model discovery without
   becoming a public provider contract?
2. Should `max_completion_tokens` be left out because it is common request
   shape rather than a feature capability?
3. Is keeping broader provider routing out of capability metadata the right
   boundary for this slice?

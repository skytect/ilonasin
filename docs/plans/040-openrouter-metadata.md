# 040 OpenRouter Metadata

## Goal

Add narrow OpenRouter-only support for the documented top-level `metadata`
chat request field without persisting or displaying caller metadata.

## Sources

- `AGENTS.md`
- all markdown files under `docs/**`
- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- current OpenRouter OpenAPI `ChatRequest.metadata`

## Scope

1. Accept top-level `metadata` only for OpenRouter chat completions.
2. Validate it as a JSON object with at most 16 key/value pairs.
3. Require metadata keys to be non-empty strings up to 64 characters.
4. Require metadata values to be JSON strings up to 512 characters. Empty
   string values are allowed.
5. Reject `null`, non-object values, non-string values, empty keys, too many
   pairs, keys longer than 64 characters, and values longer than 512
   characters before credential resolution.
6. Reject `metadata` for DeepSeek and Codex before credential resolution.
7. Forward accepted OpenRouter `metadata` unchanged to the upstream OpenRouter
   chat completion request.
8. Cover non-streaming and streaming forwarding in direct serve checks.
9. Add boundary smokes for an explicitly present empty object, an accepted empty
   string value, 16 valid pairs, 17 invalid pairs, 64-character keys, a valid
   multibyte 64-character key whose byte length exceeds 64, 65-character keys,
   512-character values, 513-character values, and a valid multibyte
   512-character value whose byte length exceeds 512.
10. Add accepted OpenRouter non-streaming and streaming marker-bearing metadata
    smokes proving forwarded metadata is not persisted or displayed through
    SQLite metadata, local errors, normalized streaming output, `serve --check`,
    or `manage --check`.
11. Add invalid and unsupported smoke cases proving marker-bearing metadata
    does not reach upstream, SQLite metadata, local errors, `serve --check`, or
    `manage --check` output.
12. Add no-eligible invalid and unsupported-provider smoke cases to prove
    validation happens before credential resolution.

## Non-Goals

- Do not support OpenRouter top-level `models`, `route`, `plugins`,
  `cache_control`, `modalities`, `image_config`, `web_search_options`,
  `stop_server_tools_when`, `trace`, or `debug`.
- Do not infer metadata from local API tokens, provider credentials, accounts,
  config, request metadata, or TUI state.
- Do not persist request metadata fields from the caller's `metadata` object.
- Do not add permanent tests.

## Implementation Plan

1. Add `Metadata map[string]string` to `openai.ChatCompletionRequest`.
2. Allow `metadata` in the strict top-level key list.
3. Add raw validation for the documented object and string-map limits.
4. Forward `metadata` from `MarshalUpstreamChatRequest` only when the field is
   present.
5. Add provider validation so DeepSeek and Codex reject it while OpenRouter
   accepts it.
6. Update serve-check fake upstream validation to assert non-streaming and
   streaming OpenRouter requests include forwarded metadata exactly where
   expected.
7. Add valid and invalid boundary smokes for present empty metadata, pair count,
   key length, value length, and multibyte character counting for both keys and
   values.
8. Add accepted marker-bearing OpenRouter smokes plus SQLite, local response,
   streaming output, `serve --check`, and `manage --check` leak scans.
9. Add invalid-value, unsupported-provider, and no-eligible smoke cases plus
   marker leak scans.
10. Run:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
   - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Review Questions

1. Is rejecting empty metadata keys the right strict subset even though the
   current OpenAPI schema only describes maximum key length?
2. Should empty string metadata values be accepted because the schema only
   describes a maximum value length?
3. Are the direct smoke checks sufficient without permanent tests?

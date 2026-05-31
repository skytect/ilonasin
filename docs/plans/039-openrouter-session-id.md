# 039 OpenRouter Session ID

## Goal

Add narrow OpenRouter-only support for the documented top-level `session_id`
chat request field.

## Sources

- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- OpenRouter OpenAPI `ChatRequest.session_id`
- OpenRouter chat completions documentation via Context7

## Scope

1. Accept top-level `session_id` only for OpenRouter chat completions.
2. Validate it as a JSON string up to 256 characters, matching the current
   OpenRouter OpenAPI schema.
3. Reject `null`, non-string values, empty strings, and strings longer than 256
   characters before credential resolution.
4. Reject `session_id` for DeepSeek and Codex before credential resolution.
5. Forward accepted OpenRouter `session_id` unchanged to the upstream
   OpenRouter chat completion request.
6. Do not support, synthesize, or forward the `x-session-id` header.
7. Cover non-streaming and streaming forwarding in direct serve checks.
8. Add accepted 256-character and rejected 257-character boundary smokes,
   including a valid multibyte 256-character value whose byte length exceeds
   256.
9. Add a marker-bearing `x-session-id` direct serve smoke that proves the header
   is not forwarded upstream and is not persisted or displayed.
10. Add invalid and unsupported smoke cases that prove marker-bearing session
    values do not reach upstream, SQLite metadata, local errors, `serve --check`,
    or `manage --check` output.
11. Add no-eligible invalid and unsupported-provider smoke cases to prove
    validation happens before credential resolution.
12. Add fake upstream response markers for OpenRouter response `session_id` and
    prove those markers are not persisted or emitted through normalized
    streaming output. Non-streaming upstream response bodies remain pass-through.

## Non-Goals

- Do not add header-based session routing.
- Do not persist request or response session IDs.
- Do not derive session IDs from local API tokens, provider credentials,
  accounts, config, or request metadata.
- Do not alter routing, fallback policy, provider privacy policy, or account
  selection.
- Do not add permanent tests.

## Implementation Plan

1. Add `SessionID *string` to `openai.ChatCompletionRequest`.
2. Allow `session_id` in the strict top-level key list.
3. Add raw validation requiring a non-empty string up to 256 characters.
4. Forward `session_id` from `MarshalUpstreamChatRequest` only when present.
5. Add provider validation so DeepSeek and Codex reject it while OpenRouter
   accepts it.
6. Update serve-check fake upstream validation to assert non-streaming and
   streaming OpenRouter requests include the forwarded session ID.
7. Add 256-character valid, multibyte 256-character valid, and
   257-character invalid boundary smokes.
8. Add `x-session-id` ignored-header smokes plus upstream absence and marker
   leak scans.
9. Add invalid-value, unsupported-provider, and no-eligible smoke cases plus
   marker leak scans.
10. Add response `session_id` marker smokes for non-stream metadata safety and
   normalized streaming output safety.
11. Run:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
   - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Review Questions

1. Is rejecting empty strings the right strict subset for a sticky routing
   identifier even though the OpenAPI schema only declares a max length?
2. Should both request and response session IDs stay out of metadata until
   there is an explicit safe storage/display design?
3. Are the direct smoke checks sufficient without permanent tests?

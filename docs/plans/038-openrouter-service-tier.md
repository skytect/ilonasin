# 038 OpenRouter Service Tier

## Goal

Add narrow OpenRouter-only support for the documented top-level
`service_tier` chat request field.

## Sources

- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- OpenRouter OpenAPI `ChatRequest.service_tier`
- OpenRouter service-tiers documentation via Context7

## Scope

1. Accept top-level `service_tier` only for OpenRouter chat completions.
2. Allow only the documented concrete values:
   - `"auto"`
   - `"default"`
   - `"flex"`
   - `"priority"`
   - `"scale"`
3. Reject `null`, non-string values, empty strings, and unrecognized string
   values before credential resolution.
4. Reject `service_tier` for DeepSeek and Codex before credential resolution.
5. Forward accepted OpenRouter `service_tier` unchanged to the upstream
   OpenRouter chat completion request.
6. Cover non-streaming and streaming forwarding in direct serve checks.
7. Add invalid and unsupported smoke cases that prove marker-bearing tier values
   do not reach upstream, SQLite metadata, local errors, `serve --check`, or
   `manage --check` output.
8. Add no-eligible invalid and unsupported-provider smoke cases to prove
   validation happens before credential resolution.
9. Add fake upstream response markers for OpenRouter response `service_tier`
   and prove those markers are not persisted or emitted through normalized
   streaming output. Non-streaming upstream response bodies remain pass-through.

## Non-Goals

- Do not support OpenRouter SDK open-enum custom tier strings.
- Do not persist served tier response metadata.
- Do not alter routing, fallback policy, provider privacy policy, or account
  selection.
- Do not add permanent tests.

## Implementation Plan

1. Add `ServiceTier *string` to `openai.ChatCompletionRequest`.
2. Allow `service_tier` in the strict top-level key list.
3. Add raw validation requiring one of the five known OpenRouter tier strings.
4. Forward `service_tier` from `MarshalUpstreamChatRequest` only when present.
5. Add provider validation so DeepSeek and Codex reject it while OpenRouter
   accepts it.
6. Update serve-check fake upstream validation to assert non-streaming and
   streaming OpenRouter requests include the forwarded tier.
7. Add invalid-value, unsupported-provider, and no-eligible smoke cases plus
   marker leak scans.
8. Add response `service_tier` marker smokes for non-stream metadata safety and
   normalized streaming output safety.
9. Run:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
   - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Review Questions

1. Is using only the five documented concrete values the right strict subset,
   even though the SDK models allow custom strings?
2. Should served-tier response metadata stay out of scope until there is an
   explicit safe storage/display design?
3. Are the smoke checks sufficient without permanent tests?

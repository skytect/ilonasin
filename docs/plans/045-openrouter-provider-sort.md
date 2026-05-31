# 045 OpenRouter Provider Sort

## Goal

Add narrow OpenRouter-only support for `provider.sort` through the existing
`provider_options.openrouter.provider` escape hatch, without changing local
credential fallback, model addressing, storage, or provider selection.

## Sources

- `AGENTS.md`
- all markdown files under `docs/**`
- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/plans/036-openrouter-privacy-routing.md`
- `docs/plans/042-openrouter-allow-fallbacks.md`
- `docs/plans/043-openrouter-provider-targets.md`
- `docs/plans/044-openrouter-provider-filters.md`
- OpenRouter OpenAPI document, `https://openrouter.ai/openapi.json`
- current `internal/provider/http_chat.go` provider option validation
- current `internal/app/app.go` direct smoke checks

## Scope

1. Accept `provider_options.openrouter.provider.sort` for OpenRouter chat
   completions.
2. Accept the string form only when it is one of `price`, `throughput`,
   `latency`, or `exacto`.
3. Accept the object form only as a non-empty object with keys `by` and/or
   `partition`.
4. Require object `by`, when present, to be one of `price`, `throughput`,
   `latency`, or `exacto`.
5. Require object `partition`, when present, to be one of `model` or `none`.
6. Reject `null`, empty strings, unknown strings, wrong scalar types, empty
   objects, unknown object keys, null object fields, and unknown object field
   values.
7. Use the documented enum subset even though the OpenAPI schema currently has
   `x-speakeasy-unknown-values: allow`, because the local API is a strict
   request subset and unsupported fields should fail clearly.
8. Forward accepted sort values to OpenRouter as top-level `provider.sort`.
9. Allow `sort` to combine with existing accepted provider fields:
   `require_parameters`, `data_collection`, `zdr`, `allow_fallbacks`, `only`,
   `ignore`, `quantizations`, and `max_price`.
10. Reject `sort` when `order` is also present, because OpenRouter defines
    `sort` as applying only when `order` is not specified.
11. Keep top-level caller `provider` rejected.
12. Continue rejecting `sort` for DeepSeek and Codex before credential
    resolution.
13. Continue rejecting remaining unsupported OpenRouter provider routing fields,
    including `preferred_max_latency`, `preferred_min_throughput`,
    `enforce_distillable_text`, `models`, and `route`.
14. Do not store provider sort values, provider routing objects, request bodies,
    raw upstream provider payloads, or raw SSE chunks.
15. Do not change ilonasin credential fallback groups, fallback events, routing,
    model addressing, account selection, retry behavior, model fallback
    behavior, or recorded cost telemetry.
16. Add direct non-streaming and streaming smoke checks proving accepted sort
    values reach only the OpenRouter fake upstream.
17. Add accepted sentinel smokes for `exacto` and `partition: "none"` proving
    sort values are forwarded upstream. Use `exacto` as the distinctive
    leak-scan marker for SQLite metadata, local errors, normalized client
    output, `serve --check`, and `manage --check`; validate `partition:
    "none"` only through exact fake-upstream request inspection.
18. Add a combined smoke check for sort plus `only`, `ignore`, provider
    filters, and `allow_fallbacks: false`.
19. Add invalid and unsupported-provider smoke checks for null sort, empty sort,
    unknown sort strings, wrong sort types, empty objects, unknown object keys,
    bad `by`, bad `partition`, null `by`, null `partition`, `sort` plus
    `order`, and marker-bearing invalid values.
20. Add no-eligible-cache smoke checks proving invalid sort and
    unsupported-provider sort fail before credential resolution.

## Non-Goals

- Do not implement `preferred_max_latency`, `preferred_min_throughput`,
  `enforce_distillable_text`, `models`, `route`, BYOK preferences,
  provider-specific headers, or model-level fallback.
- Do not auto-inject sort values from config, TUI state, credential fallback
  settings, model cache, provider health, latency observations, or local cost
  history.
- Do not infer OpenRouter native provider selection from local fallback events.
- Do not add permanent tests.

## Implementation Plan

1. Add explicit OpenRouter provider sort validators in
   `internal/provider/http_chat.go`.
2. Extend `validateOpenRouterProvider` to accept `sort` only after validation.
3. Keep the existing marshal path that forwards the validated provider object
   unchanged as OpenRouter top-level `provider`.
4. Add serve-check helpers in `internal/app/app.go` for exact provider sort
   validation, including object equality for `by` and `partition`.
5. Move `provider-sort` out of the unsupported invalid OpenRouter cases and add
   invalid shape cases for `sort`.
6. Add non-streaming and streaming success smokes for string sort and object
   sort.
7. Add accepted sentinel success smokes plus local response, SQLite,
   `serve --check`, and `manage --check` leak scans using `exacto` as the
   distinctive marker.
8. Add a combined smoke for sort plus `only`, `ignore`, provider filters, and
   `allow_fallbacks: false`.
9. Add unsupported-provider and no-eligible-cache smokes for provider sort.
10. Review the diff manually before running commands.
11. Run:
    - `find . -name '*_test.go' -type f -print`
    - `git diff --check`
    - `go test ./...`
    - `go vet ./...`
    - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
    - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
    - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Review Questions

1. Is the strict documented enum subset the right local API choice despite the
   OpenAPI unknown-value allowance?
2. Should `sort` object fields be required to be explicit strings rather than
   accepting documented nullable entries?
3. Is this a coherent slice without also adding performance thresholds?

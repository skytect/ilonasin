# 044 OpenRouter Provider Filters

## Goal

Add narrow OpenRouter-only support for provider filter fields
`provider.quantizations` and `provider.max_price` through the existing
`provider_options.openrouter.provider` escape hatch, without changing local
credential fallback, model addressing, storage, or pricing telemetry.

## Sources

- `AGENTS.md`
- all markdown files under `docs/**`
- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/plans/036-openrouter-privacy-routing.md`
- `docs/plans/042-openrouter-allow-fallbacks.md`
- `docs/plans/043-openrouter-provider-targets.md`
- OpenRouter provider routing documentation,
  `https://openrouter.ai/docs/guides/routing/provider-selection`
- OpenRouter chat completion documentation,
  `https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request`
- current `internal/provider/http_chat.go` provider option validation
- current `internal/app/app.go` direct smoke checks

## Scope

1. Accept `provider_options.openrouter.provider.quantizations` for OpenRouter
   chat completions.
2. Require `quantizations` to be a non-empty JSON array of unique strings, with
   at most 16 entries.
3. Allow only documented quantization levels: `int4`, `int8`, `fp4`, `fp6`,
   `fp8`, `fp16`, `bf16`, `fp32`, and `unknown`.
4. Accept `provider_options.openrouter.provider.max_price` for OpenRouter chat
   completions.
5. Require `max_price` to be a non-empty JSON object with only these keys:
   `prompt`, `completion`, `request`, `image`, and `audio`.
6. Require each `max_price` value to be a JSON number, finite, greater than or
   equal to zero, and less than or equal to `1000000`.
7. Treat numeric-string `max_price` values as out of scope even though the
   current OpenAPI `BigNumberUnion` allows strings, because the provider
   routing guide examples use numbers and this slice keeps a strict local
   request subset.
8. Preserve accepted `max_price` number tokens as `json.Number` values when
   forwarding upstream.
9. Forward accepted filters to OpenRouter as top-level
   `provider.quantizations` and `provider.max_price`.
10. Allow these filters to combine with existing accepted provider fields:
   `require_parameters`, `data_collection`, `zdr`, `allow_fallbacks`, `order`,
   `only`, and `ignore`.
11. Keep top-level caller `provider` rejected.
12. Continue rejecting provider filters for DeepSeek and Codex before
    credential resolution.
13. Continue rejecting every OpenRouter provider routing field other than the
    explicit allowlist, including `sort`, `preferred_max_latency`,
    `preferred_min_throughput`, and `enforce_distillable_text`.
14. Do not store provider filters, provider routing objects, request bodies, or
    raw upstream provider payloads.
15. Do not change ilonasin credential fallback groups, fallback events,
    routing, model addressing, account selection, retry behavior, model
    fallback behavior, or recorded cost telemetry.
16. Add direct non-streaming and streaming smoke checks proving accepted
    provider filters reach only the OpenRouter fake upstream.
17. Add accepted sentinel-value smokes using distinctive valid quantizations and
    exact raw JSON number tokens such as `0.00000123` and `1e-7`, proving they
    are forwarded upstream without float round-trip or numeric reformatting and
    are not persisted or displayed through SQLite metadata, local errors,
    normalized client output, `serve --check`, or `manage --check`.
18. Add combined smoke checks for filters plus provider target lists and
    `allow_fallbacks: false`.
19. Add invalid and unsupported-provider smoke checks for wrong types, empty
    arrays/objects, duplicate quantizations, 17-entry quantization arrays,
    unknown quantizations, unexpected `max_price` keys, numeric-string prices,
    non-number prices, negative prices, overflow prices, hostile exponent
    values, and marker-bearing invalid values.
20. Add `max_price` boundary smokes proving `1000000` is accepted and values
    above the bound such as `1000001` and `1000000.000001` are rejected.
21. Add no-eligible-cache smoke checks proving invalid provider filters and
    unsupported-provider filters fail before credential resolution.

## Non-Goals

- Do not implement OpenRouter `sort`, `preferred_max_latency`,
  `preferred_min_throughput`, `enforce_distillable_text`, `models`, `route`,
  BYOK preferences, provider-specific headers, or model-level fallback.
- Do not auto-inject provider filters from config, TUI state, credential
  fallback settings, model cache, or local cost history.
- Do not infer OpenRouter native provider selection from local fallback events.
- Do not treat request `max_price` values as usage/cost telemetry.
- Do not add permanent tests.

## Implementation Plan

1. Add explicit OpenRouter quantization and max-price validators in
   `internal/provider/http_chat.go`.
2. Extend `validateOpenRouterProvider` to accept `quantizations` and
   `max_price` only after validation.
3. Keep the existing marshal path that forwards the validated provider object
   unchanged as OpenRouter top-level `provider`.
4. Add serve-check helpers in `internal/app/app.go` for exact provider filter
   validation, including raw-token assertions for forwarded `json.Number`
   values.
5. Move `provider-quantizations` and `provider-max-price` out of the
   unsupported invalid OpenRouter cases and add invalid shape cases for each.
6. Add non-streaming and streaming success smokes for provider filters.
7. Add accepted sentinel-value success smokes plus local response, SQLite,
   `serve --check`, and `manage --check` leak scans.
8. Add a combined smoke for filters plus provider targets and
   `allow_fallbacks: false`.
9. Add unsupported-provider and no-eligible-cache smokes for provider filters.
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

1. Is accepting only documented quantization strings the right strict subset?
2. Is rejecting numeric-string `max_price` values acceptable for this slice,
   despite OpenAPI allowing `BigNumberUnion` strings?
3. Is `1000000` a reasonable upper bound for `max_price` to avoid unbounded
   numeric payloads while staying far above normal provider pricing?
4. Are these filter fields a coherent slice without also adding sort and
   performance thresholds?

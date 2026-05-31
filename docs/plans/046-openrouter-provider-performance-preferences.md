# 046 OpenRouter Provider Performance Preferences

## Goal

Add narrow OpenRouter-only support for provider performance preference fields
`provider.preferred_max_latency` and `provider.preferred_min_throughput`
through the existing `provider_options.openrouter.provider` escape hatch,
without changing local credential fallback, model addressing, storage, or
provider selection.

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
- `docs/plans/045-openrouter-provider-sort.md`
- OpenRouter OpenAPI document, `https://openrouter.ai/openapi.json`
- current `internal/provider/http_chat.go` provider option validation
- current `internal/app/app.go` direct smoke checks

## Scope

1. Accept `provider_options.openrouter.provider.preferred_max_latency` for
   OpenRouter chat completions.
2. Accept `provider_options.openrouter.provider.preferred_min_throughput` for
   OpenRouter chat completions.
3. Accept either field as a direct JSON number or as a non-empty object with
   percentile keys.
4. For object form, allow only `p50`, `p75`, `p90`, and `p99`.
5. Require every accepted value to be a JSON number, finite, greater than zero,
   and less than or equal to `1000000`.
6. Reject `null`, strings, booleans, arrays, empty objects, unknown object keys,
   null object values, non-number object values, zero, negative values,
   overflow values, hostile exponent values, and values above the bound.
7. Treat numeric strings as out of scope even though some OpenRouter schemas use
   broad number unions elsewhere, because these specific OpenAPI schemas show
   `type: number`.
8. Preserve accepted number tokens as `json.Number` values when forwarding
   upstream.
9. Forward accepted preferences to OpenRouter as top-level
   `provider.preferred_max_latency` and
   `provider.preferred_min_throughput`.
10. Allow performance preferences to combine with existing accepted provider
    fields: `require_parameters`, `data_collection`, `zdr`, `allow_fallbacks`,
    `order`, `only`, `ignore`, `quantizations`, `max_price`, and `sort`.
11. Preserve the existing rejection of `sort` plus `order`.
12. Keep top-level caller `provider` rejected.
13. Continue rejecting performance preferences for DeepSeek and Codex before
    credential resolution.
14. Continue rejecting remaining unsupported OpenRouter provider routing fields,
    including `enforce_distillable_text`, `models`, and `route`.
15. Do not store provider performance preferences, provider routing objects,
    request bodies, raw upstream provider payloads, or raw SSE chunks.
16. Do not change ilonasin credential fallback groups, fallback events, routing,
    model addressing, account selection, retry behavior, model fallback
    behavior, latency telemetry, stream metrics, or recorded cost telemetry.
17. Add direct non-streaming and streaming smoke checks proving accepted direct
    number and object forms reach only the OpenRouter fake upstream.
18. Add accepted sentinel-value smokes using distinctive raw JSON number tokens
    such as `0.00000123`, `1e-7`, `98765.4321`, and `1e-1024`, proving they
    are forwarded upstream without float round-trip or numeric reformatting and
    are not persisted or displayed through SQLite metadata, local errors,
    normalized client output, `serve --check`, or `manage --check`.
19. Add a combined smoke check for performance preferences plus provider sort,
    provider filters, provider targets, and `allow_fallbacks: false`, while
    avoiding the existing invalid `sort` plus `order` combination.
20. Add invalid and unsupported-provider smoke checks for wrong types, empty
    objects, unknown percentile keys, null object values, non-number object
    values, zero, negative values, values above `1000000`, overflow values,
    hostile exponent values, and marker-bearing invalid values.
21. Add no-eligible-cache smoke checks proving invalid performance preferences
    and unsupported-provider performance preferences fail before credential
    resolution.

## Non-Goals

- Do not implement `enforce_distillable_text`, `models`, `route`, BYOK
  preferences, provider-specific headers, model-level fallback, endpoint
  fallback, or latency-aware local routing.
- Do not auto-inject performance preferences from config, TUI state, credential
  fallback settings, model cache, provider health, latency observations, stream
  metrics, or local cost history.
- Do not treat request performance preference values as observed latency,
  throughput, stream, or health telemetry.
- Do not add permanent tests.

## Implementation Plan

1. Add explicit OpenRouter performance preference validators in
   `internal/provider/http_chat.go`.
2. Reuse the same safe bounded-number logic shape as `max_price`, with a
   positive lower bound and `1000000` upper bound.
3. Extend `validateOpenRouterProvider` to accept
   `preferred_max_latency` and `preferred_min_throughput` only after
   validation.
4. Keep the existing marshal path that forwards the validated provider object
   unchanged as OpenRouter top-level `provider`.
5. Add serve-check helpers in `internal/app/app.go` for exact direct-number and
   percentile-object validation, including raw-token assertions for forwarded
   `json.Number` values.
6. Move performance preference fields out of the unsupported invalid
   OpenRouter cases and add invalid shape cases for both fields.
7. Add non-streaming and streaming success smokes for direct number and object
   forms.
8. Add accepted sentinel-value success smokes plus local response, SQLite,
   `serve --check`, and `manage --check` leak scans.
9. Add a combined smoke for performance preferences plus sort, filters, target
   allow/block lists, and `allow_fallbacks: false`.
10. Add unsupported-provider and no-eligible-cache smokes for performance
    preferences.
11. Review the diff manually before running commands.
12. Run:
    - `find . -name '*_test.go' -type f -print`
    - `git diff --check`
    - `go test ./...`
    - `go vet ./...`
    - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
    - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
    - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Review Questions

1. Is accepting both direct-number and percentile-object forms in one slice
   coherent, given they share the same OpenAPI shape?
2. Is requiring positive finite JSON numbers and rejecting `0` the right strict
   local subset for latency and throughput preferences?
3. Is `1000000` a reasonable upper bound to avoid unbounded numeric payloads
   while staying far above practical latency seconds and tokens-per-second
   thresholds?

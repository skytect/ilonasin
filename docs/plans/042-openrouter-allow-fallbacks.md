# 042 OpenRouter Allow Fallbacks

## Goal

Add narrow OpenRouter-only support for `provider.allow_fallbacks` through the
existing `provider_options.openrouter.provider` escape hatch, without changing
ilonasin's own credential fallback policy or adding broad provider routing.

## Sources

- `AGENTS.md`
- all markdown files under `docs/**`
- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/plans/031-openrouter-require-parameters.md`
- `docs/plans/036-openrouter-privacy-routing.md`
- OpenRouter provider routing documentation,
  `https://openrouter.ai/docs/guides/routing/provider-selection`
- OpenRouter chat completion documentation,
  `https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request`
- current `internal/provider/http_chat.go` provider option validation
- current `internal/app/app.go` direct smoke checks

## Scope

1. Accept `provider_options.openrouter.provider.allow_fallbacks` for OpenRouter
   chat completions.
2. Require `allow_fallbacks` to be a JSON boolean.
3. Accept both `false` and `true` because OpenRouter documents the field as a
   fallback behavior toggle, not a privacy-only assertion.
4. Forward accepted values to OpenRouter as top-level
   `provider.allow_fallbacks`.
5. Keep top-level caller `provider` rejected.
6. Keep `provider_options.openrouter.provider` as an object with at least one
   supported field.
7. Continue rejecting `allow_fallbacks` for DeepSeek and Codex before
   credential resolution.
8. Continue rejecting every OpenRouter provider routing field other than
   `require_parameters`, `data_collection`, `zdr`, and `allow_fallbacks`,
   including `order`, `only`, `ignore`, `sort`, `max_price`,
   `quantizations`, `preferred_max_latency`, `preferred_min_throughput`, and
   `enforce_distillable_text`.
9. Do not store `allow_fallbacks`, provider routing objects, request bodies, or
   raw upstream provider payloads.
10. Do not change ilonasin credential fallback groups, fallback events,
    routing, model addressing, account selection, or retry behavior.
11. Add direct non-streaming and streaming smoke checks for forwarded
    `allow_fallbacks: false`.
12. Add direct smoke checks for `allow_fallbacks: true`, invalid non-boolean
    values, unsupported-provider requests, and no-eligible validation before
    credential resolution.
13. Add leak checks proving marker-bearing unsupported provider routing values
    still do not appear in local errors, SQLite metadata, CLI/TUI output,
    fake-upstream output, or successful responses.

## Non-Goals

- Do not implement OpenRouter `order`, `only`, `ignore`, `sort`, `max_price`,
  `quantizations`, `preferred_max_latency`, `preferred_min_throughput`,
  `enforce_distillable_text`, `models`, `route`, BYOK preferences, region
  routing, or model-level fallback.
- Do not auto-inject `allow_fallbacks` from ilonasin fallback policy settings.
- Do not infer OpenRouter native fallback status from local fallback events.
- Do not add permanent tests.

## Implementation Plan

1. Extend `validateOpenRouterProvider` in `internal/provider/http_chat.go` with
   a boolean-only `allow_fallbacks` case.
2. Keep the existing marshal path that forwards the validated provider object
   unchanged as OpenRouter top-level `provider`.
3. Add small serve-check helpers in `internal/app/app.go` for
   `allow_fallbacks: false`, `allow_fallbacks: true`, and combined provider
   options.
4. Update OpenRouter fake-upstream provider validation to accept and verify
   `allow_fallbacks` as a boolean.
5. Move `provider-allow-fallbacks` out of the unsupported invalid cases and add
   an invalid non-boolean case.
6. Add non-streaming and streaming smoke checks proving
   `provider.allow_fallbacks: false` reaches only the OpenRouter fake upstream.
7. Add a non-streaming smoke check proving `allow_fallbacks: true` is accepted
   and forwarded.
8. Add marker-bearing unsupported-field smoke cases for the remaining
   documented provider routing fields, including latency, throughput, and
   distillable-text filters.
9. Add no-eligible-cache smoke checks proving invalid `allow_fallbacks` and
   unsupported-provider `allow_fallbacks` fail before credential resolution.
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

1. Is accepting both boolean values right for this non-privacy fallback toggle?
2. Does keeping the field under `provider_options.openrouter.provider` preserve
   the existing provider-specific boundary?
3. Are the smoke checks enough to prove this does not alter ilonasin's local
   credential fallback semantics?

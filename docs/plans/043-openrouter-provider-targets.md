# 043 OpenRouter Provider Targets

## Goal

Add narrow OpenRouter-only support for provider target lists
`provider.order`, `provider.only`, and `provider.ignore` through the existing
`provider_options.openrouter.provider` escape hatch, without changing local
credential fallback, model addressing, or storage.

## Sources

- `AGENTS.md`
- all markdown files under `docs/**`
- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/plans/031-openrouter-require-parameters.md`
- `docs/plans/036-openrouter-privacy-routing.md`
- `docs/plans/042-openrouter-allow-fallbacks.md`
- OpenRouter provider routing documentation,
  `https://openrouter.ai/docs/guides/routing/provider-selection`
- OpenRouter chat completion documentation,
  `https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request`
- current `internal/provider/http_chat.go` provider option validation
- current `internal/app/app.go` direct smoke checks

## Scope

1. Accept `provider_options.openrouter.provider.order`,
   `provider_options.openrouter.provider.only`, and
   `provider_options.openrouter.provider.ignore` for OpenRouter chat
   completions.
2. Require each field to be a non-empty JSON array of provider slug strings.
3. Require each provider slug to be non-empty, at most 128 characters, contain
   no control characters, and contain only ASCII letters, ASCII digits,
   underscore, hyphen, dot, or slash.
4. Reject duplicate provider slugs within a single list.
5. Limit each list to at most 32 entries.
6. Forward accepted lists to OpenRouter as top-level
   `provider.order`, `provider.only`, and `provider.ignore`.
7. Allow these lists to combine with existing accepted fields:
   `require_parameters`, `data_collection`, `zdr`, and `allow_fallbacks`.
8. Keep top-level caller `provider` rejected.
9. Continue rejecting provider target lists for DeepSeek and Codex before
   credential resolution.
10. Continue rejecting every OpenRouter provider routing field other than
    `require_parameters`, `data_collection`, `zdr`, `allow_fallbacks`, `order`,
    `only`, and `ignore`, including `sort`, `max_price`, `quantizations`,
    `preferred_max_latency`, `preferred_min_throughput`, and
    `enforce_distillable_text`.
11. Do not store provider target lists, provider routing objects, request
    bodies, or raw upstream provider payloads.
12. Do not change ilonasin credential fallback groups, fallback events,
    routing, model addressing, account selection, retry behavior, or model
    fallback behavior.
13. Add direct non-streaming and streaming smoke checks proving accepted
    provider target lists reach only the OpenRouter fake upstream.
14. Include valid endpoint slugs containing `/`, such as
    `google-vertex/us-east5` or `deepinfra/turbo`, in the success smokes.
15. Add accepted marker-bearing non-streaming and streaming smokes proving valid
    provider slug values are forwarded upstream but not persisted or displayed
    through SQLite metadata, local errors, normalized client output,
    `serve --check`, or `manage --check`.
16. Add direct smoke checks for combined provider targets plus
    `allow_fallbacks: false`.
17. Add invalid and unsupported-provider smoke checks for wrong types, empty
    arrays, empty slugs, duplicate slugs, too many slugs, too-long slugs, and
    invalid slug characters.
18. Add no-eligible-cache smoke checks proving invalid provider target lists
    and unsupported-provider target lists fail before credential resolution.
19. Add leak checks proving marker-bearing invalid and accepted provider target
    values do not appear in local errors, SQLite metadata, CLI/TUI output, or
    successful responses.

## Non-Goals

- Do not implement OpenRouter `sort`, `max_price`, `quantizations`,
  `preferred_max_latency`, `preferred_min_throughput`,
  `enforce_distillable_text`, `models`, `route`, BYOK preferences, region
  routing beyond opaque provider slugs, or model-level fallback.
- Do not auto-inject provider target lists from config, TUI state, credential
  fallback settings, or model cache.
- Do not infer OpenRouter native provider selection from local fallback events.
- Do not add permanent tests.

## Implementation Plan

1. Add a small provider slug-list validator in `internal/provider/http_chat.go`.
2. Extend `validateOpenRouterProvider` to accept `order`, `only`, and `ignore`
   only after slug-list validation.
3. Keep the existing marshal path that forwards the validated provider object
   unchanged as OpenRouter top-level `provider`.
4. Add serve-check helpers in `internal/app/app.go` for exact provider target
   validation.
5. Move `provider-order`, `provider-only`, and `provider-ignore` out of the
   unsupported invalid OpenRouter cases and add invalid shape cases for each.
6. Add non-streaming and streaming success smokes for provider target lists,
   including slash endpoint slugs.
7. Add accepted marker-bearing success smokes plus local response, SQLite,
   `serve --check`, and `manage --check` leak scans.
8. Add a combined smoke for `order` plus `allow_fallbacks: false`.
9. Add unsupported-provider and no-eligible-cache smokes for provider target
   lists.
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

1. Are the proposed provider slug validation rules strict enough without
   blocking documented endpoint slugs such as `google-vertex/us-east5`?
2. Is supporting `order`, `only`, and `ignore` together the right unit because
   OpenRouter documents them as the provider targeting family?
3. Are the smoke checks enough to prove this does not alter local credential
   fallback semantics?

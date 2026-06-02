# 250 Explicit Generic Model Addresses

## Goal

Finish the explicit model-addressing cleanup for the generic OpenAI-compatible
routes, while preserving the compatibility alias needed by Codex clients.

The architecture requires client model strings to be:

```text
<provider_instance_id>/<provider_model_id>
```

Anthropic routes already reject bare aliases after plan 248. Generic chat and
responses routes may accept a bare model ID only when the model cache has
exactly one exact provider match. This is not hidden cross-provider routing:
zero matches and multiple matches both fail with `invalid_model`.

## Scope

1. Keep `internal/server/model_resolution.go` explicit-address first:
   `provider/model` strings are parsed using `routing.ParseModelAddress`.
2. If the model has no slash, resolve it through `model_cache` only when there
   is exactly one exact `model_id` match. Do not do substring matching,
   provider registry probing, provider-class guessing, or cross-model aliases.
3. Keep `/v1/chat/completions` and `/v1/responses` error behavior as a `400`
   `invalid_model` when the model string is malformed, absent from cache, or
   ambiguous.
   These requests should preserve the current early metadata-only
   `invalid_model` recording behavior.
4. Keep addressed-but-unconfigured providers as the existing `404`
   `provider_not_configured` behavior in each route.
5. Keep `/v1/models` free to advertise namespaced IDs. Bare request aliases are
   a server-side compatibility behavior, not a reason to publish ambiguous
   unqualified model names.
6. Do not change provider adapters, model discovery/cache storage, management
   APIs, TUI, Anthropic routes, credentials, IO logging, or fallback policy.
7. Do not add permanent tests.

## Verification

1. Temporary focused smoke, removed before commit:
   - `resolveModelAddress("codex/gpt-5.5")` returns provider `codex`, model
     `gpt-5.5`.
   - `resolveModelAddress("gpt-5.5")` returns provider `codex`, model
     `gpt-5.5` when the cache contains exactly one matching row.
   - `resolveModelAddress("gpt-5.5")` returns the parse error when the cache has
     zero matching rows or more than one exact provider match.
   - `resolveModelAddress("codex/")`, `resolveModelAddress("/gpt-5.5")`, and
     `resolveModelAddress("")` remain invalid.
   - `modelsResponseFromMetadata` emits namespaced `data[].id` and
     namespaced `models[].slug`.
   - rows with empty provider `DisplayName` emit namespaced
     `models[].display_name`.
   - `rg -n 'ListModelCache|strings\.Contains\(model|resolveModelAddress\(r\.Context' internal/server/model_resolution.go internal/server/*_route.go` confirms the cache lookup remains confined to the model resolver.
   - a temporary route-level `/models` and `/v1/models` smoke with seeded model
     cache confirms HTTP response `data[].id`, `models[].slug`, and
     empty-display-name fallback values are namespaced.
2. Standard checks:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
3. Temporary daemon smoke:
   - build `ilonasin`,
   - start `ilonasin serve` with a temporary home and config,
   - create a temporary local client token through the management API,
   - smoke management health,
   - smoke `ilonasin manage` under a short PTY timeout,
   - GET `/models` and `/v1/models` with auth and confirm `data[].id` and
     `models[].slug` values are namespaced when rows are available,
   - POST `/v1/chat/completions` with bare `gpt-5.5` and a unique cached match
     reaches provider dispatch,
   - POST `/v1/responses` with bare `gpt-5.5` and a unique cached match reaches
     provider dispatch,
   - POST `/v1/chat/completions` with addressed `codex/gpt-5.5` and confirm it
     reaches provider dispatch, which in a temporary home may return
     `credential_unavailable` rather than `invalid_model`.
   - POST `/v1/responses` with addressed `codex/gpt-5.5` and confirm it
     reaches provider dispatch, which in a temporary home may return
     `credential_unavailable` rather than `invalid_model`.
4. Remove all temporary smoke artifacts.

## Acceptance

- Generic local API routes infer provider instance from model cache only for a
  unique exact bare model ID match.
- Ambiguous and missing bare model IDs fail with `invalid_model`.
- Explicit addressed models keep the previous route behavior.

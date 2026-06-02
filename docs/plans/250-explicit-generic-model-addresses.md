# 250 Explicit Generic Model Addresses

## Goal

Finish the explicit model-addressing cleanup for the generic OpenAI-compatible
routes.

The architecture requires client model strings to be:

```text
<provider_instance_id>/<provider_model_id>
```

Anthropic routes already reject bare aliases after plan 248. The generic chat
and responses routes still accept a unique bare model ID from the model cache,
and `/v1/models` can still advertise bare Codex-compatible `models[].slug`
values when a response-capable model ID is unique. Both behaviors encourage
implicit routing.

## Scope

1. Change `internal/server/model_resolution.go` so
   `Server.resolveModelAddress` only parses explicit model addresses using
   `routing.ParseModelAddress`.
2. Remove model-cache lookup, ambiguity handling, and provider registry probing
   from generic request model resolution.
   Remove the dead `context.Context` parameter from the helper and update chat
   and responses call sites.
3. Keep `/v1/chat/completions` and `/v1/responses` error behavior as a `400`
   `invalid_model` when the model string is bare or malformed.
   These requests should preserve the current early metadata-only
   `invalid_model` recording behavior.
4. Keep addressed-but-unconfigured providers as the existing `404`
   `provider_not_configured` behavior in each route.
5. Verify the already-implemented `internal/server/models_response.go`
   behavior that OpenAI `data[].id`, Codex-compatible `models[].slug`, and empty
   `display_name` fallbacks are namespaced as
   `<provider_instance_id>/<provider_model_id>`.
6. Do not change provider adapters, model discovery/cache storage, management
   APIs, TUI, Anthropic routes, credentials, IO logging, or fallback policy.
7. Do not add permanent tests.

## Verification

1. Temporary focused smoke, removed before commit:
   - `resolveModelAddress("codex/gpt-5.5")` returns provider `codex`, model
     `gpt-5.5`.
   - `resolveModelAddress("gpt-5.5")` returns the parse error even if a fake
     cache contains a unique `codex/gpt-5.5` row.
   - `resolveModelAddress("codex/")`, `resolveModelAddress("/gpt-5.5")`, and
     `resolveModelAddress("")` remain invalid.
   - `modelsResponseFromMetadata` emits namespaced `data[].id` and
     namespaced `models[].slug`.
   - rows with empty provider `DisplayName` emit namespaced
     `models[].display_name`.
   - No bare response-capable model slug appears in the temporary response.
   - `rg -n 'ListModelCache|strings\.Contains\(model|resolveModelAddress\(r\.Context' internal/server/model_resolution.go internal/server/*_route.go` returns no stale generic request-resolution references.
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
   - POST `/v1/chat/completions` with bare `gpt-5.5` and confirm `400`
     `invalid_model`,
   - POST `/v1/responses` with bare `gpt-5.5` and confirm `400`
     `invalid_model`,
   - POST `/v1/chat/completions` with addressed `codex/gpt-5.5` and confirm it
     reaches provider dispatch, which in a temporary home may return
     `credential_unavailable` rather than `invalid_model`.
   - POST `/v1/responses` with addressed `codex/gpt-5.5` and confirm it
     reaches provider dispatch, which in a temporary home may return
     `credential_unavailable` rather than `invalid_model`.
4. Remove all temporary smoke artifacts.

## Acceptance

- Generic local API routes no longer infer provider instance from model cache.
- Model list output no longer advertises bare Codex-compatible slugs.
- Explicit addressed models keep the previous route behavior.

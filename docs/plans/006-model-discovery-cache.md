# Plan 006: Model Discovery Cache

## Goal

Replace the stubbed `GET /v1/models` response with provider-backed model
discovery for API-key DeepSeek and OpenRouter instances, backed by the
architecture's SQLite `model_cache` concept and exposed in the management TUI.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/plans/001-initial-go-scaffold.md`
- `docs/plans/002-local-api-tokens.md`
- `docs/plans/003-upstream-api-key-credentials.md`
- `docs/plans/004-nonstreaming-chat-adapters.md`
- `docs/plans/005-streaming-chat-adapters.md`
- `AGENTS.md`

## Scope

1. Add a provider model-discovery boundary:
   - DeepSeek and OpenRouter HTTP adapters can call provider `/models`,
   - adapters receive resolved API-key credentials only at the adapter boundary,
   - adapters return normalized model metadata without raw provider payloads.
2. Implement `GET /models` upstream discovery for:
   - DeepSeek base URL plus `/models`,
   - OpenRouter base URL plus `/models`.
3. Keep Codex placeholder-only:
   - no Codex auth import,
   - no Codex/OpenAI environment auth import,
   - no keyring/file/cookie inspection,
   - no subscription, agent identity, or Codex model discovery implementation.
4. Add typed normalized model metadata:
   - provider instance ID,
   - provider model ID,
   - local model ID as `<provider_instance_id>/<provider_model_id>`,
   - display name when safely present,
   - context length when safely present,
   - capability flags derived from safe allowlisted fields.
5. Store normalized model cache rows in SQLite:
   - one row per `(provider_instance_id, model_id)`,
   - no raw provider model payload,
   - no pricing JSON,
   - no provider descriptions,
   - no request IDs, account IDs, balances, or credential material.
6. Make authenticated local `GET /v1/models`:
   - resolve eligible credentials for API-key provider instances,
   - refresh model cache through the adapter for providers with credentials,
   - return an OpenAI-compatible list with local namespaced IDs,
   - skip placeholder providers and providers without eligible credentials,
   - fall back to existing cache for a provider when live refresh fails,
   - never expose provider credentials or raw upstream error bodies.
7. Return exact local `GET /v1/models` JSON:
   - top-level `object: "list"`,
   - `data[]` array,
   - each entry has only `id`, `object`, and `owned_by`,
   - `id` is `<provider_instance_id>/<provider_model_id>`,
   - `object` is `"model"`,
   - `owned_by` is the provider instance ID,
   - cache-only fields such as display name, context length, and capability
     flags are not exposed in the OpenAI-compatible response.
8. Use exact model discovery and cache fallback semantics:
   - placeholder providers are always skipped,
   - providers without eligible credentials are skipped in `/v1/models` even if
     stale cache rows exist,
   - providers with eligible credentials attempt live refresh,
   - live refresh success requires a non-empty normalized model list,
   - successful refresh atomically replaces cache rows for that provider,
   - failed, malformed, empty, or too-large refreshes must not delete or replace
     existing cache rows,
   - if refresh fails and cache rows exist for that credentialed provider,
     return those cache rows,
   - if one provider succeeds or has cache and another provider fails without
     cache, return the available models with HTTP `200`,
   - if no provider has eligible credentials, return HTTP `200` with an empty
     list,
   - if at least one provider had an eligible credential but every attempted
     refresh failed and no fallback cache rows exist, return a coarse local
     `502` OpenAI-style error.
9. Return model list entries in deterministic order:
   - provider instance ID order as exposed by the current registry,
   - then provider model ID ascending.
10. Extend `ilonasin manage` with a model cache summary:
   - show cached model count per provider instance,
   - show last cache update timestamp per provider instance when present,
   - do not show raw provider payloads or descriptions,
   - TUI still mutates SQLite only, not `config.toml`.
11. Extend `serve --check` fake TLS upstream coverage:
   - DeepSeek `/models` response,
   - OpenRouter `/api/v1/models` response,
   - bearer auth,
   - local namespaced model IDs,
   - cache persistence,
   - fallback to cache after a fake refresh failure,
   - no selected-home credential rows.
12. Extend `manage --check` to prove model cache rows are listed in the TUI
    summary without leaking provider payload text.

## Out of Scope

- Provider model endpoint details beyond `/models`.
- OpenRouter `/models/user`, `/providers`, and per-model endpoint discovery.
- Provider health events for model discovery failures.
- Background refresh jobs or cache TTL policy.
- User-triggered interactive model refresh controls.
- Model-level routing policy or capability enforcement.
- Pricing/cost normalization.
- OAuth credentials.
- Codex adapter implementation.
- Live provider network smoke calls.
- Permanent test files.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` is used only as a package compile check.
- Provider adapters must not import the TUI or SQLite.
- Server must depend on narrow adapter/resolver/cache interfaces, not storage
  concrete types.
- TUI may depend on a model-cache reader interface, not SQLite details.
- Request/response body bytes may exist only transiently for parsing,
  validation, cache normalization, and response writing.
- Never log or persist prompts, completions, raw bodies, raw provider payloads,
  raw stream chunks, tool arguments/results, full bearer tokens, full provider
  request IDs, full account IDs, balances, credit totals, raw pricing, or
  provider descriptions.
- Upstream `/models` response bodies are not forwarded to local clients.
- Local `/v1/models` errors use OpenAI-style envelopes without raw upstream
  payloads.
- Provider model discovery calls have a bounded timeout and a bounded response
  body. Initial bounds are 30 seconds and 16 MiB.
- Too-large, invalid JSON, missing `data`, non-array `data`, empty normalized
  result, and transport failures are normalized as body-free discovery errors.
- Model IDs from providers must be non-empty strings and must not contain
  control characters. Invalid entries are skipped; an otherwise empty live list
  is a coarse `upstream_invalid_response`.
- For DeepSeek, safe fields are `id` and optional `owned_by` only.
- For OpenRouter, safe fields are `id`, `name`, `context_length`, and
  allowlisted `supported_parameters`.
- Capability flags are stored as a deterministic comma-separated sorted string.
- OpenRouter `supported_parameters` mapping:
  - `tools` -> `tools`,
  - `tool_choice` -> `tools`,
  - `response_format` -> `json_object`,
  - `logprobs` -> `logprobs`,
  - `top_logprobs` -> `logprobs`,
  - `reasoning` -> `reasoning`,
  - `stream` or a model entry observed through the chat surface -> `stream`,
  - all discovered OpenRouter models get `chat` because they come from the chat
    model list used by this slice.
- DeepSeek `/models` does not expose capability fields in the safe allowlist.
  DeepSeek model rows get provider-default flags `chat,json_object,reasoning,stream,tools`
  derived from the checked docs, not from raw upstream payloads.
- Do not store raw `supported_parameters`, `architecture`, `pricing`, or
  descriptions.
- Cache fallback is only for model discovery. It must not imply chat fallback,
  credential fallback, cross-provider routing, or hidden account rotation.
- If a provider has no eligible credential and no cache, it contributes no
  local `/v1/models` entries.

## Proposed Package Changes

```text
internal/provider/
  chat.go       # add model discovery interfaces/types
  http_chat.go  # HTTP /models discovery implementation
internal/server/
  server.go     # /v1/models refresh/cache response handling
internal/storage/sqlite/
  db.go         # model cache upsert/list methods
internal/tui/
  tui.go        # model cache summary view
internal/app/
  app.go        # fake upstream model discovery smoke checks
```

Interface shape:

```go
type ModelDiscoverer interface {
    ListModels(ctx context.Context, req ModelRequest) (ModelResult, error)
}

type ModelDiscoverers interface {
    ForProvider(providerType string) (ModelDiscoverer, bool)
}

type ModelCache interface {
    ReplaceModelCache(ctx context.Context, providerInstanceID string, models []provider.ModelMetadata) error
    ListModelCache(ctx context.Context) ([]provider.ModelMetadata, error)
}
```

The concrete HTTP adapter may implement both chat and model discovery. The
server uses discovery through the provider boundary and persistence through a
cache interface.

The server must not type-assert concrete HTTP adapters or import SQLite. The
server-facing provider registry must expose `List()` so the handler can iterate
configured provider instances in registry order.

## Verification

Run:

```text
find . -name '*_test.go' -type f
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Smoke checks must prove:

- no permanent tests exist,
- no real provider network call happens during `serve --check`,
- local `/v1/models` requires an ilonasin local bearer token,
- the real HTTP adapter calls fake DeepSeek `/models` and OpenRouter
  `/api/v1/models`,
- upstream `/models` uses provider API-key bearer auth,
- upstream `/models` enforces timeout and body-size bounds,
- local `/v1/models` returns namespaced IDs such as
  `deepseek/deepseek-v4-pro` and `openrouter/deepseek/deepseek-v4-pro`,
- local `/v1/models` response entries contain only `id`, `object`, and
  `owned_by`,
- model cache rows are written with provider model IDs, display names when
  safe, context length when safe, capability flags, and timestamps,
- a fake refresh failure falls back to previously cached rows without exposing
  raw upstream error bodies,
- failed, malformed, empty, and too-large refreshes do not wipe existing cache,
- a credentialed provider with no cache and a failed refresh returns a coarse
  local `502` when no other provider contributes models,
- providers without eligible credentials do not contribute stale cache rows to
  local `/v1/models`,
- malformed model entries are skipped or normalized according to the plan,
- selected home DB has zero check-created local/upstream credentials after
  `serve --check` and `manage --check`,
- `manage --check` prints a model cache summary,
- `manage --check` seeds cache rows in an isolated temporary DB and proves the
  selected home DB is not polluted,
- `manage --check` asserts count/timestamp output and no raw description,
  pricing, or payload markers leak,
- check output does not contain fake provider API keys, local tokens, raw
  provider payloads, raw pricing, descriptions, request IDs, account IDs,
  balances, prompts, or completions.

## Review Questions

1. Is live refresh-on-`GET /v1/models` acceptable for the MVP, with cache
   fallback, or should refresh be TUI-only?
2. Are the normalized model metadata fields safe and useful enough for later
   routing and TUI work?
3. Does the model-cache interface keep server, provider, storage, and TUI
   boundaries clean?
4. Are the smoke checks strong enough without permanent tests or real provider
   calls?

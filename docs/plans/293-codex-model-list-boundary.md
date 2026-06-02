# 293 Codex Model List Boundary

## Goal

Move Codex-compatible `/v1/models` metadata shaping out of the server package and
behind the provider boundary.

The architecture says provider adapters own provider-specific behavior, while
the router core should not embed provider quirks beyond selecting adapters and
passing typed route options. Current `internal/server/models_response.go` still
defines Codex-specific model DTOs, reasoning-level defaults, service-tier
fallbacks, modality ordering, and `models[]` construction.

## Scope

1. Add a provider-owned helper for Codex-compatible model-list items built from
   `provider.ModelMetadata`.
2. Move these server-owned Codex presentation details into `internal/provider`:
   - the Codex `models[]` DTO,
   - reasoning-effort DTO/defaults,
   - response-capable filtering, preserving the current capability-based behavior
     where any row with `responses` capability appears in the Codex-compatible
     `models[]`, regardless of provider instance type,
   - Codex display-name fallback,
   - service-tier and input-modality fallbacks used by Codex-compatible clients.
3. Keep `internal/server/models_response.go` responsible only for the generic
   OpenAI-compatible envelope:
   - top-level `object`,
   - OpenAI `data[]` rows,
   - deterministic sorting,
   - calling the provider helper for Codex-compatible `models[]`.
4. Preserve the exact JSON field names and current values for `/models` and
   `/v1/models`.
5. Do not change model discovery, cache storage, provider HTTP parsing,
   management DTOs, TUI, route auth, config, or credentials.
6. Do not add permanent tests.

## Verification

1. Source checks:
   - `rg -n 'codexModelInfo|defaultCodexReasoningEfforts|codexFastServiceTier|reasoningEffort|capabilityList|orderedInputModalities|displayNameOrID|SupportsSearchTool' internal/server`
     returns no matches.
   - `rg -n 'Codex.*Model|CodexCompatible' internal/provider internal/server`
     shows Codex-specific model-list shaping lives in `internal/provider`, with
     server only calling the helper.
2. Temporary focused smoke, removed before commit:
   - construct representative `provider.ModelMetadata` rows for Codex,
     DeepSeek, and OpenRouter,
   - confirm `modelsResponseFromMetadata` keeps namespaced `data[].id`,
   - confirm only response-capable rows appear in `models[]`,
   - confirm response-capable non-Codex provider rows still appear in `models[]`,
   - confirm Codex-compatible `slug`, `display_name`, reasoning levels, service
     tiers, input modalities, and context windows match the pre-change behavior.
   - confirm full `models[]` JSON preserves null fields, empty arrays, omitted
     empty service tiers, default verbosity, search/tool fields, and truncation
     policy.
3. Standard checks:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
4. Daemon/manage smoke:
   - build a temporary `ilonasin`,
   - run `serve` with a temporary explicit config on a free port,
   - verify management health over the Unix socket,
   - create a local token over the management API,
   - run authenticated `GET /models` and `GET /v1/models` checks against seeded
     model cache rows and confirm the JSON fields above remain unchanged,
   - run `manage --config "$tmp/config.toml"` under short wide and narrow PTY
     timeouts and confirm API, providers, usage, and logs render.

## Acceptance

- Server model response construction no longer owns Codex-specific DTOs or
  defaults.
- Public model-list JSON remains unchanged.
- Compile, vet, serve, and manage smokes pass.

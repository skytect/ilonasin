# 410 Models Response OpenAI Boundary

## Context

Whole-codebase review flagged `internal/server/models_response.go` because the
server response struct directly names `provider.CodexModelInfo` for the
Codex-compatible `models` extension returned by `/models` and `/v1/models`.

`docs/ilonasin-architecture.md` treats `/models` as a local compatibility API
surface and says provider adapters own provider-specific behavior. The server
should orchestrate discovery and write JSON, but it should not own a
Codex-specific response shape.

## Goal

Move `/models` response DTO construction into the OpenAI compatibility boundary
while preserving the exact JSON response shape.

## Scope

1. Add an OpenAI-owned neutral model metadata DTO and model-list response
   constructor.
   - Do not import `internal/provider` into `internal/openai`, because provider
     already depends on OpenAI request/response types.
   - Inputs: `[]openai.ModelMetadata`.
   - Preserve the current `object`, `data`, and `models` JSON fields.
   - Preserve namespaced IDs as `provider_instance_id/model_id`.
   - Preserve sorting by provider instance ID then model ID.
   - Preserve Codex-compatible `models` extension content with an OpenAI-owned
     DTO that mirrors the current JSON shape.
2. Add a narrow server conversion from `provider.ModelMetadata` to
   `openai.ModelMetadata`.
   - This conversion should be mechanical and contain no Codex-specific
     response DTOs.
   - It may copy capability flags, service tiers, context length, display name,
     and input modalities needed by the OpenAI response constructor.
3. Update `internal/server/models_response.go`.
   - Remove server-owned response DTOs that directly name
     `provider.CodexModelInfo`.
   - Keep only a thin server wrapper if needed, or delete the file if all logic
     moves cleanly.
4. Update `internal/server/models.go`.
   - Call the OpenAI-owned constructor and keep logging model count behavior
     unchanged.
5. Keep behavior unchanged.
   - No model discovery, model cache, provider adapter, route, auth, logging,
     management, TUI, config, or storage behavior changes.
   - No permanent tests.

## Verification

- Temporary focused checks must prove current response behavior is preserved;
  remove them before commit.
- Include checks for:
  - sorted `data` rows by provider instance then model ID;
  - `data.id`, `data.object`, and `data.owned_by` values;
  - Codex extension rows appear only for metadata with `responses`
    capability;
  - Codex extension preserves namespaced slug, display name fallback, service
    tier fallback, context windows, input modalities, and reasoning support.
  - `go list ./internal/openai ./internal/provider ./internal/server` proves no
    new import cycle.
- Run `git diff --check`.
- Run `find . -name '*_test.go' -type f -print`.
- Run `go test ./internal/openai ./internal/server`.
- Run `go test ./...`.
- Run `go vet ./...`.
- Build `ilonasin`.
- Smoke `ilonasin serve` with an isolated `ILONASIN_HOME`.
- Smoke `ilonasin manage` against that daemon at narrow and wide terminal
  widths.

## Non-Goals

- Changing provider model discovery normalization.
- Changing Codex model metadata shaping.
- Changing model cache storage or conversion.
- Changing `/models` route auth, path handling, or logging.
- Removing the Codex-compatible `models` JSON extension.

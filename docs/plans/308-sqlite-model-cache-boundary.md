# 308 SQLite Model Cache Boundary

## Context

`docs/ilonasin-architecture.md` keeps provider adapters, SQLite storage, server
routes, and management DTOs as separate boundaries. The model cache still
crosses those boundaries:

- `internal/storage/sqlite/model_cache.go` imports `internal/provider`;
- SQLite `ReplaceModelCache` and `ListModelCache` accept and return
  `provider.ModelMetadata`;
- management snapshot interfaces also expose provider model metadata;
- the schema only persists a subset of provider model metadata, so cached Codex
  model-list fallback can lose service tier and input modality details that the
  live response had.

This slice should make model-cache persistence storage/metadata-owned while
preserving `/v1/models` behavior. It must not change model discovery HTTP
transport, provider normalization, routing selection, API auth, config loading,
TUI behavior, or unrelated storage tables.

## Plan

1. Introduce a neutral metadata-owned model-cache row type in
   `internal/metadata` with the persisted fields needed by server and
   management:
   - provider instance ID;
   - model ID;
   - display name;
   - capability flags;
   - context length;
   - default service tier;
   - normalized service tiers;
   - normalized input modalities;
   - updated time.
   SQLite persists service tiers and input modalities as JSON, but those JSON
   fields must be generated from normalized, allowlisted metadata. They must
   not persist raw upstream model payload fragments.
2. Add SQLite migration steps for nullable or defaulted
   `default_service_tier`, `service_tiers_json`, and `input_modalities_json`
   columns on `model_cache`.
3. Change `internal/storage/sqlite/model_cache.go` to read/write only the
   metadata row type and remove its `internal/provider` import.
4. Change `internal/server` model-cache interface to use metadata rows. Convert
   live `provider.ModelMetadata` to metadata rows only at the server cache
   write boundary, and convert cached metadata rows back to provider metadata
   only where `/v1/models` response construction needs provider-specific Codex
   shaping.
5. Change management model-cache reader and snapshot conversion to use metadata
   rows directly, avoiding provider DTOs in management interfaces.
6. Keep the existing `/v1/models` JSON shape stable. The only intended behavior
   improvement is that cached Codex rows preserve service tiers, default service
   tier, and input modalities across fallback.
7. Review the code before checks for migration idempotence, invalid JSON
   handling, provider import removal from SQLite and management interfaces, and
   no unrelated behavior changes.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/storage/sqlite
go test ./internal/server
go test ./internal/management
go test ./...
go vet ./...
! rg -n '"ilonasin/internal/provider"' internal/storage/sqlite/model_cache.go
! rg -n 'provider\.ModelMetadata|modelMetadataFromProvider' internal/management
```

Run a temporary smoke and remove it before commit. It must:

- open a temporary SQLite store and confirm migrations add the new model cache
  columns idempotently;
- insert a metadata model-cache row with default service tier, service tiers
  JSON, and input modalities JSON;
- read it back through `ListModelCache`;
- convert it through server `/v1/models` response construction and confirm the
  Codex model info preserves service tier and input modality fields;
- confirm invalid stored service tier or input modality JSON does not panic and
  does not leak raw provider payloads;
- confirm unknown service tier IDs and unknown input modality values are
  dropped on cache read or conversion instead of being returned.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temp directory.

## Acceptance

- SQLite model-cache code no longer imports or exposes provider DTOs.
- Management model-cache reader no longer exposes provider DTOs.
- Server owns provider-to-cache and cache-to-provider conversion for the model
  route boundary.
- Cached Codex model rows preserve service tier and input modality metadata
  needed by `/v1/models`.
- Model-cache JSON columns contain canonical allowlisted metadata only, never
  raw upstream model payload fragments.
- Existing model discovery and management snapshot behavior remains compatible.

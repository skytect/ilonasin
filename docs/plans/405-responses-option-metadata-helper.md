# 405 Responses Option Metadata Helper

## Context

`docs/ilonasin-architecture.md` treats request metadata as metadata-only
observability. Plan 404 split Chat provider-option metadata into focused
helpers, but Responses option metadata still has duplicated extraction in:

- `earlyResponsesRequestMetadata`;
- `responsesRequestMetadataBase`.

Both paths record the same allowlisted Responses request options:

- top-level `service_tier`;
- `reasoning.effort`;
- `reasoning.summary`.

This duplication makes the metadata-only boundary harder to audit and risks
future drift between early error metadata and normal Responses request metadata.

## Goal

Centralize Responses request-option metadata extraction behind one private
server helper while preserving all recorded values exactly.

## Scope

1. Add a private helper in `internal/server/request_metadata_options.go`, for
   example `applyResponsesOptionMetadata(out *metadata.Request, req
   openai.ResponsesRequest)`.
2. Use the helper in `earlyResponsesRequestMetadata`.
3. Use the helper in `responsesRequestMetadataBase`.
4. Preserve exact behavior:
   - allowlisted `service_tier` values still populate `RequestedServiceTier`;
   - invalid service tiers still record an empty requested tier;
   - allowlisted `reasoning.effort` still populates `ReasoningEffort`;
   - invalid reasoning efforts still record empty effort;
   - allowlisted `reasoning.summary` still populates `ReasoningSummary`;
   - invalid reasoning summaries still record empty summary;
   - Responses metadata does not set `EffectiveServiceTier`.
5. Do not change request parsing, validation, Chat metadata, Anthropic metadata,
   provider adapters, storage, schema, management routes, TUI, config, IO
   logging, routing, public routes, or metadata field names.

## Verification

Use temporary focused checks, then remove them before commit:

- early Responses metadata and base Responses metadata record the same
  allowlisted `service_tier`, `reasoning.effort`, and `reasoning.summary`;
- invalid service tier, effort, and summary values remain empty in both paths;
- `EffectiveServiceTier` remains empty for Responses metadata.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Responses request-option metadata extraction has one implementation.
- Early error and normal Responses metadata cannot drift for these fields.
- No runtime metadata behavior changes.
- No permanent tests are added.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

# 404 Request Metadata Option Helpers

## Context

`docs/ilonasin-architecture.md` treats request metadata as metadata-only
observability and says provider adapters own provider-specific behavior. The
server may record sanitized route options, but it should keep that recording
auditable and avoid broad provider-specific switch blocks that are hard to
review.

`internal/server/request_metadata_options.go` already owns sanitized option
metadata extraction, but `applySafeOptionMetadata` still mixes:

- common top-level service tier metadata,
- Codex provider option metadata,
- DeepSeek provider option metadata,
- OpenRouter provider option metadata.

This is a behavior-preserving modularity slice.

## Goal

Split provider-specific request-option metadata extraction into small private
helpers while preserving every recorded metadata field exactly.

## Scope

1. Keep `applySafeOptionMetadata` as the public helper used by request metadata
   builders.
2. Extract common top-level service-tier handling into a private helper.
3. Extract provider-specific option handling into private helpers:
   - `applyCodexOptionMetadata`;
   - `applyDeepSeekOptionMetadata`;
   - `applyOpenRouterOptionMetadata`.
4. Preserve exact behavior:
   - top-level chat `service_tier` still sets requested and effective service
     tier when allowlisted;
   - top-level `service_tier=default` for Codex still leaves effective tier
     empty unless Codex provider options override it;
   - Codex `provider_options.codex.service_tier=fast` still maps effective tier
     to `priority`;
   - Codex reasoning effort and summary metadata are unchanged;
   - DeepSeek reasoning effort and thinking type metadata are unchanged;
   - OpenRouter reasoning effort, max tokens, enabled, and exclude metadata are
     unchanged;
   - invalid or non-allowlisted values remain empty or unset.
5. Do not change request parsing, validation, provider adapters, storage,
   schema, management routes, TUI, config, IO logging policy, routing, public
   routes, or metadata field names.

## Verification

Use temporary focused checks, then remove them before commit:

- Codex top-level `service_tier=default` still records requested `default` and
  empty effective tier.
- Non-Codex top-level `service_tier=default` still records requested `default`
  and effective `default`.
- Codex provider option `service_tier=fast` still records requested `fast` and
  effective `priority`.
- Codex reasoning effort and summary are unchanged.
- DeepSeek reasoning effort and thinking type are unchanged.
- OpenRouter reasoning effort, reasoning max tokens, enabled, and exclude are
  unchanged.
- Invalid service tiers and invalid reasoning values stay empty.

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

- Provider-option metadata extraction is easier to audit by provider.
- No runtime metadata behavior changes.
- No permanent tests are added.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

# 407 Chat Option Metadata Provider Boundary

## Context

`docs/ilonasin-architecture.md` says provider adapters own provider-specific
behavior, including token/cost/cache metadata extraction, and the router core
should not embed provider-specific quirks beyond selecting provider instances
and models.

`internal/server/request_metadata_options.go` still interprets Chat
`provider_options` vocabulary for Codex, DeepSeek, and OpenRouter. That keeps
provider-specific option names and allowed metadata values in the server
boundary even though the provider package already validates and marshals those
same provider option objects.

## Goal

Move Chat provider-option metadata extraction into `internal/provider` while
preserving current request metadata behavior.

## Scope

1. Add a provider-owned DTO for safe Chat option metadata.
   - Keep it independent from `internal/metadata` so the provider package does
     not depend on storage or management DTOs.
   - Include only existing metadata fields:
     requested service tier, effective service tier, reasoning effort,
     reasoning summary, reasoning max tokens, reasoning enabled, reasoning
     exclude, and thinking type.
2. Add a provider-owned extraction function for Chat requests.
   - Inputs: provider type and `openai.ChatCompletionRequest`.
   - Preserve current top-level service-tier behavior, including Codex
     `default` not becoming an effective tier.
   - Preserve current early-error Chat metadata behavior where provider type is
     unknown (`""`) and top-level `service_tier=default` records requested
     `default` and effective `default`.
   - Preserve current Codex `provider_options.codex.service_tier` mapping where
     `fast` has effective `priority`.
   - Preserve current Codex, DeepSeek, and OpenRouter reasoning/thinking fields.
   - Preserve current sanitization allowlists exactly.
3. Update `internal/server/request_metadata_options.go`.
   - Server should call the provider-owned extractor and copy safe DTO fields
     into `metadata.Request`.
   - Leave Responses option metadata in server for this slice, because local
     Responses metadata is not a provider adapter Chat concern.
4. Keep behavior unchanged.
   - No config, database, routing, TUI, management API, logging, or provider
     request marshaling changes.
   - No permanent tests.

## Verification

- Temporary focused checks must be used to compare old and new metadata
  behavior; remove them before commit.
- Include focused behavior checks for the full moved truth table:
  - unknown provider type (`""`) with top-level `service_tier=default`;
  - non-Codex provider type with top-level `service_tier=default`;
  - Codex top-level `service_tier=default`, with no effective tier drift;
  - valid and invalid top-level service tiers;
  - Codex `provider_options.codex.service_tier=fast` mapping to effective
    `priority`;
  - Codex reasoning effort and summary;
  - DeepSeek reasoning effort and thinking type;
  - OpenRouter reasoning effort, max tokens, enabled, and exclude;
  - invalid provider-option metadata values remaining unrecorded.
- Run `git diff --check`.
- Run `find . -name '*_test.go' -type f -print`.
- Run `go test ./internal/provider ./internal/server`.
- Run `go test ./...`.
- Run `go vet ./...`.
- Build `ilonasin`.
- Smoke `ilonasin serve` with an isolated `ILONASIN_HOME`.
- Smoke `ilonasin manage` against that daemon at narrow and wide terminal
  widths.

## Non-Goals

- Changing which option metadata is recorded.
- Moving Responses option metadata.
- Changing provider option validation or upstream marshaling.
- Adding new request metadata fields.
- Changing credential pooling, affinity, quota, fallback, or TUI rendering.

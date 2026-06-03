# 417 Model Capability Flags Boundary

## Context

Whole-codebase review found that model capability flag strings are duplicated
across provider normalization and OpenAI model response shaping:

- `internal/provider/http_models.go` hardcodes DeepSeek and Codex fallback model
  capability strings and builds Codex capability sets with raw literals.
- `internal/provider/openrouter_metadata.go` builds OpenRouter capability sets
  with raw literals.
- `internal/openai/models_response.go` reparses the persisted comma-separated
  flag string and checks raw literals for Codex model response enrichment.

`docs/ilonasin-architecture.md` treats model cache rows and capability metadata
as metadata-domain state. The persisted SQLite field and public model JSON
should remain a comma-separated string, but the vocabulary and parse/format
helpers should have one owner.

## Goal

Move model capability flag vocabulary and parse/format helpers into
`internal/metadata`, then make provider normalization and OpenAI response
shaping consume that shared metadata-domain boundary without changing storage,
wire JSON, ordering, or model behavior.

## Scope

1. Add a small `internal/metadata/model_capabilities.go` file.
   - Export constants for the capability values currently emitted or consumed:
     `advanced_sampling`, `cache_control`, `chat`, `json_object`, `logit_bias`,
     `logprobs`, `metadata`, `model_fallbacks`, `parallel_tool_calls`,
     `prediction`, `reasoning`, `responses`, `sampling`, `service_tier`,
     `session_id`, `stream`, `tools`, `user`, and `vision`.
   - Add `FormatModelCapabilities(values ...string) string`.
     It trims, drops empty values, deduplicates, sorts lexicographically, and
     joins with commas.
   - Add `ParseModelCapabilities(flags string) []string`.
     It trims comma-separated values and drops blanks, preserving unknown values
     so existing stored rows remain readable.
   - Add `HasModelCapability(flags string, capability string) bool` for callers
     that only need membership.
2. Update provider model normalization to use the shared constants and formatter.
   - DeepSeek and generic Codex fallback strings must stay exactly the same.
   - Codex metadata-derived capability strings must stay sorted and include the
     same optional flags.
   - OpenRouter supported-parameter mapping must produce the same flags.
3. Update OpenAI `/models` response shaping to use `metadata.HasModelCapability`
   and shared constants instead of local parsing helpers and string literals.
4. Remove now-dead local model capability parsing helpers from
   `internal/openai/models_response.go`.
5. Do not change `NormalizeModelCacheRow`, SQLite schema, model cache storage,
   management DTOs, TUI output, routing, provider option validation, request
   handling, logging, or public response shapes.
6. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- DeepSeek static flags still format as
  `chat,json_object,logprobs,reasoning,stream,tools`.
- Generic Codex fallback flags still format as `chat,reasoning,stream`.
- Codex model capabilities still emit the same sorted comma-separated string,
  including optional `parallel_tool_calls`, `reasoning`, `service_tier`, and
  `vision`.
- OpenRouter supported parameters still emit the same sorted comma-separated
  capability string, including `model_fallbacks`.
- OpenAI Codex `/models` enrichment still detects `responses`, `service_tier`,
  `vision`, `reasoning`, and `parallel_tool_calls` from stored capability
  strings.
- Parsing trims comma-separated values and ignores blank parts while preserving
  unknown values.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/metadata ./internal/provider ./internal/openai
go test ./...
go vet ./...
```

Finally build a temporary `ilonasin` binary and smoke `ilonasin serve` plus
bounded `ilonasin manage` runs at narrow and wide terminal widths against an
isolated temporary `ILONASIN_HOME`, then remove all temporary files.

## Non-Goals

- No new capability semantics.
- No filtering of unknown persisted capability values.
- No schema, DTO, TUI, logging, routing, credential, or provider-option
  behavior changes.
- No permanent test files.

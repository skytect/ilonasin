# 398 Responses Conversion Policy

## Context

Three fresh senior codebase reviews independently flagged the same architecture
issue: `internal/openai` is a compatibility parser/converter, but its Responses
conversion currently branches on raw provider type strings:

- Codex raw Responses input preservation,
- Codex raw Responses tool preservation,
- Codex-only `reasoning`, `text.verbosity`, and `service_tier`,
- OpenRouter `parallel_tool_calls` forwarding.

The behavior is intentional, but the current `providerType string` argument
keeps provider-specific policy hidden inside the neutral OpenAI package. The
architecture says provider-specific behavior belongs behind provider/server
selection boundaries, while compatibility converters should be strict and
explicit.

## Goal

Replace raw provider-type branching in the OpenAI Responses converter with an
explicit conversion policy selected by the server/provider boundary.

## Scope

1. Add a small exported `openai.ResponsesConversionPolicy` value type that
   declares the existing conversion choices:
   - preserve Codex-native Responses input;
   - preserve Codex-native Responses tools;
   - allow Responses reasoning/text/service tier options;
   - allow Responses `parallel_tool_calls`.
2. Change `ResponsesRequest.ToChatCompletionRequest` to accept that policy
   instead of a provider type string.
3. Change `responsesToolsToChatTools` to accept the tool-preservation flag
   instead of a provider type string.
4. Add a focused server helper that maps current provider instances to the
   existing policy:
   - Codex preserves raw Responses input and tools, and accepts Codex
     Responses options;
   - OpenRouter accepts `parallel_tool_calls`;
   - other providers use the strict Chat-adapter conversion path.
5. Preserve all current runtime behavior and error messages.

## Out Of Scope

- Moving tool-family validation into provider adapters.
- Changing Anthropic conversion.
- Changing provider request bodies, streaming behavior, routing, credential
  pooling, quota behavior, management API, TUI, config, storage, logging, or
  model discovery.
- Broadly removing all provider-specific metadata extraction from server code.

## Verification

Add a temporary focused OpenAI package check, then remove it before commit. It
must prove:

- Codex policy preserves raw Responses input and raw Codex Responses tools;
- Codex policy accepts Responses `reasoning`, `text.verbosity`, and
  `service_tier` with the same provider-options shape;
- OpenRouter policy forwards `parallel_tool_calls` without preserving Codex raw
  input/tools;
- strict providers keep the Chat-adapter conversion path;
- strict providers still reject Responses `reasoning`, `text`, and
  `service_tier` with the unchanged error message;
- disallowed provider/tool policy combinations keep the existing error strings.

Run:

```sh
rg -n "ToChatCompletionRequest\\(|responsesToolsToChatTools\\(|providerType ==|providerType !=" internal/openai internal/server
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/openai ./internal/server
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke at narrow and wide
terminal widths.

## Acceptance

- `internal/openai` Responses conversion no longer compares provider type
  strings directly.
- The current provider-specific Responses conversion choices are selected at
  the server/provider boundary.
- Existing local request validation and provider behavior are unchanged.
- The slice is a small boundary step toward adapter-owned provider policy.

# 411 Shared Option Metadata Safety

## Context

Whole-codebase review flagged duplicate metadata allowlists after slices 407 and
408:

- `internal/provider/chat_option_metadata.go` owns Chat provider-option
  metadata extraction.
- `internal/openai/responses_option_metadata.go` owns Responses option metadata
  extraction.

Both files now duplicate the same metadata-safe service tier, reasoning effort,
and reasoning summary allowlists. That creates drift risk while preserving the
right ownership boundaries.

## Goal

Centralize shared OpenAI-compatible option metadata safety helpers while keeping
Chat and Responses metadata DTO ownership unchanged.

## Scope

1. Add shared exported helpers in `internal/openai`.
   - `SafeOptionServiceTier`.
   - `SafeOptionReasoningEffort`.
   - `SafeOptionReasoningSummary`.
   - Preserve the current allowlists exactly.
2. Update `internal/openai/responses_option_metadata.go`.
   - Use the shared helpers.
   - Remove duplicate Responses-specific helper functions.
3. Update `internal/provider/chat_option_metadata.go`.
   - Use the shared OpenAI helpers for service tier, reasoning effort, and
     reasoning summary.
   - Keep provider-owned Chat metadata extraction and DeepSeek thinking-type
     safety local to provider.
4. Keep behavior unchanged.
   - No changes to provider option validation. Validation allowlists can remain
     narrower than metadata allowlists where they currently are.
   - No changes to request conversion, routing, metadata fields, config,
     storage, management, TUI, or provider adapters.
   - No permanent tests.

## Verification

- Temporary focused checks must prove current Chat and Responses metadata
  behavior is preserved; remove them before commit.
- Include checks for:
  - shared helper valid and invalid service tiers;
  - shared helper valid and invalid reasoning efforts;
  - shared helper valid and invalid reasoning summaries;
  - Chat Codex `fast` service-tier mapping still maps effective tier to
    `priority`;
  - Chat Codex reasoning effort and summary still populate metadata;
  - Chat DeepSeek `reasoning_effort` still populates metadata;
  - Chat OpenRouter reasoning effort still populates metadata;
  - Responses valid and invalid option metadata behavior.
- Run `git diff --check`.
- Run `find . -name '*_test.go' -type f -print`.
- Run `go test ./internal/openai ./internal/provider`.
- Run `go test ./...`.
- Run `go vet ./...`.
- Build `ilonasin`.
- Smoke `ilonasin serve` with an isolated `ILONASIN_HOME`.
- Smoke `ilonasin manage` against that daemon at narrow and wide terminal
  widths.

## Non-Goals

- Changing provider validation accepted values.
- Changing metadata DTO field names or ownership.
- Moving Chat extraction out of provider or Responses extraction out of OpenAI.
- Adding new metadata fields.

# 408 Responses Option Metadata OpenAI Boundary

## Context

Slice 407 moved Chat provider-option metadata extraction out of
`internal/server` and into `internal/provider`, leaving
`internal/server/request_metadata_options.go` with only local Responses option
metadata extraction.

`docs/ilonasin-architecture.md` keeps HTTP transport, routing, provider
adapters, storage, and local compatibility parsing as separate boundaries. The
server should record metadata, but it should not own local OpenAI-compatible
Responses option vocabulary or sanitization allowlists.

## Goal

Move Responses option metadata extraction into `internal/openai`, preserving
current request metadata behavior.

## Scope

1. Add an OpenAI-owned DTO for safe Responses option metadata.
   - Keep it independent from `internal/metadata`.
   - Include only existing fields used by Responses metadata today:
     requested service tier, reasoning effort, and reasoning summary.
2. Add an OpenAI-owned extraction function for `openai.ResponsesRequest`.
   - Preserve current `service_tier` allowlist:
     `auto`, `default`, `flex`, `priority`, `scale`, `fast`.
   - Preserve current reasoning effort allowlist:
     `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`.
   - Preserve current reasoning summary allowlist:
     `auto`, `concise`, `detailed`, `none`.
   - Preserve current behavior for absent, non-string, and invalid fields:
     they are not recorded.
3. Update `internal/server/request_metadata_options.go`.
   - Server should call the OpenAI-owned extractor and copy DTO fields into
     `metadata.Request`.
   - Server should no longer contain Responses option vocabulary or duplicated
     option sanitizers after this slice.
4. Keep behavior unchanged.
   - No provider request conversion, validation, routing, config, database,
     logging, management, TUI, or credential-pooling changes.
   - No permanent tests.

## Verification

- Temporary focused checks must be used to compare old and new Responses
  metadata behavior; remove them before commit.
- Include focused behavior checks for:
  - valid Responses `service_tier`;
  - invalid Responses `service_tier`;
  - valid reasoning effort and summary;
  - invalid or non-string reasoning effort and summary;
  - absent reasoning and service tier fields.
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

- Moving Chat option metadata again.
- Changing Responses validation or Chat conversion behavior.
- Changing which metadata fields are stored or rendered.
- Adding new management or TUI surfaces.
- Moving endpoint constants or request metadata base construction.

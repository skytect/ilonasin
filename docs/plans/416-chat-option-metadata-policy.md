# 416 Chat Option Metadata Policy

## Context

Whole-codebase review found that Chat option metadata still has a raw
provider-type boundary:

- `internal/server/request_metadata_options.go` passes a `providerType string` to
  `provider.ExtractChatOptionMetadata`.
- `internal/provider/chat_option_metadata.go` switches on raw provider names.
- Early Chat error metadata passes `""` to preserve unknown-provider behavior.

The provider package should still own Chat provider-option vocabulary and
metadata extraction, but the server should not pass magic raw strings or rely on
`""` as an implicit policy.

## Goal

Replace the raw Chat option metadata provider-type string with an explicit
provider-owned metadata policy, preserving all existing metadata behavior.

## Scope

1. Add a provider-owned policy type in `internal/provider/chat_option_metadata.go`,
   for example `ChatOptionMetadataPolicy`.
   - It should encode whether Codex, DeepSeek, or OpenRouter provider-option
     metadata should be extracted.
   - It should encode the Codex top-level `service_tier=default` effective-tier
     behavior.
   - The zero value must preserve current early/unknown-provider behavior:
     top-level service tier metadata is recorded normally, provider-specific
     option metadata is not extracted, and Codex default-tier suppression is not
     applied.
2. Add a factory such as `ChatOptionMetadataPolicyForProviderType(providerType string)`.
   - `codex` enables Codex metadata extraction and Codex default-tier behavior.
   - `deepseek` enables DeepSeek metadata extraction.
   - `openrouter` enables OpenRouter metadata extraction.
   - unknown or empty provider types return the zero policy.
3. Change `ExtractChatOptionMetadata` to accept the policy type instead of a raw
   provider-type string.
4. Update server metadata code to pass explicit policies.
   - Normal Chat metadata uses `ChatOptionMetadataPolicyForProviderType(instance.Type)`.
   - Early Chat metadata uses the zero policy explicitly.
5. Keep all provider option validation, request marshaling, routing, storage,
   management, TUI, config, IO logging, and public response behavior unchanged.
6. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- Zero policy preserves current early/unknown behavior: top-level service tier is
  recorded and effective tier is set, provider-specific option metadata is not
  extracted.
- Codex policy preserves top-level `service_tier=default` effective-tier
  suppression.
- DeepSeek, OpenRouter, and unknown policies preserve top-level
  `service_tier=default` with effective tier also set to `default`; Codex is
  the only policy that suppresses effective `default`.
- Codex policy preserves `provider_options.codex.service_tier=fast` mapping to
  effective `priority`.
- Codex policy preserves reasoning effort and summary metadata.
- DeepSeek policy preserves `reasoning_effort` and thinking type metadata.
- OpenRouter policy preserves reasoning effort, max tokens, enabled, and exclude
  metadata.
- Server early metadata passes zero policy behavior explicitly.
- Server normal metadata passes provider-derived policy behavior.

Then run:

```sh
rg -n "ExtractChatOptionMetadata\(|ChatOptionMetadataPolicy|providerType string" internal/provider/chat_option_metadata.go internal/server/request_metadata_options.go internal/server/request_metadata_early.go internal/server/request_metadata_chat.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/provider ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- `ExtractChatOptionMetadata` no longer accepts a raw provider-type string.
- Early/unknown metadata behavior is explicit through a zero policy.
- Provider-specific Chat option metadata behavior is unchanged.
- No permanent tests are added.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

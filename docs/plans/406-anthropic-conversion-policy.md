# 406 Anthropic Conversion Policy

## Context

Fresh senior whole-codebase reviews found the same architecture issue:
Anthropic request conversion still receives raw provider type strings.

Current flow:

- `internal/server/anthropic_route.go` passes `instance.Type` directly into
  `req.ToChatCompletion`.
- `internal/anthropic/chat_conversion.go` accepts `providerType string`.
- The converter special-cases Codex by omitting Anthropic generation fields
  that Codex chat routes should not receive.

This mirrors the old Responses conversion boundary that was already improved
with an explicit `ResponsesConversionPolicy`. The architecture says local
compatibility routes should convert into the strict local request model, while
provider-specific behavior should be explicit and auditable at the routing
boundary.

## Goal

Replace raw provider-type Anthropic conversion with an explicit conversion
policy while preserving conversion behavior exactly.

## Scope

1. Add an Anthropic conversion policy type in `internal/anthropic`, for example:
   `ChatConversionPolicy`.
2. Change `Request.ToChatCompletion(providerType string)` to
   `Request.ToChatCompletion(policy ChatConversionPolicy)`.
3. Preserve exact behavior:
   - Codex conversion still omits Anthropic `max_tokens`, `temperature`,
     `top_p`, `top_k`, and `stop` fields from the converted Chat request.
   - Non-Codex conversion still includes those fields when present.
   - Converted messages, tools, tool choice, affinity, and present-field
     tracking are unchanged.
4. Add a server-local helper that maps `provider.Instance` to the Anthropic
   conversion policy.
5. Use the helper in `handleAnthropicMessages`.
6. Do not change Anthropic request parsing, validation, route shape, provider
   adapters, storage, schema, management routes, TUI, config, IO logging,
   routing, public API behavior, or metadata field names.

## Verification

Use temporary focused checks, then remove them before commit:

- Codex policy still omits `max_tokens`, `temperature`, `top_p`, `top_k`, and
  `stop` present fields.
- Non-Codex policy still preserves those fields when present.
- Messages, tools, tool choice, and affinity conversion remain unchanged.
- The server policy helper maps Codex providers to the Codex behavior and
  non-Codex providers to the default behavior.

Then run:

```sh
rg -n 'ToChatCompletion\\(|providerType string|instance\\.Type' internal/anthropic internal/server/anthropic_route.go internal/server
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/anthropic ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Anthropic conversion no longer receives raw provider type strings.
- Provider-specific Anthropic conversion behavior is selected explicitly in the
  server boundary.
- Runtime Anthropic compatibility behavior is unchanged.
- No permanent tests are added.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

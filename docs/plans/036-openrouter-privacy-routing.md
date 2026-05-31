# 036 OpenRouter Privacy Routing

## Goal

Add the narrow OpenRouter privacy-routing controls already called out in the
architecture notes, without opening the broader provider routing surface.

## Sources

- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- OpenRouter OpenAPI `ProviderPreferences`
- OpenRouter routing and ZDR documentation via Context7

## Scope

1. Extend `provider_options.openrouter.provider` validation to accept:
   - `require_parameters`: boolean
   - `data_collection`: exactly `"deny"`
   - `zdr`: exactly `true`
2. Keep `provider_options.openrouter.provider` as an object with at least one
   supported field.
3. Continue rejecting unsupported provider routing fields such as `order`,
   `only`, `ignore`, `allow_fallbacks`, `sort`, `max_price`, and
   `quantizations`.
4. Continue rejecting top-level `provider` input from clients.
5. Continue rejecting OpenRouter provider options for DeepSeek and Codex before
   credential resolution.
6. Forward the accepted provider privacy fields to OpenRouter as top-level
   `provider` fields after validation.
7. Update direct smoke checks to cover:
   - `require_parameters` plus `data_collection: "deny"` plus `zdr: true`
   - `data_collection: "deny"` without `require_parameters`
   - `zdr: true` without `require_parameters`
   - streaming forwarding for the accepted privacy fields
   - invalid `data_collection`
   - invalid `zdr`
   - unsupported provider routing fields with marker-bearing values
   - leak checks that rejected routing markers do not appear in local errors,
     SQLite metadata, CLI/TUI output, fake-upstream output, or successful
     responses

## Non-Goals

- Do not implement provider allow lists, block lists, ordering, sorting,
  fallback toggles, price caps, quantization filters, model fallbacks, BYOK, or
  region routing.
- Do not add permanent test files.
- Do not persist privacy-routing request content or raw provider payloads.

## Implementation Plan

1. Update provider validation in `internal/provider/http_chat.go` so the
   OpenRouter provider object is checked field by field instead of requiring
   exactly `require_parameters`.
2. Keep the existing marshal path that translates
   `provider_options.openrouter.provider` to OpenRouter's top-level
   `provider` object.
3. Update `internal/app/app.go` serve-check helpers so combined OpenRouter
   smoke requests include the accepted privacy fields and assert they reach the
   fake upstream.
4. Add no-eligible-cache smoke cases for invalid privacy-routing values and
   unsupported provider-routing fields.
5. Run:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
   - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Review Questions

1. Is accepting only `data_collection: "deny"` and `zdr: true` the right
   privacy-preserving subset, even though OpenRouter also documents
   `data_collection: "allow"` and `zdr: false`?
2. Does keeping broader provider routing unsupported preserve the current
   architecture boundary?
3. Are the proposed smoke checks enough without adding permanent tests?

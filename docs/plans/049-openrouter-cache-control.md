# 049 OpenRouter Cache Control

## Context

The architecture requires a strict OpenAI-compatible subset and says
provider-specific escape hatches must be explicit and namespaced. OpenRouter's
live OpenAPI document accessed on 2026-05-31 exposes top-level
`cache_control` on `ChatRequest` as an `AnthropicCacheControlDirective`.

The directive is a small object:

- required `type`, currently `ephemeral`,
- optional `ttl`, currently `5m` or `1h`.

The local API should keep client top-level `cache_control` rejected, while
allowing an explicit OpenRouter-only namespaced option that forwards to
OpenRouter upstream.

## Scope

1. Add OpenRouter-only support for `provider_options.openrouter.cache_control`.
2. Translate that field to upstream top-level `cache_control` for OpenRouter
   chat completions.
3. Keep client top-level `cache_control` rejected for all providers.
4. Keep the field unsupported for DeepSeek and Codex by namespace validation
   before upstream credential resolution.
5. Accept only an object with:
   - `type: "ephemeral"`,
   - optional `ttl` of `"5m"` or `"1h"`.
6. Reject `null`, non-objects, empty objects, missing `type`, unsupported
   `type`, non-string `type`, unsupported `ttl`, non-string `ttl`, and unknown
   object fields.
7. Do not change cache telemetry parsing, local persistence, routing,
   credential fallback, model addressing, or TUI behavior.

## Implementation

1. Update `internal/provider/http_chat.go`.
   - Add `cache_control` to `validateOpenRouterOptions`.
   - Add `validateOpenRouterCacheControl`.
   - Translate accepted `provider_options.openrouter.cache_control` to upstream
     `cache_control`.
   - Preserve existing `reasoning`, `models`, and `provider` translation.

2. Update `internal/app/app.go` smoke harness.
   - Add exact upstream validators for:
     - `{"type":"ephemeral"}`,
     - `{"type":"ephemeral","ttl":"5m"}`,
     - `{"type":"ephemeral","ttl":"1h"}`,
     - a combined request with reasoning, models, and provider options.
   - Add non-stream and stream smoke requests.
   - Add OpenRouter invalid cases for all rejected shapes.
   - Add unsupported-provider invalid cases for DeepSeek and Codex.
   - Add Codex no-eligible-cache checks for unsupported and invalid
     OpenRouter cache control options.
   - Add non-stream and stream top-level `cache_control` rejection coverage
     with no upstream dispatch.
   - Verify invalid marker strings are not echoed or persisted outside secret
     tables.

## Smoke Checks

Run these direct checks before code review:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
rm -rf "$tmp" "$tmpbin"
```

`go test ./...` is only a compile/package check. No permanent test files will
be added.

## Review Questions

1. Is `provider_options.openrouter.cache_control` the right local escape hatch
   while preserving rejection of client top-level `cache_control`?
2. Is limiting `ttl` to current documented values better than accepting unknown
   future strings in this strict local subset?
3. Does this slice preserve the architecture's no raw request or provider
   payload storage boundary?

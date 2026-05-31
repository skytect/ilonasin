# 048 OpenRouter Model Fallbacks

## Context

The architecture requires explicit routing by default and says provider-specific
escape hatches must be explicit and namespaced. OpenRouter's live OpenAPI
document accessed on 2026-05-31 exposes top-level `models` on `ChatRequest` as
an array of model names for completion fallback. The OpenRouter docs also state
that model fallback can cause the resolved upstream model to differ from the
requested model, so the router should keep requested and resolved metadata
separate.

This codebase currently rejects top-level `models`, which is correct for the
local OpenAI-compatible surface. The missing architecture-aligned path is an
explicit OpenRouter namespace that can forward the fallback list only to
OpenRouter.

## Scope

1. Add OpenRouter-only support for `provider_options.openrouter.models`.
2. Translate that field to upstream top-level `models` for OpenRouter requests.
3. Keep client top-level `models` rejected for all providers.
4. Keep the field unsupported for DeepSeek and Codex by namespace validation
   before upstream credential resolution.
5. Accept only a non-empty JSON array of unique model slug strings.
6. Reject `null`, non-arrays, empty arrays, non-string items, empty strings,
   duplicate strings, overlong strings, overlong lists, and model strings with
   characters outside a conservative OpenRouter model slug subset.
7. Do not change the local requested model, local provider instance selection,
   credential fallback policy, routing metadata schema, persistence, or TUI.

## Validation Shape

Use a strict local subset:

- Array length: 1 to 32.
- String length: 1 to 256.
- Allowed characters: ASCII letters, ASCII digits, `_`, `-`, `.`, `/`, `:`,
  and `~`.

This allows common OpenRouter IDs such as `openai/gpt-4o` and
`deepseek/deepseek-chat:free`, plus official model fallback examples such as
`~anthropic/claude-sonnet-latest`, while rejecting whitespace and arbitrary
private text.

## Implementation

1. Update `internal/provider/http_chat.go`.
   - Add `models` to `validateOpenRouterOptions`.
   - Add `validateOpenRouterModelList` and `isOpenRouterModelSlug`.
   - Translate accepted `provider_options.openrouter.models` to upstream
     `models`.
   - Continue translating existing `reasoning` and `provider` fields unchanged.

2. Update `internal/app/app.go` smoke harness.
   - Add exact upstream validators for:
     - a normal fallback model list,
     - a tilde-prefixed model fallback list,
     - a marker-bearing fallback model list,
     - a combined request with reasoning and provider options.
   - Add non-stream and stream smoke requests.
   - Add non-stream and stream rejection smokes proving client top-level
     `models` remains unsupported and does not reach upstream.
   - Add non-stream and stream accepted smokes where the fake upstream returns
     a different `model`, then assert SQLite records the local requested model
     as the client primary model and records the upstream response model as the
     resolved model without creating local credential fallback events.
   - Add invalid OpenRouter cases for null, wrong type, empty list,
     non-string item, empty item, duplicate item, too many items, too-long item,
     and bad characters.
   - Add unsupported-provider invalid cases for DeepSeek and Codex.
   - Add Codex no-eligible-cache checks for unsupported and invalid model
     fallback options.
   - Verify marker-bearing fallback model IDs are not echoed or stored outside
     secret tables.

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

1. Is `provider_options.openrouter.models` the right local escape hatch instead
   of accepting client top-level `models`?
2. Is the strict model slug subset conservative enough without blocking common
   OpenRouter model IDs?
3. Does forwarding `models` preserve local requested/resolved model metadata
   boundaries and avoid hidden local fallback behavior?

# 055 OpenRouter Response Format Split

## Context

Plan 051 moved OpenRouter option validation into
`internal/provider/openrouter_options.go`, but intentionally left OpenRouter
JSON schema `response_format` validation in `internal/provider/http_chat.go`
because it was coupled to the shared `response_format` path at that time.

After plans 052 through 054, the provider-specific clusters have been moved out
incrementally. The shared HTTP chat adapter still contains two OpenRouter-only
helpers:

- `validateOpenRouterJSONSchemaResponseFormat`
- `isOpenRouterJSONSchemaName`

The architecture says provider adapters own provider-specific behavior and that
provider-specific escape hatches must be explicit and namespaced. This is a
behavior-preserving split that keeps generic response-format dispatch in the
shared adapter while moving OpenRouter-specific schema validation beside the
rest of OpenRouter option validation.

## Scope

1. Move OpenRouter JSON schema response-format helpers from
   `internal/provider/http_chat.go` into
   `internal/provider/openrouter_options.go`.
2. Keep function names, accepted values, validation rules, and exact error
   strings unchanged.
3. Keep `validateChatResponseFormat` in `http_chat.go`, including its provider
   dispatch and call to `validateOpenRouterJSONSchemaResponseFormat`.
4. Keep OpenRouter option validation, DeepSeek behavior, Codex behavior,
   request marshaling, streaming transport, model discovery, storage, routing,
   TUI, and smoke harness behavior unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Move this OpenRouter-specific cluster intact into
   `internal/provider/openrouter_options.go`:
   - `validateOpenRouterJSONSchemaResponseFormat`
   - `isOpenRouterJSONSchemaName`
2. Leave shared `response_format` validation and provider selection in
   `http_chat.go`.
3. Run `gofmt` on touched Go files.
4. Manually review the diff before smoke checks. The Go diff should be a move
   plus import cleanup only.

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

1. Is `openrouter_options.go` the right home for OpenRouter JSON schema
   response-format validation?
2. Should `validateChatResponseFormat` remain in `http_chat.go` as the shared
   response-format dispatcher?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

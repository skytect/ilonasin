# 058 Model Discovery Split

## Context

Plans 051 through 057 progressively moved provider-specific validation,
metadata parsing, Codex `/responses` handling, shared chat validation, and chat
request marshaling out of `internal/provider/http_chat.go`.

The shared HTTP chat adapter file still contains the model discovery path:

- `ListModels`
- `MaxUpstreamModelsBodyBytes`
- `modelsURL`
- `normalizeModels`
- `validProviderModelID`
- `safeDisplayName`
- `safeInt`
- `modelTimeout`

The architecture says provider adapters own provider model discovery and expose
sanitized model metadata. Keeping model discovery in the same file as chat
completion transport makes the adapter boundary harder to read and maintain.

This slice is a behavior-preserving split. It does not change model endpoint
selection, sanitized metadata extraction, capability flags, storage, routing,
or error strings.

## Scope

1. Move model discovery from `internal/provider/http_chat.go` into a new
   same-package file, `internal/provider/http_models.go`.
2. Keep function and method names, receiver, accepted inputs, timeout behavior,
   model URL behavior, timeout behavior, JSON parsing behavior, duplicate
   rejection, sanitization, sorting, capability flags, error classes, and exact
   error strings unchanged.
3. Keep `joinBasePath`, shared upstream body limiting, transport error
   classification, and retry-after parsing in `http_chat.go`, because they
   remain shared helpers used by chat, streaming, OAuth refresh, or model
   discovery call paths.
4. Keep OpenRouter metadata helpers, chat validation, chat request marshaling,
   Codex `/responses`, storage, routing, TUI, and smoke harness behavior
   unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/provider/http_models.go`.
2. Move the model discovery cluster intact:
   - `ListModels`
   - `MaxUpstreamModelsBodyBytes`
   - `modelsURL`
   - `normalizeModels`
   - `validProviderModelID`
   - `safeDisplayName`
   - `safeInt`
   - `modelTimeout`
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

1. Is `http_models.go` the right boundary for provider model discovery and
   sanitized model metadata normalization?
2. Should `joinBasePath` and shared transport helpers remain in
   `http_chat.go` for this slice?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

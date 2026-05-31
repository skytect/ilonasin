# 054 DeepSeek Option Validation Split

## Context

Plans 051 through 053 moved OpenRouter option validation, Codex `/responses`
handling, and OpenRouter metadata helpers out of `internal/provider/http_chat.go`.
The shared HTTP chat adapter still contains the DeepSeek-specific
`provider_options.deepseek` validation helpers:

- `validateDeepSeekOptions`
- `isDeepSeekUserID`

The architecture says provider adapters own provider-specific behavior and that
provider-specific escape hatches must be explicit and namespaced. This is a
behavior-preserving split that keeps the shared adapter focused on dispatch,
transport, and common OpenAI-compatible request handling.

## Scope

1. Move DeepSeek-specific option validation helpers from
   `internal/provider/http_chat.go` into a new same-package file,
   `internal/provider/deepseek_options.go`.
2. Keep function names, accepted values, validation rules, and exact error
   strings unchanged.
3. Keep `validateProviderOptions`, `ValidateChatRequest`, and
   `marshalChatCompletionsRequest` call sites unchanged.
4. Keep OpenRouter behavior, Codex behavior, response-format validation,
   streaming transport, model discovery, storage, routing, TUI, and smoke
   harness behavior unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/provider/deepseek_options.go`.
2. Move this DeepSeek-specific cluster intact:
   - `validateDeepSeekOptions`
   - `isDeepSeekUserID`
3. Leave the shared provider-options dispatcher in `http_chat.go`, since it
   owns the common wrapper checks and selects the provider-specific validator.
4. Run `gofmt` on touched Go files.
5. Manually review the diff before smoke checks. The Go diff should be a move
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

1. Is moving only DeepSeek option validation the right-sized next provider
   boundary cleanup?
2. Should the shared `validateProviderOptions` wrapper remain in
   `http_chat.go` for now?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

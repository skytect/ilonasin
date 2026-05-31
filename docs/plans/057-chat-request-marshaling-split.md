# 057 Chat Request Marshaling Split

## Context

Plan 056 moved adapter-owned chat validation into
`internal/provider/chat_validation.go`. The shared HTTP adapter file now still
contains upstream chat request construction:

- `marshalChatCompletionsRequest`

That function builds the OpenAI-compatible upstream request body, applies
`max_completion_tokens` translation for DeepSeek and OpenRouter, and translates
validated `provider_options` into provider-native fields. The architecture says
provider adapters own provider-specific request translation, while the transport
layer should stay focused on HTTP execution and stream handling.

This slice is a behavior-preserving split. It does not change request shapes,
validation, forwarding, storage, or error strings.

## Scope

1. Move `marshalChatCompletionsRequest` from
   `internal/provider/http_chat.go` into a new same-package file,
   `internal/provider/chat_request.go`.
2. Keep function name, accepted inputs, validation calls, translation behavior,
   JSON decoder settings, and exact error strings unchanged.
3. Keep `CompleteChat` and `StreamChat` call sites unchanged.
4. Keep chat validation, provider-specific option validators, OpenRouter
   metadata parsing, Codex `/responses`, model discovery, stream transport,
   storage, routing, TUI, and smoke harness behavior unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/provider/chat_request.go`.
2. Move `marshalChatCompletionsRequest` intact.
3. Leave non-streaming and streaming HTTP execution in `http_chat.go`.
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

1. Is `chat_request.go` the right boundary for upstream chat-completions request
   construction and provider-option translation?
2. Should `CompleteChat` and `StreamChat` remain in `http_chat.go` for this
   slice to keep HTTP transport separate from request shaping?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

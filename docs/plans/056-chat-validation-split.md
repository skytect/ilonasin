# 056 Chat Validation Split

## Context

Plans 051 through 055 moved provider-specific validation and metadata helpers
out of `internal/provider/http_chat.go`. The shared HTTP adapter still contains
the adapter-owned chat validation cluster:

- `ValidateChatRequest`
- `hasToolMessages`
- `rejectPresentFields`
- `validateChatResponseFormat`
- `validateProviderOptions`

Plan 018 established adapter-owned feature validation as a boundary after model
routing and before credential resolution. The architecture also describes a
strict request validator before routing reaches provider transport. Keeping this
cluster in the transport-heavy HTTP adapter file makes that boundary harder to
review.

This slice is a behavior-preserving split. It does not change what is accepted,
rejected, forwarded, or stored.

## Scope

1. Move the chat validation cluster from `internal/provider/http_chat.go` into
   a new same-package file, `internal/provider/chat_validation.go`.
2. Keep function names, receiver, accepted fields, rejection order, validation
   rules, and exact error strings unchanged.
3. Keep `marshalChatCompletionsRequest`, `CompleteChat`, `StreamChat`,
   provider-specific option validators, and provider-specific request
   translation unchanged.
4. Keep OpenRouter behavior, DeepSeek behavior, Codex behavior, model
   discovery, streaming transport, storage, routing, TUI, and smoke harness
   behavior unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/provider/chat_validation.go`.
2. Move this cluster intact:
   - `ValidateChatRequest`
   - `hasToolMessages`
   - `rejectPresentFields`
   - `validateChatResponseFormat`
   - `validateProviderOptions`
3. Leave request marshaling and upstream HTTP execution in `http_chat.go`.
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

1. Is `chat_validation.go` the right boundary for adapter-owned feature
   validation while keeping transport in `http_chat.go`?
2. Should `marshalChatCompletionsRequest` remain in `http_chat.go` for this
   slice to avoid mixing validation and request construction changes?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

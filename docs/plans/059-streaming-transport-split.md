# 059 Streaming Transport Split

## Context

Plans 051 through 058 moved provider-specific validation, metadata parsing,
Codex `/responses` handling, shared chat validation, chat request marshaling,
and model discovery out of `internal/provider/http_chat.go`.

The shared HTTP chat adapter file still contains the generic streaming
transport helpers and the generic chat-completions streaming path:

- `DefaultMaxStreamLineBytes`
- `DefaultMaxStreamEventBytes`
- `DefaultMaxStreamEvents`
- `DefaultStreamIdleTimeout`
- `DefaultStreamHeaderTimeout`
- `StreamChat`
- `streamingClient`
- `streamIdleTimeout`
- `streamHeaderTimeout`
- `maxStreamLineBytes`
- `maxStreamEventBytes`
- `maxStreamEvents`
- `doStreamRequest`
- `readStream`
- `readStreamLine`
- `handleStreamEvent`
- `normalizedStreamErrorData`
- `classifyStreamReadError`
- `streamStatusForError`

The architecture says provider adapters should stream provider responses into
normalized OpenAI-style chunks, while non-streaming HTTP execution should stay
separate from streaming event parsing and limits. Keeping all generic streaming
logic in `http_chat.go` makes the file a mixed transport surface again after
the previous splits.

Some of these stream helpers are already shared by Codex `/responses` streaming
in `internal/provider/codex_responses.go`. Moving them into `http_stream.go`
keeps those Codex call sites unchanged while making the shared streaming helper
home explicit.

This slice is a behavior-preserving split. It does not change streaming request
construction, timeouts, limits, event parsing, normalized chunks, summary
metadata, OpenRouter cost extraction, error classes, retry-after handling,
storage, routing, or TUI behavior.

## Scope

1. Move the generic streaming chat-completions cluster from
   `internal/provider/http_chat.go` into a new same-package file,
   `internal/provider/http_stream.go`.
2. Keep function and method names, receiver, accepted inputs, timeout behavior,
   stream limit behavior, event parsing, normalized error body, status mapping,
   OpenRouter usage cost extraction, and exact error strings unchanged.
3. Keep non-streaming `CompleteChat`, shared URL construction, shared upstream
   body limiting, JSON parsing, transport error classification, and retry-after
   parsing in `http_chat.go`, because those helpers remain shared by multiple
   provider call paths.
4. Keep Codex streaming request construction, event handling, and chunk
   translation in `internal/provider/codex_responses.go`. Codex call sites may
   continue to use the shared stream helper methods after those helpers move to
   `http_stream.go`.
5. Keep chat validation, chat request marshaling, model discovery, storage,
   routing, TUI, and smoke harness behavior unchanged.
6. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/provider/http_stream.go`.
2. Move the generic streaming cluster intact:
   - the five default stream constants,
   - `StreamChat`,
   - `streamingClient`,
   - stream timeout and limit accessors,
   - `doStreamRequest`,
   - `readStream`,
   - `readStreamLine`,
   - `handleStreamEvent`,
   - `normalizedStreamErrorData`,
   - `classifyStreamReadError`,
   - `streamStatusForError`.
3. Leave `chatCompletionsURL`, `marshalChatCompletionsRequest`,
   `classifyTransportError`, and `retryAfterFromHeader` in their current files
   and call them from the new same-package file.
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

1. Is `http_stream.go` the right boundary for generic SSE streaming transport
   and normalized OpenAI-style stream chunks?
2. Should `StreamChat` move with the stream parser and timeout helpers, while
   `CompleteChat` remains in `http_chat.go`?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

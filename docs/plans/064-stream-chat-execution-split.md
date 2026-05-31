# 064 Stream Chat Execution Split

## Context

Plans 060 through 063 split `/v1/models`, shared server credential helpers,
the `/v1/chat/completions` route entrypoint, and non-streaming chat execution
out of `internal/server/server.go`. The main server file still contains server
bootstrap, local auth middleware, streaming chat execution, stream response
writing, health conversion, metadata recording, and JSON response helpers.

The architecture treats streaming chat completions as part of the
OpenAI-compatible API surface, but streaming has a distinct server execution
path: it requires an `http.Flusher`, writes server-sent events, may retry only
before the stream has started, records stream metrics, and records same-provider
credential fallback metadata.

This slice is behavior-preserving. It does not change request routing,
credential resolution, OAuth refresh behavior, fallback policy, stream chunk
format, metadata, health recording, storage, response bodies, or error strings.

## Scope

1. Move streaming chat execution from `internal/server/server.go` into a new
   same-package file, `internal/server/chat_stream.go`.
2. Move this cluster intact:
   - `streamContext`
   - `streamAttempt`
   - `singleStreamContext`
   - `singleStreamAttempt`
   - `handleSingleCredentialStreamingChat`
   - `handleStreamingChat`
   - `streamSink`
   - `streamSink.WriteEvent`
   - `streamSink.WriteDone`
   - `streamSink.start`
3. Move stream-specific helpers with that execution path:
   - `retryableStreamAttempt`
   - `healthFromStreamAttempt`
   - `healthFromSingleStreamAttempt`
   - `shouldRecordStreamHealth`
4. Leave shared helpers in their current files for this slice:
   - `resolvedChatModel`, shared by streaming and non-streaming metadata
   - `retryableHTTPStatus`, shared by streaming and non-streaming retry logic
   - `fallbackReason`, shared by streaming and non-streaming fallback metadata
   - metadata record helpers and JSON response helpers
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/chat_stream.go`.
2. Move the streaming execution cluster intact.
3. Keep helper names, dispatch calls, metadata records, fallback records, SSE
   headers, and error strings unchanged.
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

1. Is `internal/server/chat_stream.go` the right boundary for streaming chat
   execution, SSE response writing, and pre-stream fallback behavior?
2. Should shared helpers like `retryableHTTPStatus`, `resolvedChatModel`, and
   `fallbackReason` remain outside the stream-specific file for this slice?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

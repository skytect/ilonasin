# 063 Nonstream Chat Execution Split

## Context

Plans 060 through 062 split `/v1/models`, shared server credential helpers, and
the `/v1/chat/completions` route entrypoint out of `internal/server/server.go`.
The main server file still contains server bootstrap, local auth middleware,
non-streaming chat execution, streaming chat execution, health conversion,
metadata recording, stream response writing, and JSON response helpers.

The architecture describes the request path as distinct stages:

```text
OpenAI-compatible request parser
  -> strict request validator
    -> model address resolver
      -> routing policy
        -> credential resolver
          -> provider adapter
```

After plan 062, `chat_route.go` owns parsing, validation, address resolution,
provider lookup, adapter lookup, credential selection, and dispatch to either
streaming or non-streaming execution. Non-streaming execution is now a separate
server responsibility: it calls provider adapters, performs same-provider
credential fallback for API-key providers, refreshes Codex OAuth on eligible
401s, records metadata and fallback events, and writes the final response.

This slice is behavior-preserving. It does not change routing, credential
resolution, OAuth refresh behavior, fallback policy, metadata, health recording,
storage, response bodies, or error strings.

## Scope

1. Move non-streaming chat execution from `internal/server/server.go` into a
   new same-package file, `internal/server/chat_nonstream.go`.
2. Move this cluster intact:
   - `nonStreamContext`
   - `chatAttempt`
   - `singleChatContext`
   - `singleChatAttempt`
   - `handleSingleCredentialChat`
   - `handleNonStreamingChat`
3. Move non-streaming chat status and retry helpers with that execution path:
   - `normalizedChatStatus`
   - `localChatStatus`
   - `normalizedChatErrorClass`
   - `localChatErrorClass`
   - `retryableChatAttempt`
   - `shouldRecordChatHealth`
4. Leave streaming execution, stream contexts, stream retry helpers, health
   conversion helpers, `resolvedChatModel`, metadata record helpers, local auth
   wrapper, and response writers unchanged in their current files for this
   slice. `resolvedChatModel` is shared by streaming and non-streaming metadata
   paths, so it should not live in the non-streaming execution file.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/chat_nonstream.go`.
2. Move the non-streaming execution cluster intact.
3. Keep helper names, dispatch calls, metadata records, fallback records, and
   error strings unchanged.
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

1. Is `internal/server/chat_nonstream.go` the right boundary for non-streaming
   chat execution and same-provider credential fallback?
2. Should streaming execution and shared health conversion helpers remain in
   `server.go` for this slice so the move stays focused?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

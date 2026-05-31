# 062 Chat Route Split

## Context

Plans 060 and 061 split `/v1/models` handling and shared server credential
helpers out of `internal/server/server.go`. The file still contains server
construction, local auth middleware, `/v1/chat/completions` request parsing and
dispatch, non-streaming chat execution, streaming chat execution, health
conversion, metadata recording, and response writers.

The architecture describes the request path as:

```text
OpenAI-compatible request parser
  -> strict request validator
    -> model address resolver
      -> routing policy
        -> credential resolver
          -> provider adapter
```

`handleChatCompletions` is the route entrypoint for that path. It owns request
body limiting, OpenAI request decoding, strict request validation, model address
resolution, provider instance lookup, adapter lookup, credential dispatch, and
selection of streaming versus non-streaming execution. Keeping that route
entrypoint in the main server construction file keeps unrelated server
responsibilities mixed together.

This slice is behavior-preserving. It does not change request validation,
model addressing, adapter selection, credential resolution, OAuth behavior,
fallback, streaming, metadata, storage, response bodies, or error strings.

## Scope

1. Move the `/v1/chat/completions` route entrypoint from
   `internal/server/server.go` into a new same-package file,
   `internal/server/chat_route.go`.
2. Move `maxRequestBodyBytes` with `handleChatCompletions`, because it is only
   used by that route entrypoint.
3. Keep non-streaming execution, streaming execution, context structs, retry
   helpers, health conversion, metadata recording, local auth wrapper, and
   response writers unchanged in their current file for this slice.
4. Keep `Handler` in `server.go`; it should continue to register
   `s.handleChatCompletions` from the same package.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/chat_route.go`.
2. Move `maxRequestBodyBytes` and `handleChatCompletions` intact.
3. Keep helper names, dispatch calls, metadata records, and error strings
   unchanged.
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

1. Is `internal/server/chat_route.go` the right boundary for the
   `/v1/chat/completions` route entrypoint and request pipeline dispatch?
2. Should non-streaming and streaming execution helpers remain in `server.go`
   for this slice so the move stays focused on the route entrypoint?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

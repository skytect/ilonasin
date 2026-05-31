# 068 Server Chat Shared Helpers Split

## Context

Plans 060 through 067 split route handling, credential helpers, chat execution,
health conversion, metadata recording, and response writing out of
`internal/server/server.go`. The main server file now contains server
construction, route registration, local auth middleware, and three shared chat
execution helpers:

- `resolvedChatModel`
- `retryableHTTPStatus`
- `fallbackReason`

Those helpers are used by non-streaming and streaming chat execution files.
They are not part of server construction or route wiring, so keeping them in
`server.go` leaves a small execution-helper concern mixed into the bootstrap
file.

This slice is behavior-preserving. It does not change resolved model metadata,
retry decisions, fallback metadata, routing, credential resolution, response
bodies, error strings, or storage.

## Scope

1. Move shared chat execution helpers from `internal/server/server.go` into a
   new same-package file, `internal/server/chat_helpers.go`.
2. Move this cluster intact:
   - `resolvedChatModel`
   - `retryableHTTPStatus`
   - `fallbackReason`
3. Keep route-specific, non-streaming, streaming, credential, health, metadata,
   and response helpers in their current files.
4. Keep server construction, route registration, `MetadataRecorder`, and local
   auth middleware in `server.go` for this slice.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/chat_helpers.go`.
2. Move the three helpers intact.
3. Keep helper names, retryable status set, resolved model fallback behavior,
   and fallback reason behavior unchanged.
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

1. Is `internal/server/chat_helpers.go` the right neutral boundary for helpers
   shared by streaming and non-streaming chat execution?
2. Should local auth middleware and route registration remain in `server.go`
   for this slice so the move stays focused?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

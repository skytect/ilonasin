# 067 Server Response Writers Split

## Context

Plans 060 through 066 split route handling, credential helpers, chat execution,
health conversion, and metadata recording out of `internal/server/server.go`.
The main server file now contains server construction, local auth middleware,
shared retry/model/fallback helpers, and response writer helpers.

The architecture separates the local OpenAI-compatible HTTP API surface from
provider adapters, routing, credential resolution, and metadata-only storage.
Server response writing is a shared HTTP API concern used by the model route,
chat route, non-streaming chat execution, and streaming pre-stream error path.
Keeping response writers in the bootstrap file leaves unrelated HTTP response
formatting mixed with server construction.

This slice is behavior-preserving. It does not change response bodies, content
types, status codes, error strings, routing, credential resolution, fallback,
metadata recording, or storage.

## Scope

1. Move response writer helpers from `internal/server/server.go` into a new
   same-package file, `internal/server/response.go`.
2. Move this cluster intact:
   - `writeRaw`
   - `writeError`
   - `writeJSON`
3. Keep server construction, route registration, local auth middleware,
   `MetadataRecorder`, shared retry/model helpers, and fallback helper in
   `server.go` for this slice.
4. Keep all call sites unchanged across `models.go`, `chat_route.go`,
   `chat_nonstream.go`, and `chat_stream.go`.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/response.go`.
2. Move the three response writer helpers intact.
3. Keep content-type defaults, `openai.Error` wrapping, JSON encoder behavior,
   and `http.ErrHandlerTimeout` handling unchanged.
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

1. Is `internal/server/response.go` the right neutral boundary for shared HTTP
   response helpers?
2. Should route registration and auth middleware remain in `server.go` for this
   slice so the move stays focused?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

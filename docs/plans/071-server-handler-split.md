# 071 Server Handler Split

## Context

Plans 060 through 070 split model routing, credential helpers, chat routing,
chat execution, health conversion, metadata recording, response writing, shared
chat helpers, local API auth middleware, and server dependency interfaces out
of `internal/server/server.go`. The main server file now contains the
`Server` struct, constructors, and route registration through `Handler`.

The architecture treats HTTP routing as its own local API boundary. Route
registration wires the OpenAI-compatible local API paths to handlers and auth
middleware, while `server.go` should primarily describe the server dependency
state and constructor surface.

This slice is behavior-preserving. It does not change paths, methods, auth
middleware, handler dispatch, constructor signatures, response bodies,
credential resolution, metadata recording, or storage behavior.

## Scope

1. Move route registration from `internal/server/server.go` into a new
   same-package file, `internal/server/handler.go`.
2. Move this method intact:
   - `Handler`
3. Keep `Server`, `New`, and `NewWithClock` in `internal/server/server.go`.
4. Keep route handler implementations, auth middleware, response helpers, chat
   execution, model discovery, metadata recording, health conversion,
   dependency interfaces, and credential helpers in their current files.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/handler.go`.
2. Move `Handler` intact.
3. Keep method registrations, local auth wrapping, and `http.NewServeMux`
   behavior unchanged.
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

1. Is `internal/server/handler.go` the right boundary for local API route
   registration?
2. Should `Server`, `New`, and `NewWithClock` remain in `server.go` for this
   slice?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

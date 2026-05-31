# 070 Server Dependency Interfaces Split

## Context

Plans 060 through 069 split model routing, credential helpers, chat routing,
chat execution, health conversion, metadata recording, response writing, shared
chat helpers, and local API auth middleware out of `internal/server/server.go`.
The main server file now contains server construction, route registration, the
`Server` struct, and dependency interfaces.

The architecture keeps local API auth, upstream credentials, provider adapters,
routing, HTTP transport, config, TUI, and SQLite storage as separate
boundaries. The remaining dependency interfaces in `server.go` are boundary
contracts for provider registry access, model cache storage, and metadata-only
storage. Keeping those contracts in the construction file still mixes boundary
definitions with bootstrap and route wiring.

This slice is behavior-preserving. It does not change constructor signatures,
route registration, credential resolution, provider dispatch, metadata
recording, response bodies, storage behavior, or error strings.

## Scope

1. Move server dependency interfaces from `internal/server/server.go` into a
   new same-package file, `internal/server/interfaces.go`.
2. Move this cluster intact:
   - `MetadataRecorder`
   - `ProviderRegistry`
   - `ModelCache`
3. Keep `Server`, `New`, `NewWithClock`, and `Handler` in
   `internal/server/server.go`.
4. Keep route handlers, auth middleware, chat execution, model discovery,
   response helpers, metadata wrapper methods, health conversion helpers, and
   credential helpers in their current files.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/interfaces.go`.
2. Move the three interfaces intact.
3. Keep method names, argument types, return types, and constructor signatures
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

1. Is `internal/server/interfaces.go` the right boundary for server-facing
   storage and provider dependency contracts?
2. Should constructor and route registration remain in `server.go` for this
   slice?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

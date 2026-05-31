# 072 App Bootstrap Split

## Context

Plans 060 through 071 narrowed the server package so construction, dependency
contracts, routing, auth, response writing, metadata recording, health mapping,
and chat execution each have clearer local boundaries.

`internal/app/app.go` is now the largest mixed surface in the codebase. It
contains production command entrypoints, runtime bootstrap, adapter factories,
test-server style smoke checks, fake upstreams, and assertion helpers. The
architecture keeps config, home resolution, SQLite storage, provider registry,
TUI, routing, HTTP transport, and provider adapters as separate boundaries.
The shared app runtime bootstrap is a composition concern and should not stay
buried inside the same file as the smoke-check machinery.

This slice is behavior-preserving. It does not change home resolution, config
loading, directory creation, SQLite opening, provider registry creation,
command behavior, smoke checks, response bodies, storage behavior, or error
strings.

## Scope

1. Move app option and runtime bootstrap definitions from
   `internal/app/app.go` into a new same-package file,
   `internal/app/runtime.go`.
2. Move this cluster intact:
   - `Options`
   - `runtime`
   - `bootstrap`
3. Keep `Serve`, `ServeCheck`, `Manage`, `ManageCheck`, app adapter factories,
   smoke-check helpers, fake upstream servers, and assertion helpers in their
   current file for this slice.
4. Keep constructor signatures and call sites unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/app/runtime.go`.
2. Move the three runtime bootstrap definitions intact.
3. Keep `Options` exported fields, `runtime` fields, bootstrap arguments,
   default writer behavior, safe check home behavior, path creation, and cleanup
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

1. Is `internal/app/runtime.go` the right boundary for app options, runtime
   state, and bootstrap composition?
2. Should production entrypoints and smoke-check helpers remain in `app.go` for
   this slice so the move stays focused?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

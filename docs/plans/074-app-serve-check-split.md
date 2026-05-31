# 074 App Serve Check Split

## Context

Plans 072 and 073 split app runtime bootstrap and production command
entrypoints out of `internal/app/app.go`. The file still contains
non-interactive check entrypoints, check orchestration, fake upstreams, and
many helper assertions.

The architecture requires direct smoke checks for `ilonasin serve` and
`ilonasin manage`. `ServeCheck` is the daemon-oriented smoke orchestration: it
creates a temporary checked home and database, starts the local API handler,
validates local token auth, verifies provider adapter behavior, verifies OAuth
refresh behavior, and confirms selected home metadata was not mutated. Keeping
that entrypoint in the general helper file leaves command-level check
orchestration mixed with low-level helper machinery.

This slice is behavior-preserving. It does not change smoke coverage, temp
directory cleanup, server startup and shutdown, provider adapter wiring,
credential setup, metadata assertions, response bodies, storage behavior, or
error strings.

## Scope

1. Move the `ServeCheck` entrypoint from `internal/app/app.go` into a new
   same-package file, `internal/app/serve_check.go`.
2. Move only this function:
   - `ServeCheck`
3. Keep `ManageCheck`, app adapter factories, fake upstream servers, snapshot
   helpers, and all check assertion helpers in their current files for this
   slice.
4. Keep constructor signatures and CLI call sites unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/app/serve_check.go`.
2. Move `ServeCheck` intact.
3. Keep temporary store creation, fake upstream server setup, local listener
   startup, auth checks, adapter checks, metadata checks, shutdown behavior,
   and exact error strings unchanged.
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

1. Is `internal/app/serve_check.go` the right boundary for daemon smoke-check
   orchestration?
2. Should `ManageCheck` and lower-level helper machinery remain in `app.go`
   for this slice so the move stays focused?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

# 073 App Production Commands Split

## Context

Plans 060 through 072 split server boundaries and moved app runtime bootstrap
into `internal/app/runtime.go`. `internal/app/app.go` still begins with the
production command entrypoints, then immediately continues into large
non-interactive smoke-check flows and helper machinery.

The architecture has one binary with two production subcommands, `serve` and
`manage`. Those production entrypoints compose runtime bootstrap, local API
auth, upstream credential resolution, provider adapters, server routing, and
TUI execution. The check commands are verification scaffolding and should be
kept separate from the production command flow over time.

This slice is behavior-preserving. It does not change command behavior,
bootstrap, provider adapters, server construction, TUI construction, smoke
checks, response bodies, storage behavior, or error strings.

## Scope

1. Move production app command entrypoints from `internal/app/app.go` into a
   new same-package file, `internal/app/commands.go`.
2. Move this cluster intact:
   - `Serve`
   - `Manage`
3. Keep `ServeCheck`, `ManageCheck`, adapter factories, smoke-check helpers,
   fake upstream servers, assertion helpers, and runtime bootstrap in their
   current files for this slice.
4. Keep constructor signatures and CLI call sites unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/app/commands.go`.
2. Move `Serve` and `Manage` intact.
3. Keep upstream service construction, OAuth refresher and login wiring,
   server construction, TUI invocation, printed serve message, and close
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

1. Is `internal/app/commands.go` the right boundary for production `serve` and
   `manage` entrypoint composition?
2. Should `ServeCheck`, `ManageCheck`, and smoke-check helper machinery remain
   in `app.go` for this slice so the move stays focused?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

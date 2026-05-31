# 075 App Manage Check Split

## Context

Plans 072 through 074 split app runtime bootstrap, production command
entrypoints, and daemon smoke-check orchestration out of
`internal/app/app.go`. The file still starts with `ManageCheck`, then continues
into lower-level TUI lifecycle exercises, OAuth fake servers, serve fake
upstreams, and assertion helpers.

The architecture treats `manage` as a first-class TUI command backed by SQLite,
with the TUI allowed to mutate SQLite but not `config.toml`. `ManageCheck` is
the management-oriented smoke orchestration: it exercises local tokens,
upstream credentials, fallback policies, model cache summaries, observability,
OAuth flows, telemetry pruning, and non-interactive TUI rendering while
checking selected home metadata remains unchanged. That command-level
orchestration should be separate from the low-level helper machinery.

This slice is behavior-preserving. It does not change smoke coverage, temp
directory cleanup, TUI check wiring, provider credential setup, metadata
assertions, storage behavior, output, or error strings.

## Scope

1. Move the `ManageCheck` entrypoint from `internal/app/app.go` into a new
   same-package file, `internal/app/manage_check.go`.
2. Move only this function:
   - `ManageCheck`
3. Keep all exercise helpers, fake OAuth servers, fake upstream servers,
   snapshot helpers, adapter factories, and assertion helpers in their current
   files for this slice.
4. Keep constructor signatures and CLI call sites unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/app/manage_check.go`.
2. Move `ManageCheck` intact.
3. Keep the order of exercises, TUI check invocation, selected home snapshot
   comparison, stdout write behavior, and exact error strings unchanged.
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

1. Is `internal/app/manage_check.go` the right boundary for management
   smoke-check orchestration?
2. Should lower-level TUI exercise helpers and fake service machinery remain in
   `app.go` for this slice so the move stays focused?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

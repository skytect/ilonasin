# 076 App Manage Check Exercises Split

## Context

Plans 072 through 075 split app runtime bootstrap, production commands, and the
two smoke-check entrypoints out of `internal/app/app.go`. The file is still the
largest remaining app package file and now starts with management smoke-check
exercise helpers before continuing into fake OAuth servers, serve-check fake
upstreams, provider option validation, HTTP smoke clients, and assertion
helpers.

The architecture treats `manage` as a first-class TUI command backed by SQLite.
The TUI may mutate SQLite, but it must not mutate `config.toml`. The management
smoke exercises are a coherent boundary because they seed temporary SQLite
stores, call TUI exercise functions, and assert that credential, OAuth,
observability, fallback policy, telemetry, and selected-home behavior stay
within those state boundaries.

This slice is behavior-preserving. It does not change smoke coverage, seeded
metadata, temp directory cleanup, fake OAuth server behavior, provider adapter
behavior, storage assertions, output, or error strings.

## Scope

1. Move management smoke-check exercises from `internal/app/app.go` into a new
   same-package file, `internal/app/manage_check_exercises.go`.
2. Move this direct management smoke helper cluster:
   - `exerciseUpstreamCredentialCheck`
   - `exerciseFallbackPolicyCheck`
   - `exerciseLocalTokenCheck`
   - `exerciseModelCacheCheck`
   - `exerciseObservabilityCheck`
   - `exerciseOAuthCheck`
   - `exerciseOAuthDeviceLoginCheck`
   - OAuth device login assertion and failure helpers
   - `exerciseOAuthRefreshCheck`
   - OAuth refresh assertion and failure helpers
   - `exerciseTelemetryPruneCheck`
3. Keep fake OAuth server implementations, serve-check upstream fakes, provider
   option validators, HTTP client helpers, shared snapshot helpers, adapter
   factories, shared OAuth credential seeding, and serve-check assertions in
   their current files for this slice.
4. Keep function names, signatures, exercise ordering, temporary cleanup,
   seeded values, and exact errors unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/app/manage_check_exercises.go`.
2. Move the direct management smoke exercise cluster intact.
3. Let same-package references continue to call fake OAuth server types and
   shared helpers that remain in `app.go`.
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

1. Is `internal/app/manage_check_exercises.go` the right boundary for
   management TUI smoke exercise orchestration?
2. Should fake OAuth server implementations remain in `app.go` for this slice
   so the extraction stays focused?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

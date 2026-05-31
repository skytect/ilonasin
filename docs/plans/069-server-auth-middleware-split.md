# 069 Server Auth Middleware Split

## Context

Plans 060 through 068 split route handling, credential helpers, chat execution,
health conversion, metadata recording, response writing, and shared chat helper
logic out of `internal/server/server.go`. The main server file now contains
server construction, route registration, dependency interfaces, and local API
auth middleware.

The architecture has a distinct local API auth boundary:

```http
Authorization: Bearer <ilonasin_token>
```

Ilonasin client tokens are separate from upstream provider credentials. The
server already enforces that boundary through `withAuth`, but the middleware
still lives in the server construction file.

This slice is behavior-preserving. It does not change auth verification,
request routing, error status, error body, credential resolution, response
bodies, metadata recording, or storage.

## Scope

1. Move local API auth middleware from `internal/server/server.go` into a new
   same-package file, `internal/server/auth.go`.
2. Move this method intact:
   - `withAuth`
3. Keep server construction, route registration, and dependency interfaces in
   `server.go` for this slice.
4. Keep response helpers, route handlers, chat execution, metadata recording,
   health conversion, and credential helpers in their current files.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/auth.go`.
2. Move `withAuth` intact.
3. Keep auth header lookup, verifier call, unauthorized error response, and
   handler dispatch unchanged.
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

1. Is `internal/server/auth.go` the right boundary for local API auth
   middleware?
2. Should route registration remain in `server.go` for this slice?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

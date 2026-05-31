# 061 Server Credential Helpers Split

## Context

Plan 060 moved `GET /v1/models` handling into `internal/server/models.go`.
After that move, `internal/server/server.go` still contains route setup,
request parsing, chat routing, retry/fallback logic, stream handling, metadata
recording, response writers, and shared credential helper logic.

The architecture describes the runtime path as:

```text
strict request validator
  -> model address resolver
    -> routing policy
      -> credential resolver
        -> provider adapter
```

The server already depends on credential resolver interfaces rather than
SQLite, which matches the architecture. The remaining issue is file-level
modularity: shared server credential helpers are embedded in the main server
route file instead of having a clear home.

This slice is behavior-preserving. It does not change credential selection,
OAuth refresh, retry decisions, provider credential construction, routing,
fallback, metadata, storage, or error strings.

## Scope

1. Move shared server credential helpers from `internal/server/server.go` into a
   new same-package file, `internal/server/credentials.go`.
2. Move this cluster intact:
   - `resolveModelCredential`
   - `shouldRefreshOAuthAfterChat401`
   - `shouldRefreshOAuthAfterStream401`
   - `refreshOAuthCredentialForRetryIfBearer`
   - `providerAPIKey`
3. Keep model-specific OAuth helpers in `internal/server/models.go`, because
   they are tied to the `/v1/models` route.
4. Keep `handleChatCompletions`, non-streaming chat, streaming chat, fallback,
   health conversion, metadata recording, local auth wrapper, and response
   writers unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/credentials.go`.
2. Move the shared credential helper cluster intact.
3. Keep the helper names and signatures unchanged so existing call sites in
   `server.go` and `models.go` continue to resolve through the same package.
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

1. Is `internal/server/credentials.go` the right boundary for server-side
   credential resolution and provider credential construction helpers?
2. Should model-specific OAuth 401 helpers stay in `models.go` while chat and
   stream OAuth retry helpers move to the shared credential helper file?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

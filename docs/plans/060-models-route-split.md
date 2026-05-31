# 060 Models Route Split

## Context

Plans 051 through 059 made the provider adapter boundary clearer by splitting
provider validation, request shaping, model discovery, and streaming transport
out of the old mixed provider file.

The server layer now has the next obvious mixed boundary:
`internal/server/server.go` contains the shared server bootstrap, local API
auth wrapper, `/v1/models` route handling, `/v1/chat/completions` route
handling, fallback, health recording, stream response writing, and JSON error
helpers.

The architecture describes a high-level runtime pipeline where the local API
auth layer, OpenAI-compatible request routes, model discovery, routing,
credential resolution, provider adapters, and metadata-only storage are distinct
responsibilities. `GET /v1/models` is its own OpenAI-compatible surface with
model cache fallback and sanitized model response shaping, so it deserves a
separate server file.

This slice is behavior-preserving. It does not change model discovery,
credential resolution, OAuth refresh, model cache fallback, health recording,
sorting, response shape, storage, routing, chat behavior, or error strings.

## Scope

1. Move the `/v1/models` route handler from `internal/server/server.go` into a
   new same-package file, `internal/server/models.go`.
2. Move only model-route-specific helpers with it:
   - `handleModels`
   - `shouldRefreshOAuthAfterModel401`
   - `isOAuthAuthFailure`
3. Keep `resolveModelCredential` in `server.go` for this slice, because it is a
   shared bearer credential helper used by both `/v1/models` and Codex chat.
   A later slice can give shared credential helpers their own file.
4. Keep `Handler`, `withAuth`, `/v1/chat/completions`, chat fallback,
   streaming, health conversion helpers, record helpers, and response writers
   unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/models.go`.
2. Move the model route cluster intact:
   - local `model` response struct inside `handleModels`,
   - cache grouping and fallback,
   - provider discovery loop,
   - OAuth model 401 refresh retry,
   - model cache replacement,
   - sorting and OpenAI-compatible list response.
3. Move the two model-specific OAuth helper methods:
   - `shouldRefreshOAuthAfterModel401`
   - `isOAuthAuthFailure`
4. Leave shared helpers in their current files and call them across the same
   package.
5. Run `gofmt` on touched Go files.
6. Manually review the diff before smoke checks. The Go diff should be a move
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

1. Is `internal/server/models.go` the right boundary for the `/v1/models`
   route and sanitized model-list response shaping?
2. Should shared bearer credential resolution stay in `server.go` for this
   slice, given it is used by both model discovery and Codex chat?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

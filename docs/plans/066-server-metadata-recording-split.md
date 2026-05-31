# 066 Server Metadata Recording Split

## Context

Plans 060 through 065 split server route handling, credential helpers, chat
routing, chat execution, streaming execution, and health event conversions out
of `internal/server/server.go`. The main server file still contains server
bootstrap, local auth middleware, shared retry/model helpers, metadata recording
wrappers, and JSON response helpers.

The architecture requires metadata-only observability and explicitly separates
HTTP routing, provider execution, and storage-facing metadata recording. The
server already depends on a `MetadataRecorder` interface, but the small wrapper
methods that write request, stream, health, and fallback metadata still live in
the bootstrap file.

This slice is behavior-preserving. It does not change recorded fields,
metadata storage behavior, request routing, credential resolution, fallback
policy, response bodies, or error strings.

## Scope

1. Move metadata recording wrapper methods from `internal/server/server.go`
   into a new same-package file, `internal/server/metadata.go`.
2. Move this cluster intact:
   - `record`
   - `recordWithID`
   - `recordStream`
   - `recordHealth`
   - `recordFallbacks`
3. Keep `MetadataRecorder` in `server.go` for this slice, because it is part of
   the server constructor surface.
4. Keep health conversion helpers in `health.go`, chat execution helpers in
   their chat files, response writers in `server.go`, server bootstrap in
   `server.go`, and local auth middleware unchanged.
5. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/metadata.go`.
2. Move the five recording wrapper methods intact.
3. Keep method names, nil checks, request ID handling, and fallback event
   mutation unchanged.
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

1. Is `internal/server/metadata.go` the right neutral boundary for server-side
   metadata recording wrappers?
2. Should `MetadataRecorder` remain in `server.go` for this slice because it is
   part of the constructor and dependency surface?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

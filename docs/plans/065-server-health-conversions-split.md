# 065 Server Health Conversions Split

## Context

Plans 060 through 064 split server route handling, credential helpers, chat
routing, non-streaming chat execution, and streaming chat execution out of
`internal/server/server.go`. The main server file is now mostly bootstrap,
local auth middleware, shared retry and metadata helpers, health conversion
helpers, and response writers.

The architecture requires metadata-only observability with explicit provider,
credential, model, HTTP status, error class, and retry-after health records.
Those health records are built from provider adapter results across multiple
server flows:

- non-streaming chat execution,
- streaming chat execution,
- model discovery.

Keeping those conversion helpers in the server bootstrap file still mixes
observability mapping with HTTP server construction.

This slice is behavior-preserving. It does not change health event content,
request routing, credential resolution, fallback policy, metadata recording,
storage, response bodies, or error strings.

## Scope

1. Move health event conversion helpers from `internal/server/server.go` into a
   new same-package file, `internal/server/health.go`.
2. Move this cluster intact:
   - `healthFromChatAttempt`
   - `healthFromSingleChatAttempt`
   - `healthFromModelDiscovery`
3. Keep stream-specific health conversions in `internal/server/chat_stream.go`,
   because plan 064 placed them with streaming execution.
4. Keep metadata recording methods in `server.go` for this slice:
   - `record`
   - `recordWithID`
   - `recordStream`
   - `recordHealth`
   - `recordFallbacks`
5. Keep shared retry helpers, resolved model helpers, response writers, server
   bootstrap, and local auth middleware unchanged.
6. Do not add provider features, request fields, persistence, migrations, or
   permanent tests.

## Implementation

1. Create `internal/server/health.go`.
2. Move the three health conversion helpers intact.
3. Keep helper names and returned `metadata.HealthEvent` fields unchanged.
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

1. Is `internal/server/health.go` the right neutral boundary for health event
   conversions shared by model discovery and non-streaming chat?
2. Should stream-specific health conversions remain in `chat_stream.go` for
   this slice because they are tightly coupled to stream summaries?
3. Are compile, vet, build, and CLI smoke checks enough for this move-only
   extraction?

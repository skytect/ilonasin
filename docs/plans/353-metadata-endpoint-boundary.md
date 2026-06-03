# 353 Metadata Endpoint Boundary

## Context

Request endpoint names are metadata-domain values. The current code repeats the
same endpoint allowlist in multiple places:

- `internal/server/request_metadata_endpoints.go` owns endpoint string
  constants used when writing request metadata;
- `internal/management/snapshot_sanitize.go` has a separate
  `safeEndpointString` switch for management DTO sanitization;
- `internal/tui/display_sanitize.go` has a separate `safeEndpointDisplay`
  switch for rendering.

The duplicated switches currently agree, but this is fragile. Adding, removing,
or renaming a metadata endpoint can leave server recording, management
sanitization, and TUI rendering out of sync. `docs/ilonasin-architecture.md`
expects metadata-only observability and daemon-owned safe projections, so the
metadata domain should own the allowed endpoint identifiers.

## Goal

Move request endpoint identifiers and endpoint sanitization into
`internal/metadata`, then make server, management, and TUI use that single
metadata-domain boundary.

## Scope

1. Add a small `internal/metadata/endpoints.go` file that defines:
   - `EndpointChatCompletions`;
   - `EndpointResponses`;
   - `EndpointAnthropicMessages`;
   - `EndpointAnthropicCountTokens`;
   - `SafeEndpoint(value string) string`.
2. Update `internal/server/request_metadata_endpoints.go` so existing
   unexported server constants alias the metadata constants. This preserves the
   current server call sites and endpoint values.
3. Update management snapshot sanitization to call `metadata.SafeEndpoint`
   instead of a local endpoint switch, and remove the local
   `safeEndpointString` helper.
4. Update TUI display sanitization to call `metadata.SafeEndpoint` instead of a
   local endpoint switch, preserving the existing `safeEndpointDisplay` helper
   name for TUI call sites.
5. Do not change endpoint string values, request metadata schema, storage,
   route paths, public API behavior, logs, TUI layout, management DTO shape,
   or provider behavior.

## Verification

Before implementation review:

1. Review the diff manually for scope, import cleanup, and unchanged endpoint
   string values.
2. Run a temporary focused smoke, removed before commit, covering:
   - all four endpoint constants sanitize to themselves through
     `metadata.SafeEndpoint`;
   - unknown, blank, whitespace-padded unknown, and unsafe-marker values sanitize
     to empty;
   - server metadata constants still equal the exported metadata constants;
   - management and TUI endpoint helpers use the shared sanitizer.
3. Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/metadata ./internal/management ./internal/tui ./internal/server
go test ./...
go vet ./...
```

4. Build a temporary `ilonasin` binary, start `ilonasin serve` with an isolated
   temporary `ILONASIN_HOME`, verify management health over the Unix socket,
   run a short `ilonasin manage` TUI smoke, terminate the daemon, and clean up
   temporary files.

## Expected Outcome

- Endpoint identifiers and endpoint allowlisting live in the metadata domain.
- Server, management, and TUI stay in sync for request endpoint labels.
- No endpoint values, route behavior, storage behavior, management payload
  shape, or TUI layout changes.

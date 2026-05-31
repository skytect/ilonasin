# 078 Daemon Management Local Tokens

## Context

The architecture describes `ilonasin manage` as the local control plane, but the
previous text left direct SQLite versus admin socket deferred. The latest
direction resolves that question: `manage` should become a TUI client, and the
daemon should own all management operations and SQLite mutation.

The current code violates that direction for local API tokens:

- `app.Manage` constructs `credentials.Service{Repo: rt.Store}`.
- `tui.Model` calls `m.tokens.Create`, `m.tokens.List`, and
  `m.tokens.Disable` directly.
- `manage --check` exercises token creation and disablement without going
  through any daemon-owned boundary.

This slice migrates only the local API token management path because it is small
and self-contained. Later slices can migrate upstream credentials, OAuth,
observability, and pruning through the same daemon-owned management shape.

## Goal

Move local API token management behind a daemon-owned management API and make
the TUI token flow use that API-shaped client.

This must be a real architectural movement toward daemon ownership, not a
TUI-shaped facade over direct SQLite.

## Scope

1. Update `docs/ilonasin-architecture.md` to record the resolved direction:
   - `serve` owns the management service and SQLite mutations.
   - `manage` is a client of a daemon-owned internal management API.
   - direct `manage` SQLite access is legacy during migration and must be
     removed progressively.
2. Add `internal/management` for daemon-owned management operations.
   - Define explicit request/response DTOs for local API token operations.
   - Add `Service` backed by the existing credential domain service.
   - Keep repository and SQLite types out of DTOs and client interfaces.
3. Add an internal management HTTP surface to the daemon for local API token
   operations only:
   - list local tokens,
   - create a local token,
   - disable a local token.
4. Expose the management surface only on a separate local Unix-domain HTTP
   socket owned by `serve`.
   - Do not mount management routes on the public OpenAI-compatible mux.
   - Place the socket under a daemon runtime directory inside the selected home
     with directory mode `0700`.
   - Include a stable daemon identity component derived from the resolved config
     path and resolved database path so `manage --config X` cannot silently talk
     to a daemon for a different config/database in the same home.
   - Before binding, probe an existing socket path first. If a daemon responds,
     refuse startup. Only unlink the path when connect fails and the path is a
     socket. Never unlink an arbitrary file.
   - Remove the live socket on daemon shutdown where practical.
   - Use filesystem permissions as the first auth boundary for this slice.
   - Do not introduce a persisted bearer token for management.
5. Add a management HTTP client for local token operations.
   - The client should expose operation-shaped methods matching the DTOs, not
     storage-shaped repository methods.
6. Change the TUI local-token dependency from `credentials.LocalTokenManager`
   to a local-token management client interface.
   - TUI token list, create, and disable must call the client.
   - Other TUI dependencies can remain direct for now, clearly legacy.
7. Change `app.ManageCheck` to start an internal daemon server with the Unix
   management socket and exercise the TUI local-token path through the
   management HTTP client.
8. Change `app.Manage` local-token behavior to use the daemon socket.
   - If the daemon management socket is unavailable, interactive `manage` must
     fail with a concise daemon-not-running error rather than falling back to
     direct SQLite for local-token operations.
   - Non-token flows can remain direct legacy dependencies for now, but they
     must be visibly scoped as migration work and not presented as final.
9. Keep the OpenAI-compatible public API behavior unchanged.

## Non-Goals

- Do not migrate upstream credentials, OAuth, model cache, observability, or
  pruning in this slice.
- Do not persist management API auth material.
- Do not add management endpoints for provider credentials or OAuth.
- Do not expose prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, full bearer tokens, or full account IDs.
- Do not change SQLite schema or migrations.
- Do not add permanent tests.
- Do not push.

## Design Constraints

1. Management operation contracts must use explicit request/response structs.
2. `internal/tui` must not import `internal/storage/sqlite`.
3. The management HTTP handlers must live on the daemon/server side, not in the
   TUI package.
4. Token creation responses may carry the newly generated local token only back
   to the management client, and only for immediate TUI reveal. It must not be
   logged or persisted in plaintext.
5. The management socket path must not include tokens or secrets.
6. `serve --check` must prove the public OpenAI-compatible endpoints are still
   unchanged.
7. `manage --check` must prove the migrated token flow goes through HTTP
   management transport, not direct `credentials.Service`.
8. Any remaining direct manage dependencies must be named as legacy migration
   work in the architecture text and must not be presented as final.
9. Public HTTP clients must not be able to reach management routes through the
   public `server.bind` listener.
10. Management-socket clients must not be able to reach public `/v1/*` routes
    through the management listener.

## Implementation Plan

1. Add `internal/management/tokens.go`.
   - DTOs: `ListLocalTokensResponse`, `CreateLocalTokenRequest`,
     `CreateLocalTokenResponse`, `DisableLocalTokenRequest`,
     `DisableLocalTokenResponse`.
   - Interface: `LocalTokenClient`.
   - Service methods backed by `credentials.LocalTokenManager`.
2. Add management socket and handler wiring on the daemon side.
   - Add a dedicated management HTTP handler or server that is not the public
     `Server.Handler()` mux.
   - Add routes under an internal prefix such as
     `/_ilonasin/manage/local-tokens`.
   - Return JSON DTOs only.
   - Add a lightweight health or identity endpoint for live-socket probing.
3. Add `internal/management/http_client.go`.
   - Implement `LocalTokenClient` over HTTP using a Unix-domain socket
     transport.
   - Normalize HTTP errors into safe local errors.
   - Never include token strings in errors.
4. Update app server construction.
   - `Serve` and `ServeCheck` build and start a separate management socket
     server next to the public HTTP daemon.
   - `ManageCheck` starts an internal daemon HTTP server and passes an HTTP
     local-token client into the TUI check path.
   - `Manage` builds a local-token HTTP client from the configured management
     socket path and fails if that socket cannot be reached.
5. Update `internal/tui`.
   - Replace the local token field type with `management.LocalTokenClient`.
   - Token list/create/disable call the management client.
   - Leave non-token interfaces unchanged for this slice.
6. Update `tui.ExerciseTokenLifecycle` and `manage --check` so token lifecycle
   exercises the HTTP client path.
7. Update `docs/ilonasin-architecture.md` management wording.
8. Run `gofmt`.
9. Add smoke helpers that assert:
   - public listener returns not found for management routes,
   - management socket returns not found for public `/v1/models`,
   - socket runtime directory is `0700`,
   - a second daemon refuses to unlink or replace a live socket,
   - non-socket files at the socket path are not unlinked,
   - shutdown removes the live socket where practical,
   - TUI token lifecycle used the management HTTP client.
10. Inspect for direct local-token service usage from TUI, `app.Manage`, and
    `exerciseLocalTokenCheck`.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
! rg -n "credentials\\.LocalTokenManager|credentials\\.Service\\{Repo:" internal/tui
! rg -n "tui\\.Run\\([^\\n]*credentials\\.Service|tokenService := credentials\\.Service" internal/app/commands.go internal/app/manage_check.go
! rg -n "ExerciseTokenLifecycle\\(ctx, credentials\\.Service|credentials\\.Service\\{Repo: store\\}" internal/app/manage_check_exercises.go
! rg -n "storage/sqlite" internal/tui
rm -rf "$tmp" "$tmpbin"
```

The negative searches must fail the smoke if the migrated token path still uses
the legacy direct service from `internal/tui`, `app.Manage`, `app.ManageCheck`,
or `exerciseLocalTokenCheck`.

## Review Questions

1. Is local API token management the right first daemon-owned vertical slice?
2. Does the management HTTP shape avoid freezing the current SQLite/domain
   service shape into the future daemon contract?
3. Is a Unix-domain management socket the right bootstrap for local `serve` to
   `manage` communication in this architecture?
4. Does leaving non-token management flows direct for now still move the
   codebase toward the requested final state, or should the slice require a
   broader first transport?

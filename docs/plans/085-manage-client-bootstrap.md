# 085 Manage Client Bootstrap

## Context

`docs/ilonasin-architecture.md` says SQLite is the mutable source of truth, the
daemon owns SQLite mutation, and `ilonasin manage` should be a client of the
daemon-owned local management API.

Recent slices moved production TUI local-token mutations, snapshot reads,
upstream credential mutations, OAuth login/refresh, fallback-policy updates,
and telemetry pruning through the management socket. Production `Manage` still
uses full app `bootstrap`, which opens SQLite and returns `rt.Store`, even
though the TUI no longer receives a direct store-backed mutation dependency.

That keeps `manage` coupled to SQLite lifecycle and migrations for no current
production need.

## Goal

Make production `ilonasin manage` bootstrap as a management client instead of a
daemon/storage runtime.

After this slice, production `Manage` should resolve home/config, configure
logging, build the provider registry, construct the Unix management client, and
run the TUI through the daemon socket. It should not open SQLite, run storage
migrations, close a store, or instantiate store-backed credential services.

## Architecture Inputs

- `AGENTS.md`
- all Markdown files under `docs/**`
- especially `docs/ilonasin-architecture.md`
- `docs/plans/072-app-bootstrap-split.md`
- `docs/plans/073-app-production-commands-split.md`
- `docs/plans/078-daemon-management-local-tokens.md`
- `docs/plans/079-daemon-management-snapshot.md`
- `docs/plans/082-daemon-management-upstreams.md`
- `docs/plans/083-daemon-management-oauth.md`
- `docs/plans/084-daemon-management-pruning.md`

## Scope

1. Split app runtime bootstrap into shared core bootstrap plus storage bootstrap.
   - Core bootstrap resolves writers, safe check home behavior, home directory,
     config loading, path directory creation, logging setup, and provider
     registry construction.
   - Put the client/core runtime in a SQLite-free source file so import-level
     guards can prove it does not depend on storage.
   - Storage bootstrap wraps core bootstrap and opens SQLite for daemon/server
     commands and smoke checks that intentionally exercise storage.
2. Add a narrow client runtime shape for production `Manage`.
   - Include home dir, config path, resolved config, provider registry, logger,
     and cleanup.
   - Do not include `*sqlite.Store`.
3. Update production `Manage`.
   - Use the client runtime.
   - Build the management socket path from resolved home/config/database paths.
   - Fail if the management snapshot cannot be loaded.
   - Pass the management client to all production TUI management slots.
4. Keep daemon and smoke storage behavior where it belongs.
   - `Serve` continues using storage bootstrap.
   - `ServeCheck` and `ManageCheck` may continue using storage bootstrap because
     they start isolated in-process daemons and validate SQLite invariants.
   - Production-like final `tui.Check` in `ManageCheck` continues using the
     management client.
5. Add source-level smoke guards.
   - `manage --check` should fail if production `Manage` calls storage
     bootstrap, opens SQLite, references `rt.Store`, or constructs
     store-backed credential services.
   - `manage --check` should also fail if the client/core runtime used by
     production `Manage` imports or uses SQLite, exposes a store field, calls
     storage bootstrap, or constructs store-backed credential services.
   - Guard the management socket call so production `Manage` uses the resolved
     home dir, resolved config path, and resolved database path.
   - Existing TUI mutation-argument guards remain.
6. Keep public API behavior unchanged.

## Non-Goals

- Do not migrate model-cache or observability read fallback paths in this slice.
- Do not remove storage access from `serve`, `serve --check`, or targeted smoke
  helpers.
- Do not change management HTTP routes, DTOs, provider adapters, storage schema,
  migrations, config format, or TUI key bindings.
- Do not change default config creation behavior.
- Do not add permanent tests.
- Do not push.

## Design Constraints

1. Production `Manage` must not open SQLite or require SQLite migration success.
2. Production `Manage` must still fail clearly when the daemon management socket
   is unavailable or serving a different config/database identity.
3. Client bootstrap may create the home/config/log/cache directories just as
   existing bootstrap does; this slice removes SQLite coupling, not all local
   filesystem setup.
4. Logging remains configured through the existing logging package and retains
   redaction behavior.
5. The management socket path must use the resolved config path and resolved
   database path, matching the daemon identity rules from plan 078.
6. Direct SQLite references may remain in `ManageCheck` and helper functions
   where they are smoke scaffolding, but source guards should distinguish those
   from production `Manage`.
7. No prompts, completions, request bodies, response bodies, raw provider
   payloads, raw SSE chunks, tool arguments, tool results, full bearer tokens,
   full provider request IDs, or full account IDs may be logged or surfaced.

## Implementation Plan

1. Refactor runtime bootstrap.
   - Introduce an internal core runtime struct without storage.
   - Move shared setup logic from `bootstrap` into a core helper.
   - Keep `bootstrap` as the storage-opening wrapper so existing storage-backed
     call sites stay simple.
2. Update production `Manage`.
   - Call the core/client bootstrap helper.
   - Remove `rt.Store.Close()` from production `Manage`.
   - Defer client runtime cleanup so logging outputs keep the existing
     lifecycle.
   - Keep the snapshot preflight before launching the TUI.
3. Extend smoke guards.
   - Parse or source-scan `internal/app/commands.go` to prove `Manage` does not
     call storage bootstrap, open SQLite, refer to `Store`, or construct direct
     credential services.
   - Parse or source-scan the client bootstrap/runtime implementation to prove
     it does not import `internal/storage/sqlite`, call `sqlite.Open`, expose a
     `Store` field, call the storage-opening `bootstrap`, or construct direct
     credential services.
   - Assert production `Manage` constructs `management.SocketPath` with resolved
     client runtime home, config, and database paths.
   - Add a smoke check proving production `Manage` with no running daemon fails
     at the management snapshot preflight and does not create/open the SQLite
     database.
   - Keep the existing AST guard proving production TUI mutation arguments use
     `tokenClient`.
4. Cleanup.
   - Remove unused imports caused by moving SQLite out of production `Manage`.
   - Run `gofmt`.
   - Read through the diff before smoke checks.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
```

`go test ./...` is a compile/package check only. No permanent test files will be
added.

## Review Questions

1. Is shared core bootstrap the right boundary, or should production `Manage`
   use a completely separate config/logging loader?
2. Does keeping config/default directory creation in client bootstrap preserve
   current UX while still removing the important SQLite coupling?
3. Are source guards plus `manage --check` enough to prevent production
   `Manage` from regaining direct storage dependencies?

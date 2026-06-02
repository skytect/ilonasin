# 255 App IO Secret Source Boundary

## Goal

Decouple app-level IO scrubber wiring from the concrete SQLite store without
changing IO logging behavior.

The current `internal/app/io_secrets.go` imports `internal/storage/sqlite` only
to call `ListCredentialSecretMaterial`. That makes a privacy bootstrap helper
know about a storage implementation detail. This slice should keep exact
configured-secret scrubbing intact while making the app boundary depend on a
small secret-source interface.

## Scope

1. Add a small unexported app-local interface for listing configured credential secret
   material:
   - method shape should match the existing storage method;
   - keep it in `internal/app`; do not export it or reuse it outside app
     bootstrap/refresh-hook wiring;
   - the interface must not expose secrets to management snapshots, TUI, normal
     logs, or public APIs.
2. Update `refreshIOConfiguredSecrets` to accept the interface instead of
   `*sqlite.Store`.
3. Update `ioSecretRefreshHook` to accept the same interface.
4. Keep the existing SQLite method `ListCredentialSecretMaterial` as the
   implementation. Do not move secret querying into logging or management.
5. Update `Serve` and management-server wiring to pass the store through the
   interface.
6. Preserve the existing behavior:
   - no-op when IO logger or secret source is nil;
   - startup refresh replaces configured secrets;
   - credential mutation hook adds ephemeral secrets and refreshes configured
     secrets;
   - the hook keeps using the captured stable daemon context for configured
     secret refreshes rather than the mutation call context;
   - refresh-hook errors remain ignored inside the callback, as today.

## Boundaries

- No database schema changes.
- No management API, DTO, TUI, provider, server route, Anthropic, config, or
  normal logging changes.
- No new secret exposure path.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
rg -n '"ilonasin/internal/storage/sqlite"' internal/app/io_secrets.go && exit 1 || true
go test ./...
go vet ./...
```

Run a temporary focused in-package smoke, then remove it before commit:

- fake a secret source that returns one configured secret;
- refresh an IO logger from that source and verify the configured secret is
  scrubbed from JSON, form, and plain text;
- mutate the source through the refresh hook with a new ephemeral secret and a
  replacement configured secret;
- verify the new ephemeral and configured secrets are scrubbed;
- verify a nil IO logger and nil secret source are no-ops.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify the API/providers/usage/logs
   chrome renders.
5. Start a second disposable daemon with IO capture enabled and no stored
   credentials, verifying startup still succeeds and creates `ilonasin-io.log`.
6. Remove all temporary artifacts.

## Acceptance

- `internal/app/io_secrets.go` no longer imports concrete SQLite storage.
- Exact configured-secret IO scrubbing behavior is unchanged.
- Secret material remains loaded only into the in-memory IO scrubber.
- Compile, vet, focused smoke, serve smoke, manage smoke, capture-on smoke,
  and implementation review pass.

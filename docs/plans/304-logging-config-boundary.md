# 304 Logging Config Boundary

## Context

`docs/ilonasin-architecture.md` separates config bootstrap from runtime
cross-cutting services. Logging is a shared infrastructure boundary, but
`internal/logging` currently imports `internal/config` only to read logging
fields and the log directory in `Setup` and `SetupIO`.

This creates an unnecessary package-level dependency from logging back into the
configuration model. The app bootstrap should translate config into
logging-owned options, then logging should operate on those options.

This slice is boundary-only. It must not change log formats, default outputs,
log file names, IO capture policy, secret scrubbing, provider IO capture,
management behavior, TUI behavior, config loading, or public API behavior.

## Plan

1. Add logging-owned option structs in `internal/logging`:
   - `Options` for normal logging level, format, outputs, and log directory;
   - `IOOptions` for IO capture enablement and log directory.
2. Change `logging.Setup` to accept `logging.Options` instead of
   `config.Config`.
3. Change `logging.SetupIO` to accept `logging.IOOptions` instead of
   `config.Config`.
4. In `internal/app/runtime_core.go`, translate `config.Config` to those
   logging options at the callsite.
5. Remove the `internal/config` import from `internal/logging`.
6. Review the code before checks for behavior drift, nil/empty defaults,
   permissions, and accidental changes outside app/logging/docs.

## Verification

Run:

```sh
! rg -n '"ilonasin/internal/config"' internal/logging
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/logging
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting
`ilonasin serve` with a temporary `ILONASIN_HOME` and config, checking
management health over the Unix socket, running `ilonasin manage` under bounded
narrow and wide terminals, and cleaning up the daemon and temp directory.

Run a temporary focused logging smoke, then remove it before commit:

- `Setup` with file output creates `ilonasin.log` in the configured log
  directory with mode `0600` where Unix permissions apply;
- `SetupIO` with capture disabled returns nil and does not create
  `ilonasin-io.log`;
- `SetupIO` with capture enabled creates `ilonasin-io.log` in the configured
  log directory with mode `0600` where Unix permissions apply;
- configured and marker-shaped secrets are still scrubbed from IO body text.

## Acceptance

- `internal/logging` no longer imports `internal/config`.
- App bootstrap remains the only changed callsite translating config into
  logging setup options.
- Normal logs and IO logs still use the same defaults, filenames, permissions,
  formats, and capture gating.
- No server, provider, management, storage, TUI, or config parser behavior
  changes are introduced.

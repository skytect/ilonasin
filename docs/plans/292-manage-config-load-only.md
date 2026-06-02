# 292 Manage Config Load Only

## Goal

Keep `config.toml` static from the `ilonasin manage` path.

The architecture says `config.toml` is static runtime configuration and the TUI
must not mutate it. Current `Manage` calls `bootstrapClient`, which uses the same
`config.LoadOrCreate` path as `Serve`; when no default config exists, `manage`
can create `config.toml`.

## Scope

1. Split application config bootstrap mode:
   - `serve` keeps first-run default config creation when no explicit `--config`
     is provided.
   - `manage` loads the resolved config path only and returns a clear missing
     config error if it does not exist.
2. Keep explicit `--config` behavior unchanged for both commands: missing files
   should error.
3. Preserve the existing config path and data/log/cache/database normalization
   used by daemon socket identity.
4. Preserve post-config runtime bootstrap side effects such as creating
   data/log/cache directories and setting up logging after an existing config is
   loaded.
5. Do not change config file shape, defaults, provider registry behavior,
   logging setup, SQLite schema, management socket identity, TUI rendering, or
   daemon management routes.
6. Do not add permanent tests.

## Verification

1. Source checks:
   - `rg -n 'bootstrapClient|bootstrapCore|LoadOrCreate' internal/app internal/config`
     shows `manage` no longer reaches a create-if-missing config path, while
     `serve` still can.
2. Fresh-home direct CLI smoke:
   - build a temporary `ilonasin` binary,
   - run `ILONASIN_HOME="$tmp/home" ilonasin manage` with a short timeout and no
     config file,
   - confirm it exits nonzero with a clear missing-config error,
   - confirm `$tmp/home/config.toml` was not created.
3. Explicit missing-config smoke:
   - `serve --config "$tmp/missing.toml"` exits nonzero and does not create the
     missing file,
   - `manage --config "$tmp/missing.toml"` exits nonzero and does not create the
     missing file.
4. Serve/manage daemon smoke:
   - build a temporary `ilonasin` binary,
   - run `serve` in a fresh temporary home with no explicit config and confirm it
     creates default config,
   - run `manage` with no explicit config against that daemon and confirm it
     uses the same default config/socket identity,
   - run `serve` with an explicit temporary config and confirm management health,
   - create a local token over the management API,
   - run `manage --config "$tmp/config.toml"` under a short PTY timeout and
     confirm API, providers, usage, and logs render.
4. Standard checks:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`

## Acceptance

- `ilonasin manage` never creates `config.toml`.
- `ilonasin serve` keeps first-run bootstrap behavior.
- Existing daemon and TUI smokes still pass.

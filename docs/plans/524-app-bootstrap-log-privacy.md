# 524 App Bootstrap Log Privacy

## Context

Plan 522 found that the normal `app_bootstrap` log records raw local filesystem
paths through `home_dir` and `config_file`.

`docs/ilonasin-architecture.md` requires metadata-only normal observability and
plan 520 aligned normal structured logging to redact URL and path-like key
families. The bootstrap log should not persist local home or config file paths
in normal application logs.

## Goal

Remove raw filesystem path values from the normal `app_bootstrap` log while
preserving useful operational metadata.

## Scope

1. Update `internal/app/runtime_core.go`.
2. Replace raw `home_dir` and `config_file` log attributes with safe non-path
   metadata, such as configured status booleans or stable literal values.
3. Preserve bootstrap behavior, config loading, home resolution, directory
   creation, logging setup, IO logging, provider registry setup, server,
   management, TUI, storage, provider behavior, and SQLite behavior.
4. Do not change global structured-log redaction policy in this slice.
5. Do not add permanent tests.

## Out Of Scope

- Broad slog call-site audit.
- Changing config paths or home resolution.
- Changing management socket identity paths.
- Changing logging output format, event names, or IO logging.
- Changing provider, server, management, TUI, storage, or SQLite behavior.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- `app_bootstrap` output no longer includes raw `home_dir` or `config_file`
  path attributes.
- The bootstrap log still records `event=app_bootstrap`.
- The bootstrap log still records safe operational metadata.

Run:

```sh
rg -n 'app_bootstrap|home_dir|config_file|log_output' internal/app/runtime_core.go docs/plans/524-app-bootstrap-log-privacy.md
gofmt -w internal/app/runtime_core.go
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/524-app-bootstrap-log-privacy.md
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, keepalive disabled, and configured
   provider instances.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a
   pseudo-terminal.
5. Confirm TUI output includes ANSI color sequences.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- Normal bootstrap logs no longer persist local home or config file paths.
- `app_bootstrap` remains available as an operational event.
- Runtime bootstrap behavior is unchanged.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated `internal/app/runtime_core.go` only.
- Replaced raw `home_dir` and `config_file` bootstrap log attributes with safe
  literal metadata: `home=configured` and `config=loaded`.
- Preserved the `event=app_bootstrap` and `log_output=configured` attributes.
- Preserved bootstrap behavior, config loading, home resolution, directory
  creation, logging setup, IO logging, provider registry setup, server,
  management, TUI, storage, provider behavior, and SQLite behavior.

## Verification Record

- Senior plan review: three reviewers reported no findings.
- Temporary focused harness: passed. It started `ilonasin serve` with isolated
  `ILONASIN_HOME`, captured JSON stderr logs, verified
  `event=app_bootstrap`, `home=configured`, `config=loaded`, and
  `log_output=configured`, and verified the raw temporary home/config path plus
  old `home_dir` and `config_file` keys were absent. Temporary files were
  removed before commit.
- `rg -n 'app_bootstrap|home_dir|config_file|log_output' internal/app/runtime_core.go docs/plans/524-app-bootstrap-log-privacy.md`:
  passed.
- `gofmt -w internal/app/runtime_core.go`: passed.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty"
  docs/plans/524-app-bootstrap-log-privacy.md`: passed for the new untracked
  plan file. Git returned status `1` only because the files differ, with no
  whitespace findings.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a
  pseudo-terminal. Both bounded runs exited by timeout with status `124` as
  expected.
- TUI color capture: passed. The 80-column capture contained 436 SGR sequences
  across 9 unique 256-color foreground/background codes, and the 140-column
  capture contained 658 SGR sequences across 10 unique 256-color
  foreground/background codes.
- Senior implementation review: three reviewers reported no findings.
- Cleanup: temporary home, binary, config, terminal captures, temporary log
  capture, and daemon process were removed.

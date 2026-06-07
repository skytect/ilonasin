# 515 IO Logger Error Privacy

## Context

Plan 512 found that `IOLogger.reportError` writes raw `err.Error()` strings to
normal application logs when IO-log encode, open, rotate, write, or rollback
operations fail. `docs/plans/077-structured-application-logging.md` requires
normal logs to avoid raw `err.Error()` values except for local static
marker-free errors and to prefer normalized safe classes.

IO logging itself may contain scrubbed request and response bodies when enabled,
but failures in the IO logging subsystem must still be reported through
metadata-only normal logs.

## Goal

Preserve useful IO logging failure diagnostics while removing raw error text
from normal logs.

## Scope

1. Update `internal/logging/io.go`.
2. Replace the normal-log `error` attribute in `IOLogger.reportError` with a
   fixed normalized `error_class`.
3. Classify common local IO failure categories without including paths,
   filenames, raw wrapped error text, or provider/client payloads:
   - encode failure;
   - permission errors;
   - missing path errors;
   - already-exists errors;
   - invalid path errors;
   - short write;
   - closed file;
   - other IO logger failure.
4. Preserve existing stage names, event name, IO record writing behavior,
   rotation behavior, rollback behavior, scrubbing behavior, setup behavior,
   and return behavior.
5. Keep the change local to the logging package and this plan.
6. Do not add permanent tests.

## Out Of Scope

- Changing normal application logging setup or redaction globally.
- Changing IO log file format or rotation policy.
- Changing IO body scrubbing.
- Changing provider, server, management, TUI, config, SQLite, or routing
  behavior.
- Adding new committed test files.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- `reportError` emits `event=io_log_write_failed`, stage, and normalized
  `error_class`.
- `reportError` does not emit an `error` attribute containing raw
  `err.Error()`.
- wrapped encode, permission, missing path, already-exists, invalid path, short
  write, closed file, and generic errors classify to stable safe classes.

Run:

```sh
rg -n 'reportError|io_log_write_failed|slog\.String\("error"|error_class|ioLoggerErrorClass' internal/logging/io.go docs/plans/515-io-logger-error-privacy.md
gofmt -w internal/logging/io.go
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/515-io-logger-error-privacy.md
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

- Normal IO logger failure logs no longer contain raw `err.Error()` text.
- Normal IO logger failure logs retain event, stage, and a useful safe
  `error_class`.
- IO logging functionality and error return behavior are otherwise unchanged.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated `internal/logging/io.go` only.
- Replaced the normal-log `error` attribute in `IOLogger.reportError` with
  `error_class`.
- Added `ioLoggerErrorClass` with fixed classes for encode, permission,
  missing path, already-exists, invalid path, short write, closed file, and
  generic IO logger failures.
- Preserved existing event name, stage names, IO record writing, rotation,
  rollback, scrubbing, setup, and return behavior.

## Verification Record

- Senior plan review: one reviewer reported no findings; two reviewers found
  low-risk plan issues. The verification regex was corrected, and the temporary
  harness scope was expanded to cover every class listed in the plan.
- Temporary focused harness: passed. It verified `event`, `stage`, and
  `error_class` output, absence of a raw `error` attr, absence of wrapped raw
  marker text, and classification for encode, permission, missing path,
  already-exists, invalid path, short write, closed file, and generic errors.
  Temporary harness was removed before commit.
- `rg -n 'reportError|io_log_write_failed|slog\.String\("error"|error_class|ioLoggerErrorClass' internal/logging/io.go docs/plans/515-io-logger-error-privacy.md`:
  passed. Code matches contain `error_class` and the classifier; the raw
  `slog.String("error"` attr no longer appears in code.
- `gofmt -w internal/logging/io.go`: passed.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty" docs/plans/515-io-logger-error-privacy.md`:
  passed for the new untracked plan file. Git returned status `1` only because
  the files differ, with no whitespace findings.
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
- TUI color capture: passed. The 80-column capture contained 108 256-color SGR
  foreground sequences, and the 140-column capture contained 175.
- Cleanup: temporary home, binary, config, terminal captures, temporary harness,
  and daemon process were removed.
- Senior implementation review: three reviewers reported no findings.

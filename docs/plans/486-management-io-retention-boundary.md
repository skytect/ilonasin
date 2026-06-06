# 486 Management IO Retention Boundary

## Context

Plan 485 added IO log retention metadata to the management runtime snapshot. The
implementation works, but `internal/app/management.go` now imports
`internal/config` only so `startManagementServer` can read `IOMaxBytes` and
`IOMaxFiles`.

That is broader than the architecture boundary needs. App bootstrap should
translate config into runtime-owned values, then management startup should
receive only safe runtime metadata.

## Goal

Remove the config package dependency from `internal/app/management.go` by
passing a narrow app-local IO retention value into management startup.

## Scope

1. Add a small unexported app type for IO retention metadata.
2. Build that type from `rt.Config.Logging` in `Serve`.
3. Change `startManagementServer` to accept the narrow type instead of
   `config.LoggingConfig`.
4. Preserve management `RuntimeStatus` JSON, TUI rendering, logging behavior,
   IO rotation behavior, config loading, and public API behavior.
5. Do not add permanent tests.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run a direct CLI smoke with a temporary binary and isolated home:

- start `ilonasin serve`;
- confirm management snapshot over the Unix socket still includes
  `io_max_bytes` and `io_max_files`;
- run bounded `ilonasin manage` through a PTY and confirm the IO policy pane
  still renders.

## Acceptance

- `internal/app/management.go` no longer imports `internal/config`.
- Runtime snapshot IO retention fields are unchanged.
- No behavior changes outside app bootstrap argument shaping.

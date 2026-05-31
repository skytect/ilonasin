# 100 Remove Permanent Checks

## Context

The repo now has a large permanent smoke-check surface:

- `serve --check` and `manage --check` CLI flags,
- `app.ServeCheck` and `app.ManageCheck`,
- check-only app files under `internal/app/*check*.go`,
- fake upstream and check helpers embedded in `internal/app/app.go`,
- SQLite migration smoke helpers under `internal/storage/sqlite/smoke.go`,
- TUI `Exercise*` helpers only used by management checks.

This has become counterproductive because feature changes need matching edits in
the permanent check harness. The project direction is to avoid permanent tests
and use direct compile, vet, and short-lived CLI smoke commands instead.

## Goal

Delete permanent check and test code, including `--check`, while keeping the
production `serve` and `manage` commands intact.

## Scope

1. Remove `--check` from the CLI.
   - Delete `serve --check` and `manage --check` flag handling.
   - Update usage text to only mention `serve` and `manage` with optional
     `--config`.
   - Do not leave compatibility aliases for `--check`.
2. Delete check-only app entrypoints and files.
   - Delete `internal/app/serve_check.go`.
   - Delete `internal/app/manage_check.go`.
   - Delete `internal/app/manage_check_exercises.go`.
   - Delete `internal/app/manage_snapshot_check.go`.
   - Delete `internal/app/management_check.go`.
   - Delete `internal/app/responses_provider_tools_check.go`.
   - Delete `internal/app/responses_custom_tool_check.go`.
3. Remove check-only code embedded in production files.
   - Remove fake upstream servers, check validators, check exercise functions,
     and check-only HTTP helpers from `internal/app/app.go`.
   - Remove check-only bootstrap paths from `internal/app/runtime.go` and
     `internal/app/runtime_core.go`, including `checkSafeHome` and
     `ilonasin-check-*` temporary home behavior.
   - Keep production helpers that are used by `Serve`, `Manage`, bootstrap,
     logging, management runtime, provider registry, or real app flows.
4. Remove permanent smoke helpers outside app.
   - Delete `internal/storage/sqlite/smoke.go`.
   - Remove the check-only `tui.Check` entrypoint and its `newCheckModel`
     bootstrap.
   - Remove TUI `Exercise*` helpers if they are no longer referenced by
     production code.
5. Update current agent instructions.
   - Remove `--check` from `AGENTS.md` smoke commands.
   - Keep historical plan docs unchanged.
6. Do not touch unrelated work currently dirty in:
   - `internal/provider/chat.go`
   - `internal/provider/http_models.go`
   - `internal/server/models.go`

## Non-Goals

- Do not change provider behavior, routing, OAuth, quota, model discovery, or
  Codex compatibility logic.
- Do not add tests or replacement permanent check harnesses.
- Do not update historical plan docs that mention old check commands.
- Do not push.

## Acceptance

- `fd '(_test\\.go$|check|smoke)' .` shows no production check/test/smoke code,
  except historical docs under `docs/plans/`.
- `rg -n 'checkSafeHome|ilonasin-check|ServeCheck|ManageCheck|--check|RunMigrationSmokeCheck|Exercise[A-Z]|newCheckModel|tui\\.Check'`
  shows no live code references outside historical docs.
- `go test ./...` passes as a compile/package check.
- `go vet ./...` passes.
- A fresh binary builds.
- Direct short-lived `ilonasin serve` smoke starts the daemon, creates the
  management socket, and exits cleanly when terminated.
- Direct `ilonasin manage` smoke is run against that daemon with a short timeout.
  If it cannot be made noninteractive without a TTY, record the exact result and
  verify the management snapshot preflight separately.

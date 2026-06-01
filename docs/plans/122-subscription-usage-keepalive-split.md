# 122 Subscription Usage Keepalive Status Split

## Context

Plans 104, 119, 120, and 121 established Codex subscription usage management,
response aggregation, response sanitization, and DTO boundaries. The remaining
`internal/management/subscription_usage.go` owns route/client definitions,
refresh orchestration, provider usage recording, provider-window extraction,
and keepalive status rendering.

The architecture separates daemon management reads from provider behavior and
keeps keepalive execution disabled until a verified wire-level output cap exists.
The current keepalive code is status-only: it reports disabled by default, or
`unavailable_output_cap_unverified` when enabled, and returns the configured or
default schedule. Moving this status helper out of refresh orchestration keeps
the subscription usage route easier to audit without changing runtime behavior.

This slice is behavior-preserving. It does not add keepalive execution, does not
change config mutation, does not change upstream Codex calls, and does not
change management response JSON.

## Goal

Move subscription keepalive status rendering into a focused same-package file
without changing behavior.

After this slice, `subscription_usage.go` still owns route/client definitions,
refresh orchestration, provider usage recording, and provider-window extraction.
`subscription_usage_keepalive.go` owns keepalive status rendering and schedule
normalization for the management response.

## Scope

1. Create `internal/management/subscription_usage_keepalive.go`.
2. Move these helpers from `subscription_usage.go` unchanged:
   - `keepaliveStatus`
   - `safeScheduleTime`
3. Preserve current status behavior:
   - disabled config reports `status = "disabled"`,
   - enabled config reports `status = "unavailable_output_cap_unverified"`,
   - `output_cap_verified` remains false,
   - empty schedule falls back to `07:00`, `12:00`, `17:00`, `22:00`,
   - invalid schedule entries become empty strings.
4. Preserve all JSON fields and route behavior.
5. Do not add keepalive execution.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No keepalive scheduler.
- No provider keepalive requests.
- No config changes or TUI config mutation.
- No response shape changes.
- No sanitizer policy changes.
- No provider usage fetch changes.
- No SQLite schema or persistence changes.
- No TUI layout changes.
- No broader split of `subscription_usage.go` in this slice.

## Implementation

1. Add `subscription_usage_keepalive.go` with `package management`.
2. Move `keepaliveStatus` and `safeScheduleTime` intact.
3. Add only the imports needed by the moved helpers.
4. Remove any now-unused imports from `subscription_usage.go`.
5. Run `gofmt` on touched files.
6. Review the diff before smoke checks. The Go diff should be move-only except
   import cleanup.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
git diff --check
```

Then run the direct CLI smoke:

- build a fresh `ilonasin` binary,
- start `ilonasin serve --config "$cfg"` with a temporary `ILONASIN_HOME`,
- wait for the management socket,
- verify `/_ilonasin/manage/snapshot`,
- verify `/_ilonasin/manage/subscription-usage`,
- run `ilonasin manage --config "$cfg"` under a short PTY timeout,
- terminate the daemon and remove temporary files.

## Acceptance

- Compile/package check passes.
- Vet passes.
- `git diff --check` passes.
- Fresh binary builds.
- Direct `serve` smoke exposes snapshot and subscription usage routes.
- Direct `manage` smoke reaches the daemon-backed TUI path and exits cleanly or
  times out with status 124.
- Diff is move-only except import cleanup.

## Review Questions

1. Is `subscription_usage_keepalive.go` the right boundary for status-only
   keepalive response behavior?
2. Should `safeScheduleTime` move with `keepaliveStatus` because it is schedule
   normalization, not response sanitization?
3. Are compile, vet, diff whitespace, and direct serve/manage smokes enough for
   this move-only extraction?

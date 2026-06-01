# 119 Subscription Usage Response Split

## Context

Plans 104, 105, and 113 added Codex subscription usage refresh, management DTOs,
and clearer window summaries for per-account and pooled subscription usage. The
current `internal/management/subscription_usage.go` now owns several concerns:
route constants and client interface, DTOs, refresh orchestration, OAuth bearer
resolution, provider usage calls, response aggregation, window labeling,
sanitization, and keepalive status.

The architecture says the management API should be modular, daemon-owned, and
metadata-only. Splitting response aggregation away from refresh orchestration
keeps the subscription usage boundary easier to audit: refresh code talks to
OAuth/provider/storage dependencies, while response code turns safe stored
snapshots into management DTOs.

This slice is behavior-preserving. It does not change upstream Codex calls,
SQLite writes, management routes, response JSON, sanitizer policy, keepalive
status, TUI rendering, config, or provider behavior.

## Goal

Move subscription usage response-shaping helpers into a focused same-package
file without changing behavior.

After this slice, `subscription_usage.go` still owns route/client definitions,
DTOs, refresh orchestration, provider usage recording, sanitizer, and keepalive
status. `subscription_usage_response.go` owns stored-snapshot-to-response
aggregation and window formatting helpers.

## Scope

1. Create `internal/management/subscription_usage_response.go`.
2. Move these helpers from `subscription_usage.go` unchanged:
   - `subscriptionUsageResponse`
   - `subscriptionUsageRow`
   - `subscriptionUsageAggregates`
   - `subscriptionUsageWindows`
   - `subscriptionUsagePoolWindows`
   - `subscriptionUsagePoolWindow`
   - `latestSubscriptionObserved`
   - `earliestTime`
   - `windowLabel`
   - `remainingPercent`
   - `boundedPercent`
   - `boundedPercentPoints`
   - `cloneTime`
3. Keep these helpers in `subscription_usage.go` because they are tied to
   provider refresh/recording or sanitizer/keepalive concerns:
   - `windowUsed`
   - `windowMinutes`
   - `windowReset`
   - `sanitizeSubscriptionUsageResponse`
   - `keepaliveStatus`
   - `safeScheduleTime`
4. Preserve account and pool aggregation behavior, sorting, window labels, reset
   cloning, percent clamping, and account-percent point totals.
5. Preserve all JSON fields and route behavior.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No response shape changes.
- No new routes or config fields.
- No provider usage fetch changes.
- No SQLite schema or persistence changes.
- No keepalive execution changes.
- No TUI layout changes.
- No broader split of `subscription_usage.go` in this slice.

## Implementation

1. Add `subscription_usage_response.go` with `package management`.
2. Move the listed response helpers intact.
3. Add only the imports needed by the moved helpers.
4. Remove now-unused imports from `subscription_usage.go`.
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

1. Is `subscription_usage_response.go` the right boundary for stored snapshot to
   management DTO aggregation?
2. Should provider-window extraction helpers remain with refresh/recording code
   for this slice?
3. Are compile, vet, diff whitespace, and direct serve/manage smokes enough for
   this move-only extraction?

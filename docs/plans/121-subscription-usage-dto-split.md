# 121 Subscription Usage DTO Split

## Context

Plans 104, 105, 113, 119, and 120 established the Codex subscription usage
management route, moved route DTOs into the subscription usage module, added
window summaries, and split response aggregation and response sanitization.
`internal/management/subscription_usage.go` still contains route/client
definitions, DTO definitions, refresh orchestration, provider usage recording,
and keepalive status.

The architecture target keeps management API operations modular and auditable.
The subscription usage DTOs are route-surface declarations used by the
standalone subscription usage route, the full management snapshot, and TUI
rendering. Keeping them in a focused file makes the API shape easier to review
separately from refresh behavior and provider interaction.

This slice is behavior-preserving. It does not change upstream Codex calls,
SQLite writes, management routes, response JSON, sanitizer policy, keepalive
status, TUI rendering, config, or provider behavior.

## Goal

Move subscription usage DTO definitions into a focused same-package file without
changing behavior.

After this slice, `subscription_usage.go` still owns the subscription usage route
path, management client interface, refresh orchestration, provider usage
recording, and keepalive status. `subscription_usage_dto.go` owns subscription
usage response DTO declarations. Existing response aggregation and sanitization
files continue to use those DTOs unchanged.

## Scope

1. Create `internal/management/subscription_usage_dto.go`.
2. Move these type declarations from `subscription_usage.go` unchanged:
   - `SubscriptionUsageRow`
   - `SubscriptionUsageAggregate`
   - `SubscriptionUsageWindow`
   - `SubscriptionUsagePoolWindow`
   - `KeepaliveStatus`
   - `SubscriptionUsageResponse`
3. Preserve field order, field names, JSON tags, and type names.
4. Keep `PathSubscriptionUsage` and `SubscriptionUsageClient` in
   `subscription_usage.go`, because they are route/client boundary definitions.
5. Preserve all route behavior and full snapshot embedding behavior.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No response shape changes.
- No sanitizer policy changes.
- No new routes or config fields.
- No provider usage fetch changes.
- No SQLite schema or persistence changes.
- No keepalive execution changes.
- No TUI layout changes.
- No broader split of `subscription_usage.go` in this slice.

## Implementation

1. Add `subscription_usage_dto.go` with `package management`.
2. Move the listed type declarations intact.
3. Add only the imports needed by moved DTOs.
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

1. Is `subscription_usage_dto.go` the right boundary for route response DTOs
   after response and sanitizer splits?
2. Should `PathSubscriptionUsage` and `SubscriptionUsageClient` remain in
   `subscription_usage.go` for this slice?
3. Are compile, vet, diff whitespace, and direct serve/manage smokes enough for
   this move-only extraction?

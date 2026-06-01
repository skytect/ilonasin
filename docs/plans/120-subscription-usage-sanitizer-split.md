# 120 Subscription Usage Sanitizer Split

## Context

Plans 104, 105, 113, and 119 established the Codex subscription usage management
route, DTOs, window summaries, and response aggregation split. The remaining
`internal/management/subscription_usage.go` still mixes refresh orchestration
with response sanitization for the standalone subscription usage route and the
full management snapshot.

The architecture requires management views to remain metadata-only and avoid
exposing full account IDs, bearer tokens, raw provider payloads, balances,
credits, prompts, completions, request/response bodies, raw SSE chunks, tool
arguments, tool results, and provider request IDs. Keeping subscription usage
sanitization focused makes that safety boundary easier to audit without changing
how refresh or response aggregation works.

This slice is behavior-preserving. It does not change upstream Codex calls,
SQLite writes, management routes, response JSON, sanitizer policy, keepalive
status, TUI rendering, config, or provider behavior.

## Goal

Move subscription usage response sanitization into a focused same-package file
without changing behavior.

After this slice, `subscription_usage.go` still owns route/client definitions,
DTOs, refresh orchestration, provider usage recording, and keepalive status.
`subscription_usage_response.go` owns response aggregation. The new
`subscription_usage_sanitize.go` owns sanitizing subscription usage responses.

## Scope

1. Create `internal/management/subscription_usage_sanitize.go`.
2. Move `sanitizeSubscriptionUsageResponse` from `subscription_usage.go` into
   the new file unchanged.
3. Preserve all calls to shared sanitizer helpers:
   - `safeMachineString`
   - `safeSnapshotString`
4. Keep `safeScheduleTime` in `subscription_usage.go` because it is keepalive
   schedule normalization, not response sanitization.
5. Preserve all sanitized fields and redaction behavior.
6. Preserve all JSON fields and route behavior.
7. Do not add permanent tests.
8. Do not push.

## Non-Goals

- No sanitizer policy changes.
- No response shape changes.
- No new routes or config fields.
- No provider usage fetch changes.
- No SQLite schema or persistence changes.
- No keepalive execution changes.
- No TUI layout changes.
- No broader split of `subscription_usage.go` in this slice.

## Implementation

1. Add `subscription_usage_sanitize.go` with `package management`.
2. Move `sanitizeSubscriptionUsageResponse` intact.
3. Remove any now-unused imports from `subscription_usage.go`.
4. Run `gofmt` on touched files.
5. Review the diff before smoke checks. The Go diff should be move-only except
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

1. Is `subscription_usage_sanitize.go` the right boundary for standalone
   subscription usage response sanitization?
2. Should `safeScheduleTime` remain with keepalive status rather than move with
   response sanitization?
3. Are compile, vet, diff whitespace, and direct serve/manage smokes enough for
   this move-only extraction?

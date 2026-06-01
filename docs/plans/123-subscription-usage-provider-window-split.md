# 123 Subscription Usage Provider Window Split

## Context

Plans 119 through 122 split subscription usage response aggregation,
sanitization, DTOs, and keepalive status out of
`internal/management/subscription_usage.go`. The remaining file owns the route
path, management client interface, refresh orchestration, OAuth bearer
resolution, provider usage calls, storage recording, and three small helpers for
extracting safe values from provider Codex rate-limit windows.

The architecture keeps provider-specific behavior behind provider adapters, but
management refresh code still has to translate the provider adapter's typed
`CodexRateLimitWindow` values into safe storage fields. Moving those extraction
helpers into a focused same-package file keeps refresh orchestration easier to
read while preserving the current typed provider boundary.

This slice is behavior-preserving. It does not change upstream Codex calls,
SQLite writes, management routes, response JSON, sanitizer policy, keepalive
status, TUI rendering, config, or provider behavior.

## Goal

Move subscription usage provider-window extraction helpers into a focused
same-package file without changing behavior.

After this slice, `subscription_usage.go` owns route/client definitions, refresh
orchestration, OAuth bearer resolution, provider usage calls, and storage
recording. `subscription_usage_provider.go` owns converting a typed
`provider.CodexRateLimitWindow` into safe management storage scalar values.

## Scope

1. Create `internal/management/subscription_usage_provider.go`.
2. Move these helpers from `subscription_usage.go` unchanged:
   - `windowUsed`
   - `windowMinutes`
   - `windowReset`
3. Preserve nil handling, UTC reset cloning, and percent/window-minute values.
4. Preserve all storage writes and response behavior.
5. Do not add permanent tests.
6. Do not push.

## Non-Goals

- No provider adapter changes.
- No Codex usage payload changes.
- No response shape changes.
- No sanitizer policy changes.
- No keepalive execution changes.
- No SQLite schema or persistence changes.
- No TUI layout changes.
- No broader split of `subscription_usage.go` in this slice.

## Implementation

1. Add `subscription_usage_provider.go` with `package management`.
2. Move the listed helpers intact.
3. Add only the imports needed by moved helpers.
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

1. Is `subscription_usage_provider.go` the right boundary for typed provider
   window extraction used during usage refresh?
2. Should these helpers remain in management rather than provider because they
   shape storage fields from an already-normalized provider DTO?
3. Are compile, vet, diff whitespace, and direct serve/manage smokes enough for
   this move-only extraction?

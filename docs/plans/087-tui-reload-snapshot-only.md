# 087 TUI Reload Snapshot Only

## Context

`docs/ilonasin-architecture.md` says `ilonasin manage` is a client of the
daemon-owned management API and direct TUI SQLite access is legacy. Plan 079
made production read loading use the daemon management snapshot. Plan 086
removed direct model-cache and observability readers from production TUI
entrypoints and smoke view checks.

The TUI model still has legacy direct metadata readers for upstream credentials
and OAuth state:

- `upstreamReader management.UpstreamMetadataReader`
- `oauthReader credentials.OAuthMetadataReader`
- `reloadDirect`
- TUI smoke helpers that call those direct readers after seeding SQLite

Production `Run` and `Check` already require a `management.SnapshotClient`, but
the internal TUI model can still be assembled in direct-reader mode by helper
paths. That keeps the old architecture alive inside `internal/tui`.

## Goal

Make all TUI reloads snapshot-only.

After this slice, `Model.reload` should always require and use
`management.SnapshotClient`. The TUI model should not store direct upstream or
OAuth metadata readers, and `reloadDirect` should be gone. Targeted app smoke
helpers may still seed and verify SQLite, but any TUI view state they exercise
must come from a management snapshot client.

## Architecture Inputs

- `AGENTS.md`
- all Markdown files under `docs/**`
- especially `docs/ilonasin-architecture.md`
- `docs/plans/079-daemon-management-snapshot.md`
- `docs/plans/082-daemon-management-upstreams.md`
- `docs/plans/083-daemon-management-oauth.md`
- `docs/plans/086-tui-snapshot-only-reads.md`

## Scope

1. Simplify TUI model construction.
   - Remove `upstreamReader` and `oauthReader` fields from `Model`.
   - Remove direct metadata reader parameters from `NewModel` and
     `newCheckModel`.
   - Keep mutation clients as management-shaped interfaces:
     local tokens, upstream credentials/fallback policies, OAuth, and pruning.
2. Make reload snapshot-only.
   - `Model.reload` should fail when `m.snapshot` is nil.
   - Delete `reloadDirect`.
   - Keep snapshot application and DTO-to-view conversion unchanged for this
     slice.
3. Convert TUI smoke helpers.
   - `ExerciseTokenLifecycle` takes a snapshot client for reloads and a
     management local-token client for mutations.
   - `ExerciseUpstreamCredentialLifecycle` takes a snapshot client for reloads
     and a management upstream client for mutations.
   - `ExerciseFallbackPolicyLifecycle` takes a snapshot client for reloads and a
     management upstream client for mutations.
   - `ExerciseOAuthFallbackPolicySummary`, `ExerciseOAuthSummary`,
     `ExerciseOAuthRefresh`, `ExerciseOAuthDeviceLogin`, and
     `ExerciseOAuthDeviceLoginFailure` take snapshot clients for display state.
   - Failure subchecks may keep small management-shaped failing mutation fakes,
     but they must not use direct metadata readers.
4. Update app smoke helpers.
   - Pass the existing management HTTP clients as snapshot clients where an
     in-process management daemon is running.
   - Where a smoke helper only needs a static display state, build a
     `management.Service.LoadManagementSnapshot` response and pass the existing
     snapshot check client.
   - Continue using storage/services in app smoke helpers only for seeding,
     verifying persistence, and provider-domain assertions.
5. Remove direct mutation adapter dead code if no longer needed.
   - Delete `directUpstreamMutations` and `directOAuthMutations`.
   - Use management HTTP clients for live mutation paths and explicit
     management-shaped fakes for failure-only paths.
6. Extend source guards.
   - `manage --check` should fail if `internal/tui` contains
     `UpstreamMetadataReader`, `OAuthMetadataReader`, `upstreamReader`,
     `oauthReader`, `reloadDirect`, direct local-token list calls, direct
     upstream list calls, direct fallback list calls, direct OAuth list calls,
     or direct provider-account list calls.
   - `manage --check` should fail if app smoke helpers pass domain services as
     TUI metadata readers.
   - `manage --check` should fail if `internal/tui` contains
     `directUpstreamMutations` or `directOAuthMutations`.

## Non-Goals

- Do not change management snapshot DTOs, snapshot sanitization, HTTP routes,
  storage schema, migrations, provider adapters, request routing, TUI rendering
  text, quota recording, pruning semantics, fallback policy behavior, OAuth
  behavior, or account pooling behavior.
- Do not remove direct storage seeding or verification from app smoke helpers.
- Do not convert TUI view rows from credential domain structs to management DTOs
  in this slice; snapshot mapping can remain the bridge for now.
- Do not add permanent tests.
- Do not push.

## Design Constraints

1. Every TUI reload must go through `management.SnapshotClient`.
2. A missing snapshot client is an error, not a reason to fall back.
3. TUI helper failure cases must not leak raw provider payloads, bearer tokens,
   OAuth tokens, account IDs, prompts, completions, request bodies, response
   bodies, SSE chunks, tool data, balances, or credits.
4. App smoke helpers may use direct domain services for seeding and independent
   persistence checks, but not as TUI read dependencies.
5. Existing production `app.Manage` and production-like `ManageCheck` must keep
   passing the management HTTP client in every TUI management slot.
6. Source guards should cover both TUI model internals and app smoke call sites
   so direct-reader mode cannot return unnoticed.

## Implementation Plan

1. Update `internal/tui`.
   - Remove direct reader fields and constructor parameters.
   - Change `reload` to require `m.snapshot`.
   - Delete `reloadDirect`.
   - Update helper constructors and exercise functions to use snapshot clients.
2. Update app smoke helpers.
   - Pass management HTTP clients to TUI exercises for live snapshot reloads
     after mutations.
   - Build service snapshots for static summary checks.
   - Keep resolver/service checks outside the TUI where they validate domain
     behavior.
3. Remove dead fakes/adapters.
   - Delete direct mutation adapters that only existed to bridge domain services
     into TUI management-shaped APIs if they are no longer referenced.
4. Update `manage --check` guards.
   - Reject direct reader fields, interfaces, function parameters, reload
     methods, and app call-site patterns.
   - Reject local-token direct list calls in TUI reload paths while keeping
     create and disable mutation calls allowed.
   - Reject direct domain-to-management mutation adapters in `internal/tui`.
   - Keep previous guards for snapshot-only model-cache and observability reads.
5. Cleanup.
   - Run `gofmt`.
   - Read through the diff before smoke checks.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
```

`go test ./...` is a compile/package check only. No permanent test files will be
added.

## Review Questions

1. Is deleting `reloadDirect` the right next architecture step now that
   production reads and model-cache/observability helpers are snapshot-backed?
2. Are the listed smoke helper conversions enough to preserve behavioral
   coverage without keeping direct TUI metadata readers?
3. Are the source guards strong enough to prevent direct TUI read mode from
   returning?

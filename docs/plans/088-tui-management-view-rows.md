# 088 TUI Management View Rows

## Context

`docs/ilonasin-architecture.md` says `ilonasin manage` should be a client of the
daemon-owned management API. Plans 079, 086, and 087 moved TUI reads to the
management snapshot and removed direct TUI metadata readers.

The TUI still converts snapshot DTOs back into credential domain metadata before
rendering and mutating:

- `[]credentials.UpstreamCredentialMetadata`
- `[]credentials.FallbackPolicyMetadata`
- `[]credentials.OAuthCredentialMetadata`
- `[]credentials.ProviderAccountMetadata`

That keeps `internal/tui` coupled to credential storage/domain row shapes even
though the daemon management snapshot already provides explicit allowlisted
management DTOs for the same display state.

## Goal

Make TUI credential, fallback, OAuth, and provider-account view state use
`internal/management` DTOs directly.

After this slice, snapshot rows for these views should not be converted back
into credential domain metadata inside `internal/tui`. The TUI may still import
`internal/credentials` for shared domain constants and sentinel errors that
have not yet been moved behind management contracts.

## Architecture Inputs

- `AGENTS.md`
- all Markdown files under `docs/**`
- especially `docs/ilonasin-architecture.md`
- `docs/plans/079-daemon-management-snapshot.md`
- `docs/plans/086-tui-snapshot-only-reads.md`
- `docs/plans/087-tui-reload-snapshot-only.md`

## Scope

1. Change TUI view row fields to management DTOs.
   - `Model.credentials` becomes `[]management.UpstreamCredential`.
   - `Model.fallbackPolicies` becomes `[]management.FallbackPolicy`.
   - `Model.oauthRows` becomes `[]management.OAuthCredential`.
   - `Model.accountRows` becomes `[]management.ProviderAccount`.
2. Apply snapshot rows directly.
   - Remove `upstreamCredentialsFromSnapshot`.
   - Remove `fallbackPoliciesFromSnapshot`.
   - Remove `oauthCredentialsFromSnapshot`.
   - Remove `providerAccountsFromSnapshot`.
   - Do not mutate the snapshot response backing slices while filtering visible
     rows. Use non-mutating filters or defensive copies before assigning model
     state.
   - Keep existing snapshot conversion for provider, model-cache, and
     observability rows out of scope.
3. Update TUI helpers to use management DTO types.
   - `visibleUpstreamCredentials`
   - `visibleFallbackPolicies`
   - `fallbackPolicyEnabled`
   - upstream disable, fallback enable, fallback disable, and OAuth refresh
     selection paths.
4. Keep current user-visible rendering and behavior unchanged.
5. Extend `manage --check` source guards so `internal/tui` cannot reintroduce
   the credential metadata view row types or the removed snapshot conversion
   functions.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- Do not change management snapshot JSON DTOs, daemon routes, HTTP clients, or
  storage interfaces.
- Do not move credential constants or sentinel errors in this slice.
- Do not convert provider, model-cache, or observability TUI rows away from
  their existing domain structs.
- Do not change rendering text, fallback behavior, OAuth behavior, account
  pooling, quota recording, pruning semantics, migrations, or provider adapters.

## Design Constraints

1. TUI display state for credentials, fallback policies, OAuth credentials, and
   provider accounts must be management API row types.
2. The TUI must not store or render raw provider payloads, bearer tokens, OAuth
   tokens, full account IDs, request bodies, response bodies, prompts,
   completions, SSE chunks, tool arguments, tool results, balances, or credits.
3. Management snapshot sanitization remains the daemon-side boundary. TUI
   rendering must keep its defensive display redaction.
4. App smoke helpers may keep using credential domain services for seeding and
   independent provider-domain assertions, but not for TUI view rows.

## Implementation Plan

1. Update TUI row field types and snapshot application.
   - Assign `snapshot.UpstreamCredentials`, `snapshot.FallbackPolicies`,
     `snapshot.OAuthCredentials`, and `snapshot.ProviderAccounts` directly.
   - Delete the four DTO-to-credential conversion helpers.
2. Update typed helpers and call sites.
   - Change visible-row filters and `fallbackPolicyEnabled` to accept
     management DTO slices.
   - Make visible-row filters allocate output slices instead of using
     `rows[:0]`, because the snapshot response is management API input and
     should be treated as immutable by the TUI.
   - Keep credential kind constants from `internal/credentials` for now.
   - Keep OAuth challenge state out of scope because it is command state rather
     than snapshot view state and carries the device-login handle capability.
3. Add source guards.
   - Reject `credentials.UpstreamCredentialMetadata`,
     `credentials.FallbackPolicyMetadata`,
     `credentials.OAuthCredentialMetadata`, and
     `credentials.ProviderAccountMetadata` in `internal/tui/tui.go`.
   - Reject the removed conversion helper names in `internal/tui/tui.go`.
   - Add a positive AST guard that asserts the four `Model` fields are exactly
     `[]management.UpstreamCredential`, `[]management.FallbackPolicy`,
     `[]management.OAuthCredential`, and `[]management.ProviderAccount`.
4. Cleanup.
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

1. Is converting these TUI view rows to management DTOs the right next
   architecture step after snapshot-only reloads?
2. Should this slice also convert OAuth challenge state to the management DTO,
   or keep that as a separate cleanup because it is command state rather than
   snapshot view state?
3. Are the proposed source guards enough to stop this specific credential-row
   coupling from returning?

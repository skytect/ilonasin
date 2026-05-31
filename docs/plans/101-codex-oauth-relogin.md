# 101 Codex OAuth Relogin

## Context

Codex OAuth login currently inserts a new OAuth credential every time device
login completes. SQLite stores a unique provider/account hash in
`provider_accounts`, so logging into the same Codex account again can fail with
a duplicate credential path. That makes recovery from an invalidated refresh
token awkward: the user has to manually remove or mutate old state before
logging in again.

The architecture expects OAuth login and refresh to be daemon-owned management
operations, while keeping raw OAuth tokens, full account IDs, and raw provider
payloads out of logs, snapshots, and display.

## Goal

Make Codex OAuth relogin work through the existing management/TUI login flow:
logging into an account that already exists should replace that account's OAuth
token bundle and safe display metadata atomically, without exposing sensitive
material and without requiring manual deletion first.

## Scope

1. Add an OAuth relogin/upsert repository operation.
   - Match existing OAuth credentials by `provider_instance_id` and
     `account_hash`, not by full account ID.
   - Use a conflict-safe transaction: do not rely on insert-then-catch around
     `provider_accounts(provider_instance_id, account_hash)`.
   - If no matching account exists, keep the current insert behavior.
   - If a matching account exists, update the existing OAuth credential in one
     transaction:
     - replace access and refresh token secret material,
     - update expiry and scopes,
     - set `last_refresh_at` to `NULL` because relogin is not refresh activity,
     - clear refresh failure class/description,
     - update safe account display label and plan label,
     - clear `disabled_at` so relogin re-enables the account,
     - preserve the existing local credential ID and fallback group.
   - If a matching provider-account row exists with `credential_id NULL`, a
     missing credential, or a non-OAuth credential, attach it to a newly created
     OAuth credential instead of leaving the unique account row as a blocker.
   - Full account IDs may be held only transiently in memory for hash derivation
     and sanitization, then discarded. They must not be logged, returned,
     rendered, or persisted.
   - Update existing `credential_secrets` rows in place when they exist. Insert
     missing `oauth_access` or `oauth_refresh` secret rows only when repairing an
     incomplete existing OAuth credential, then rewire `oauth_tokens` to those
     rows inside the same transaction.
2. Route device-login completion through that upsert.
   - Keep the management API shape stable; `CompleteOAuthDeviceLogin` still
     returns the resulting credential metadata.
   - Do not add direct SQLite access to the TUI.
3. Improve the TUI text minimally.
   - Make the existing `l` action read as login/relogin so the recovery path is
     discoverable.
   - Do not start the larger TUI revamp in this plan.
4. Keep privacy boundaries intact.
   - Do not store full account IDs separately.
   - Do not log or render raw tokens, full account IDs, provider payloads, or
     request/response bodies.

## Non-Goals

- Do not implement browser OAuth login.
- Do not add account deletion as the primary recovery path.
- Do not redesign the TUI layout, scrolling, tabs, quota views, or graphs.
- Do not change provider routing, quota pooling, or Codex compatibility logic.
- Do not add permanent test files or check harnesses.
- Do not push.

## Acceptance

- Repeating Codex device-login completion for the same account hash replaces the
  stored OAuth token material and returns the same local credential ID.
- A relogin clears prior refresh failure metadata and re-enables a disabled
  OAuth credential.
- New-account device login still inserts a new credential.
- A matching orphaned provider-account row no longer blocks relogin.
- Management and TUI outputs remain metadata-only and do not expose account IDs
  or token material.
- A one-off disposable Go smoke program under the repo root exercises the
  duplicate relogin, disabled re-enable, refresh failure clearing, and orphaned
  provider-account cases, then is removed. Do not add permanent `_test.go`
  files, scripts, or check harnesses.
- `go test ./...` passes as a compile/package check.
- `go vet ./...` passes.
- A fresh binary builds.
- Direct short-lived `ilonasin serve` and `ilonasin manage` smokes run against a
  disposable home.

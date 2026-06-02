# 316 OAuth Action Account Sanitizer

## Context

`docs/ilonasin-architecture.md` requires management snapshots and TUI-visible
metadata to avoid exposing raw account identifiers, request IDs, tokens, bodies,
prompts, completions, tool data, or raw provider payload markers. The
management snapshot sanitizer already treats account display labels with
`safeAccountDisplayString`, which intentionally permits normal email-like
labels while redacting account-ID and secret-shaped values.

One remaining management action boundary differs from that policy:
`oauthCredentialFromCredentials` builds OAuth action responses from credential
metadata, but sanitizes `AccountDisplayLabel` with generic
`safeSnapshotString`. Snapshot responses later sanitize the same DTO field with
`safeAccountDisplayString`. This makes OAuth login/complete/refresh responses
and snapshot responses diverge at the same management boundary.

Plan review also found that `safeAccountDisplayString` currently redacts
`acct_`-style account markers, but not explicit `account_id` or `account-id`
markers. That makes the account-display policy weaker than the architecture
requires for full upstream account IDs.

## Plan

1. Change `oauthCredentialFromCredentials` to use
   `safeAccountDisplayString` for `AccountDisplayLabel`.
2. Tighten the account-display unsafe pattern in management and matching TUI
   display sanitizers so explicit account-ID markers redact. Cover
   `account_id`, `account-id`, `account id`, `account.id`, and `account:id`.
   Do not reintroduce broad generic `account` redaction, because safe fallback
   labels such as `Codex account` and ordinary words such as `accounting` must
   remain visible.
3. Keep `ProviderInstanceID`, `Label`, `PlanLabel`, `Scopes`, refresh failure
   class, and refresh failure description sanitization unchanged.
4. Verify no storage, provider, server route, config, logging, OAuth
   refresh, or SQLite behavior changes are introduced.
5. Review the diff for privacy drift, account-label over-redaction,
   under-redaction, and consistency with snapshot/subscription account display
   sanitization.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/management
go test ./...
go vet ./...
```

Run a temporary focused management smoke, then remove it before commit. It must
prove:

- safe email-like account display labels survive
  `oauthCredentialFromCredentials`;
- safe non-email fallback account labels such as `Codex account` survive;
- ordinary words such as `accounting` survive;
- unsafe `acct_`, `account_id`, `account-id`, `account id`, `account.id`, and
  `account:id` shaped labels redact in management action responses, snapshots,
  subscription usage, and TUI display sanitizers;
- generic secret/body/prompt/request-marker labels still redact;
- refresh failure class and description behavior remains unchanged.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME` and isolated config, checking management health
over the Unix socket, running `ilonasin manage` under bounded narrow and wide
terminals, and cleaning up the daemon and temporary directory.

## Acceptance

- OAuth action responses and management snapshot responses use the same account
  display sanitizer for OAuth account labels.
- Safe email-like labels remain visible.
- Secret/account/request/body/prompt-shaped labels remain redacted.
- No management API shape, storage schema, provider behavior, TUI behavior, or
  logging policy changes are introduced.

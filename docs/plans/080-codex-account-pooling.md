# 080 Codex Account Pooling

## Context

Codex OAuth serving currently resolves one bearer credential for a provider
instance. API-key providers already have same-provider, same-model fallback
across enabled credentials when the fallback group is explicitly enabled.

The target architecture allows subscription account fallback only when it is
explicit, auditable, and not used for hidden quota or payment evasion.
`docs/codex-auth.md` also says Codex itself does not switch accounts on 429.
It only recovers auth on 401 and otherwise uses retry/backoff behavior.

The current worktree also contains relevant uncommitted Codex routing-header
work that must be audited before it is kept:

- attach `ChatGPT-Account-ID` and `X-OpenAI-Fedramp` to Codex upstream calls,
- use Codex-shaped `/models` responses and the Codex client version query.

This slice should absorb and verify that work because account pooling is not
correct unless each request uses the intended account-bound credential. Codex
persists account ID separately, but ilonasin will not add separate full account
ID storage in this slice. Any `ChatGPT-Account-ID` header is therefore
best-effort from the access token only.

## Goal

Add explicit Codex OAuth account pooling for availability fallback while
preserving the architecture's credential boundaries and the no-quota-cycling
rule.

After this slice, a Codex provider instance with multiple enabled OAuth
credentials in an enabled fallback group can retry the same provider/model on a
different OAuth account for retryable availability failures. It must not switch
accounts on 401 after refresh fails, on 429, on invalid provider bodies, after
a stream has started, or for any cross-provider or cross-model case.
Because this is subscription-style account fallback, it must also be gated by
an explicit Codex provider policy flag in local configuration. Enabling a
fallback group alone is not sufficient.

## Scope

1. Reconcile the current Codex request/header work before pooling.
   - Preserve the uncommitted Codex account-routing header work only after it
     is made compatible with `serve --check`.
   - The first implementation step is to make the current Codex chat/model
     smoke pass without pooling changes, so account pooling is not built on a
     failing baseline.
   - Codex client version constants must match a documented source. Either use
     the version in `docs/codex-auth.md` or update the docs with the newer
     hard source before using a newer version in code.
   - Codex account-routing headers must also match a documented source. The
     reviewed source shows Codex uses `TokenData.account_id`, not access-token
     decoding, as the durable account source.
   - Because ilonasin will not store a separate full account ID, this slice may
     attach `ChatGPT-Account-ID` only when it can be derived transiently from
     the access token. The account-bound bearer token still identifies the
     account to upstream when that claim is absent.
2. Add a pooled Codex OAuth bearer resolver.
   - Add `ResolveOAuthBearers(ctx, providerInstanceID, now)` to the OAuth
     serving boundary.
   - Add a repository method returning all non-disabled OAuth bearer candidate
     rows for a provider instance, including fallback group, expiry metadata,
     and nullable access-token secret ID. This method must not filter out
     expired rows or rows with missing access secrets before the primary row is
     identified.
   - Candidate identity is exactly:
     - provider credential kind is `oauth`,
     - credential is not disabled,
     - provider instance is configured, OAuth-capable, and `type = "codex"`.
   - Selection order is deterministic by `provider_credentials.id ASC`.
   - The first non-disabled Codex OAuth credential is the primary account,
     even when expired or missing an access-token secret.
   - If the primary credential is expired, the server/service layer must
     attempt same-credential refresh before returning a pool. If refresh fails,
     fail auth without selecting the next account.
   - If the primary credential is missing an access-token secret, fail auth
     without selecting the next account.
   - Only after the primary credential has a valid access token may the
     resolver include additional accounts.
   - If the primary credential's fallback group is disabled, return only that
     credential.
   - If the primary credential's fallback group is enabled, return only
     credentials in that same group whose access-token secret exists and whose
     expiry is absent or later than `now`.
   - Never join, select, return, log, or expose refresh-token material in this
     resolver.
3. Reuse the fallback-policy table for Codex OAuth account groups without
   mixing credential domains.
   - `ListFallbackPolicies` should include API-key groups and OAuth groups,
     but each row must have a credential kind/domain.
   - Policy resolution and counts must filter by credential kind. API-key and
     OAuth credentials must never be counted together.
   - Add credential kind to the durable fallback policy identity. Migrate
     `credential_fallback_policies` from unique `(provider_instance_id,
     group_label)` to unique `(provider_instance_id, credential_kind,
     group_label)`, backfilling existing rows as `api_key`.
   - Update mutation interfaces to identify provider instance, credential kind,
     and group label. API-key and OAuth fallback groups must not share a policy
     row.
   - The TUI snapshot should show Codex OAuth fallback groups with at least two
     enabled credentials.
   - The service mutation boundary must allow toggling Codex OAuth fallback
     groups. The existing direct TUI mutation path may remain legacy, but it
     must not be API-key-only after this slice.
   - Add a separate provider-policy/user-config gate for Codex OAuth account
     pooling. The OAuth resolver must require both the enabled OAuth fallback
     group and this Codex account-pooling permission before returning more than
     the primary credential.
4. Use the OAuth bearer pool in Codex serving.
   - Keep API-key providers on the existing API-key credential pool.
   - For OAuth providers, build a `[]provider.BearerCredential` pool.
   - Share non-streaming and streaming fallback loops across API-key and OAuth
     bearer credentials where practical.
   - Preserve existing same-credential 401 refresh behavior.
   - If a credential returns 401, refresh and retry that same credential once.
     If it still fails, do not switch accounts for that auth failure.
   - Switch to the next OAuth account only for retryable availability failures:
     network errors, timeouts, and retryable `5xx` before streaming starts.
   - Do not switch accounts for 429 or `rate_limit_exceeded`.
   - Treat Codex SSE `response.failed` with code `rate_limit_exceeded` as
     nonretryable for account fallback, even if it is normalized to a local
     upstream error.
   - Do not switch accounts after a stream has emitted bytes.
   - Do not pool model discovery in this slice. `/models` must use only the
     primary Codex OAuth credential so account-specific model visibility is not
     cached as provider-instance-wide metadata from a secondary account.
5. Finish Codex account-routing headers as best-effort transient derivation.
   - Keep the full account ID out of logs, metadata, snapshots, CLI/TUI output,
     request metadata, and fallback metadata.
   - Do not add separate full account ID storage.
   - Attach `ChatGPT-Account-ID` and `X-OpenAI-Fedramp` only when those values
     can be derived transiently from the access token.
   - Treat access-token JWT parsing as best-effort compatibility. Fake JWT
     smoke data must not be the sole evidence for claiming exact Codex parity.
   - Do not attach those headers to DeepSeek or OpenRouter.
   - Keep the Codex originator, user agent, and client version constants in the
     provider boundary.
6. Preserve metadata-only observability.
   - Record final credential ID, retry count, fallback count, and
     `availability_retry`.
   - Record fallback events from one local credential ID to another local
     credential ID.
   - Do not store prompts, completions, request bodies, response bodies, raw
     provider payloads, raw SSE chunks, tool arguments, tool results, full
     bearer tokens, full provider request IDs, full account IDs, balances, or
     credits.
7. Extend smoke coverage.
   - Seed at least two Codex OAuth credentials in the isolated serve-check DB.
   - Enable their fallback group explicitly.
   - Make the fake Codex upstream fail the first account with a retryable
     availability error and succeed on the second account for non-streaming
     chat.
   - Add the equivalent pre-stream fallback check for streaming chat.
   - Add a post-first-byte stream failure check proving the second account is
     not called.
   - Assert the fake upstream saw the expected account-bound access token for
     each account.
   - Assert account-routing headers are present only for access tokens whose
     payload carries the routing claim.
   - Assert full account IDs are allowed only in process memory and outbound
     Codex headers. They must not be rendered, logged, written to metadata, or
     stored outside existing hashed account metadata.
   - Assert refresh tokens and ineligible credential markers are not seen
     upstream, not rendered, not stored outside allowed secret rows, and not
     logged.
   - Assert Codex HTTP 429 and Codex streaming `rate_limit_exceeded` do not use
     the next account and do not record an account fallback.
   - Assert expired primary-account refresh success uses the same account and
     does not call the second account.
   - Assert expired primary-account refresh failure does not call the second
     account and records no fallback event.
   - Assert refreshed-token-still-401 does not call the second account and
     records no fallback event.
   - Assert disabled credentials are excluded from the pool.
   - Assert an expired or missing-access-secret primary credential fails auth
     without silently selecting a later account unless same-credential refresh
     succeeds.
   - Assert DeepSeek and OpenRouter still never receive OAuth credentials or
     Codex account-routing headers.
   - Assert `manage --check` renders the Codex fallback policy only as safe
     metadata.
   - Assert model discovery uses only the primary Codex OAuth credential.
   - Assert enabling the OAuth fallback group without the Codex account-pooling
     policy gate still returns only the primary credential.

## Non-Goals

- Do not implement quota pooling, rate-limit account cycling, or automatic 429
  account switching.
- Do not add cross-provider or cross-model fallback.
- Do not add browser OAuth login.
- Do not import Codex local `auth.json`, keyring state, cookies, or external
  Codex credential files.
- Do not store full account IDs separately.
- Do not add permanent tests.
- Do not migrate the remaining non-token TUI mutation paths to daemon
  management endpoints in this slice, except as needed to pass the new
  credential-kind and Codex account-pooling policy inputs through the existing
  mutation boundary.
- Do not push.

## Design Constraints

1. Account pooling must be opt-in through kind-aware fallback-policy state and
   an explicit Codex account-pooling provider-policy/user-config gate.
2. Pooling is same provider instance and same provider model only.
3. 429 is not retryable for account fallback.
4. 401 recovery stays same-credential refresh, not account fallback.
5. Streaming fallback is allowed only before the first byte is written.
6. Pooling is Codex-only in this slice. Other OAuth-capable provider types need
   their own provider policy before account fallback can be enabled.
7. Provider adapters must not import SQLite, management, TUI, or storage types.
8. Server code receives resolver interfaces and typed bearer credentials, not
   storage services.
9. Full account IDs may exist only in process memory while deriving outbound
   Codex headers. They must not be logged, rendered, exposed in snapshots,
   stored separately, or written to normal metadata tables.
10. All logs and errors must remain marker-safe.
11. The plan must work with the dirty worktree by staging only files belonging
    to this slice.

## Implementation Plan

1. Stabilize the current dirty Codex baseline.
   - Fix or adjust the uncommitted Codex header/request changes until
     `serve --check` passes before adding pooling.
   - Reconcile Codex client version evidence with `docs/codex-auth.md`.
   - Reconcile Codex account-routing header evidence with `docs/codex-auth.md`.
     Keep access-token claim parsing only as best-effort header derivation, not
     as exact Codex durable account state.
2. Extend credential interfaces and storage.
   - Add pooled OAuth resolver methods to `credentials`.
   - Implement `Store.ResolveOAuthBearerCredentials`.
   - Return all non-disabled OAuth credential rows before eligibility filtering
     so primary expiry and missing-access-secret cases cannot skip to a later
     credential.
   - Extend `ResolvedOAuthBearerCredential` with fallback group metadata, or
     use an internal resolver DTO that carries the group through pool
     selection before conversion to provider credentials.
   - Factor common expiry and routing-claim mapping helpers so single and
     pooled resolvers stay consistent.
3. Include OAuth groups in fallback policy metadata.
   - Add a migration that introduces `credential_kind` to
     `credential_fallback_policies`, backfills existing rows to `api_key`, and
     enforces uniqueness across `(provider_instance_id, credential_kind,
     group_label)`.
   - Change `EnableFallbackGroup` and `DisableFallbackGroup` to accept a
     credential kind or add kind-specific methods, then thread that through
     direct TUI and daemon snapshot mutation paths.
   - Update the SQLite fallback-policy query to count enabled credentials by
     kind, without mixing API-key and OAuth domains.
   - Update fallback policy mutation validation to permit Codex OAuth provider
     instances and to reject unsupported provider/kind combinations.
   - Update management snapshot and TUI visibility filters so Codex OAuth
     fallback groups can render.
   - Add local config for explicit Codex OAuth account pooling permission and
     require it before an OAuth pool can contain secondary credentials.
4. Refactor server credential execution.
   - Convert API-key resolved credentials to `provider.BearerCredential`.
   - Use one non-streaming fallback loop for bearer credentials.
   - Use one streaming fallback loop for bearer credentials.
   - Keep existing single-credential 401 refresh semantics inside the OAuth
     path.
5. Finish Codex provider header work.
   - Keep `internal/provider/codex_headers.go`.
   - Ensure Codex model discovery and chat attach documented Codex-specific
     non-secret headers.
   - Attach account headers only when routing claims are present in the access
     token.
   - Ensure fake upstream checks account headers only when supported, without
     logging account IDs.
6. Add serve-check exercises.
   - Add safe fake access JWT helpers for account-routing claims.
   - Add non-streaming and streaming Codex account fallback checks.
   - Add no-fallback-on-429 check.
   - Add no-fallback-on-streaming-rate-limit-event and post-first-byte failure
     checks.
   - Add 401 refresh success, refresh failure, and refreshed-token-still-401
     checks that prove no account fallback occurs.
   - Add checks that ineligible credentials are excluded.
7. Add manage-check coverage for the safe fallback policy summary.
   - Cover the new kind-aware fallback policy identity and Codex account-pool
     permission gate.
8. Do a code read-through before running commands.
9. Run:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
   - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Review Questions

1. Is using the existing fallback-policy table the right opt-in control for
   Codex OAuth account pooling?
2. Is it correct to forbid account switching for 429 and failed 401 refresh?
3. Does sharing the API-key and OAuth fallback loops reduce duplication without
   collapsing credential domains?
4. Is best-effort access-token routing acceptable for this slice, given exact
   Codex parity would require separate account ID persistence?

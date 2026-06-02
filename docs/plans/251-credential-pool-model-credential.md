# 251 Credential Pool Model Credential

## Goal

Keep Codex model-metadata auth aligned with the credential pool actually used
for a request.

`planCredentialAttempts` currently sets `modelCredential` to `credentials[0]`
before quota filtering. If that first credential is under an active quota block
and a later credential remains eligible, the request attempts use the later
credential but Codex model metadata can still be fetched with the blocked first
credential through `provider.ChatRequest.ModelCredential`.

That violates the architecture requirement that pooling remain constrained and
auditable for the requested provider instance/model, and it can make an
otherwise eligible fallback path fail before reaching inference.

## Scope

1. Update `internal/server/credential_pool.go` so `credentialAttemptPlan` picks
   `modelCredential` from the first actual planned attempt after quota
   filtering.
2. Preserve current behavior when no quota reader exists or no quota blocks are
   active.
3. Preserve exhausted-pool behavior when every credential is quota-blocked.
4. Preserve auth-refresh behavior in non-streaming and streaming execution:
   - refreshing the model credential still updates the current attempt when it
     is the same credential,
   - auth fallback can still move the model credential to the next attempt.
5. Do not change credential storage, quota storage, fallback policy storage,
   provider adapters, management API, TUI, config, or IO logging.
6. Do not add permanent tests.

## Verification

1. Temporary focused smoke, removed before commit:
   - with credentials `[1, 2]` and no quota reader, attempts are `[1, 2]` and
     model credential is `1`,
   - with credential `1` quota-blocked and credential `2` eligible, attempts
     are `[2]` and model credential is `2`,
   - with credential `1` quota-blocked and credentials `[2, 3]` eligible,
     attempts remain `[2, 3]` and model credential is `2`,
   - with all credentials quota-blocked, attempts are empty, exhausted is true,
     and the earliest active-until value is preserved as the local retry-after,
   - mixed retry-after, reset-at, and fallback-cooldown derived active-until
     values still choose the earliest active-until for an exhausted pool,
   - non-streaming execution sends the same eligible credential ID as both
     `Credential` and `ModelCredential` to the adapter after filtering,
   - streaming execution sends the same eligible credential ID as both
     `Credential` and `ModelCredential` to the adapter after filtering,
   - non-streaming and streaming execution still refresh the post-filter model
     credential on model-discovery auth failure and retry with the refreshed
     same credential,
   - auth fallback still moves the model credential to the next planned attempt
     after quota filtering, for example `[1 blocked, 2, 3]` where attempt `2`
     returns auth-retry before response commitment and the next adapter call
     uses credential `3` as both `Credential` and `ModelCredential`,
   - request metadata records the actual attempted credential, not the
     pre-filtered quota-blocked credential,
   - quota pre-filtering does not emit a fallback event by itself,
   - exhausted-pool route behavior still performs no adapter dispatch and
     records HTTP `429`, `upstream_quota_pool_exhausted`, `attempt_count=0`,
     `credential_id=0`, and the existing retry-after response behavior.
2. Standard checks:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
3. Temporary daemon smoke:
   - build `ilonasin`,
   - start `ilonasin serve` with a temporary home and config,
   - create a temporary local client token through the management API,
   - smoke management health,
   - smoke `ilonasin manage` under a short PTY timeout.
4. Remove all temporary smoke artifacts.

## Acceptance

- A quota-blocked leading credential cannot remain as the Codex model metadata
  credential when another credential is chosen for the actual attempt.
- Existing exhausted-pool and auth-refresh paths remain intact.

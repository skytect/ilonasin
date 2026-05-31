# 097 Default Quota Pooling

## Context

Plans 080 and 081 deliberately separated availability account fallback from
quota observability. The current product direction changes that policy:
same-provider, same-model credential pooling should be default behavior for
both API-key and OAuth credentials, and quota pressure should participate in
pool selection.

This slice turns the existing observability foundation into routing behavior.
It also updates the architecture docs because they currently describe quota
cycling as not allowed by default.

## Goal

Make credential pooling default across API keys and Codex OAuth accounts for a
single requested provider instance and provider model.

After this slice, when a credential is unavailable or quota-blocked, the daemon
can try another eligible credential of the same provider instance for the same
model without requiring `config.toml` gates or TUI-enabled fallback policies.
The routing remains metadata-only, auditable, and local. It must not cross
provider instances, cross models, store raw payloads, store full account IDs, or
record sensitive provider details.

## Scope

1. Update architecture and plan references.
   - Change `docs/ilonasin-architecture.md` fallback language from opt-in
     account fallback to default same-provider credential pooling.
   - Keep the hard constraints: no cross-provider fallback, no cross-model
     fallback, no hidden model downgrade, no prompt/body/payload persistence.
   - Document that quota pooling is a local credential-selection mechanism,
     not provider balance scraping or billing inference.
2. Make API-key pooling default.
   - Return all enabled API-key credentials for the requested provider instance
     by default.
   - Do not require an enabled fallback-policy row before a second API key is
     eligible.
   - Ignore legacy `fallback_group` boundaries for serving eligibility. The
     group remains display/operator metadata only in this slice.
   - Keep deterministic credential order, with the existing primary credential
     first unless active quota state moves it behind available credentials.
   - Keep fallback-policy storage and TUI rows as legacy/operator metadata for
     now, but stop using them as the serving gate.
3. Make Codex OAuth account pooling default.
   - Stop requiring `codex_account_pooling = true` in `config.toml`.
   - Stop requiring an enabled OAuth fallback-policy row before additional
     Codex OAuth credentials are eligible.
   - Ignore legacy OAuth `fallback_group` boundaries for serving eligibility.
   - Keep same-credential 401 refresh behavior. Auth failures must not become
     blind account switching.
   - Treat locally missing or expired primary OAuth access tokens as
     refreshable local auth state, not upstream quota state. If primary refresh
     fails, do not silently choose a secondary account for that auth failure.
   - Keep `/models` on the primary credential unless a specific model-route
     smoke proves safe to pool model discovery later.
4. Add quota-aware credential selection.
   - Add a narrow server read interface for active quota state. The server must
     not import SQLite types.
   - Add `metadata.ActiveQuotaBlock` and a storage method like
     `ActiveQuotaBlocks(ctx, providerInstanceID, modelID, now)` for routing.
     Do not reuse display-oriented `QuotaByProvider` for routing.
   - Use `quota_events` as the local source for recent quota blocks.
   - Treat a credential as quota-blocked for a provider/model when a quota
     observation has a future `retry_after` or `reset_at`.
   - For quota observations with no explicit retry/reset time, apply an
     in-planner 10 minute cooldown from `observed_at`. This fabricated cooldown
     must not be rendered as a provider reset time.
   - Use latest-event-wins per provider/model/credential. A later quota event
     replaces older quota timing for routing. Because successes are not quota
     rows, null retry/reset events expire after the 10 minute local cooldown.
   - Treat `insufficient_quota` with no retry/reset as a local 10 minute block,
     not a permanent hard block.
   - Order available candidates by stable credential ID order.
   - Do not call credentials with active local quota blocks while any eligible
     unblocked credential exists.
   - If all credentials are blocked, return a local upstream quota error with a
     safe retry hint rather than cycling indefinitely. Record request metadata
     with no final credential ID, HTTP `429`, and error class
     `upstream_quota_pool_exhausted`.
   - Keep the Codex model-discovery credential separate from the planner's
     attempt order. `ModelCredential` must remain the primary credential even
     when quota ordering starts chat on a secondary credential.
5. Retry quota failures before a response is committed.
   - For non-streaming chat, retry a same-provider/model request on the next
     eligible credential after HTTP `429`, HTTP `402`, `rate_limit_exceeded`,
     or `insufficient_quota`.
   - For streaming chat, retry quota failures only before any bytes have been
     written to the client.
   - Do not retry after a stream has started.
   - Preserve existing availability retry behavior for network failures,
     timeouts, and retryable `5xx`.
   - Add separate quota retry predicates. Do not broaden
     `retryableChatAttempt` or `retryableStreamAttempt`, because those helpers
     also drive final local error mapping.
   - Use distinct fallback reasons such as `availability_retry` and
     `quota_retry`.
   - Set `fallback_events.allowed_by_policy = true` to mean allowed by the
     built-in default same-provider credential pooling policy, not by a
     user-enabled fallback-policy row.
6. Record per-attempt quota metadata.
   - Record quota observations for every quota-limited attempt, not only the
     final attempt.
   - Buffer failed-attempt quota observations in memory and write them only
     after the final `request_metadata` row exists, so every quota event can be
     linked to the request metadata ID.
   - Use only safe scalar fields already allowed by the metadata policy:
     provider instance, provider model, local credential ID/label, status,
     normalized error class, retry/reset time, source, and counts.
   - Do not store prompts, completions, request bodies, response bodies, raw
     provider payloads, raw SSE chunks, tool arguments, tool results, full
     bearer tokens, full provider request IDs, full account IDs, balances, or
     credits.
7. Surface pool state in management/TUI.
   - Extend read interfaces as needed so the daemon can query active quota
     blocks by provider/model/credential.
   - Keep `manage` as a client of daemon snapshot data for new display work.
   - Keep routing reads label-free. The planner should use credential IDs only.
   - Show quota-blocked credentials and retry/reset hints using local
     credential IDs and safe labels only after management snapshot
     sanitization.
   - Rename or relabel TUI fallback controls so they are not presented as the
     serving eligibility gate after this slice.
   - Do not add new direct `config.toml` mutation paths.
8. Extend smoke checks without permanent tests.
   - API-key non-stream smoke: first key returns quota, second key succeeds.
   - API-key stream pre-start smoke: first key returns quota before bytes,
     second key succeeds.
   - Stream post-start smoke: quota failure after first byte does not retry.
   - Codex OAuth smoke: second account is eligible by default without
     `codex_account_pooling` or fallback-policy setup.
   - Codex model credential smoke: quota-reordered chat attempts still use the
     primary credential for model discovery.
   - Responses smoke: `/v1/responses` follows the same quota-pooling behavior
     as non-streaming chat.
   - All-known-blocked smoke: daemon returns a safe local quota error and does
     not call upstream repeatedly.
   - Existing negative quota-fallback assertions from prior plans are updated
     to assert the new default quota-pooling behavior.
   - Manage smoke: fallback-policy toggles are checked as UI/metadata state,
     not as serving resolver gates.
   - Metadata smoke: quota events and fallback events contain only safe fields.
   - Privacy scan: logs, SQLite metadata, snapshots, and CLI output must not
     contain raw bodies, bearer tokens, full account IDs, provider request IDs,
     balances, or credits.

## Non-Goals

- Do not implement cross-provider fallback.
- Do not implement cross-model fallback.
- Do not query provider billing, balances, credits, plan limits, or account
  settings.
- Do not estimate remaining quota from pricing tables.
- Do not add permanent test files.
- Do not migrate unrelated TUI mutation paths.
- Do not push.

## Design Constraints

1. Pooling is same configured provider instance and same provider model only.
2. API-key and OAuth pools must not mix credential kinds.
3. Auth failures stay auth failures unless same-credential refresh succeeds.
4. Streaming retry is allowed only before response bytes are committed.
5. Candidate ordering must be deterministic and inspectable.
6. Quota state is inferred only from local safe metadata already produced by
   requests routed through ilonasin.
7. Storage must not import provider, server, TUI, config, or HTTP packages.
8. Provider adapters must not import SQLite, management, TUI, or storage types.
9. Full account IDs may exist only transiently for outbound Codex routing
   headers. They must never be logged, stored, rendered, or written to normal
   metadata.
10. This slice must work with the existing dirty `internal/storage/sqlite/db.go`
    timestamp parsing change without reverting it.

## Implementation Plan

1. Document the new default pooling policy.
   - Update architecture fallback text.
   - Update stale plan or audit notes only where they would mislead future
     work.
2. Add a credential pool planner.
   - Introduce a small server-side planner that accepts candidate credentials
     plus active quota observations and returns an ordered attempt list.
   - Keep the planner independent from SQLite and provider adapters.
   - Return a typed exhausted-pool result when all candidates are quota-blocked.
   - Return the preserved primary/model credential separately from the ordered
     attempt list.
3. Loosen serving credential resolvers.
   - API-key resolver returns all enabled credentials for a provider instance.
   - Codex OAuth resolver returns all eligible enabled bearer credentials by
     default after primary same-credential auth material is valid.
   - Remove serving dependence on `CodexAccountPooling` and fallback-policy
     enabled flags.
   - Remove serving dependence on `fallback_group` boundaries.
4. Add active quota reads.
   - Add a read method that returns active quota blocks by provider instance,
     model, credential ID, retry-after, reset-at, and observed time.
   - Reuse existing `quota_events`; add a migration only if an index is needed.
5. Wire quota-aware retries.
   - Apply the planner to non-streaming and streaming chat attempts.
   - Record `quota_retry` fallback events when moving to another credential for
     quota reasons.
   - Record quota observations for all quota-limited attempts.
   - Preserve current availability retry semantics.
6. Update management/TUI display.
   - Add active quota/pool state to the management snapshot or extend existing
     quota rows enough for the TUI to render it.
   - Keep display fields safe and compact.
7. Strengthen smoke checks.
   - Add focused fake-upstream checks to `serve --check`.
   - Add snapshot/TUI rendering checks to `manage --check`.
   - Add privacy markers for raw provider bodies, tokens, full account IDs,
     balances, credits, and provider request IDs.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Acceptance:

- no permanent tests exist,
- compile/package, vet, build, `serve --check`, `manage --check`, and
  whitespace checks pass,
- API-key and Codex OAuth credential pooling are default serving behavior,
- quota failures can move to another eligible credential before response
  commit,
- all-known-blocked pools fail locally with a safe quota error,
- usage, quota, fallback, and health metadata remain safe scalar metadata,
- no raw request/response/provider payloads, tokens, full account IDs, provider
  request IDs, balances, or credits leak to logs, SQLite metadata, snapshots, or
  CLI/TUI output.

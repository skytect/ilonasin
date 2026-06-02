# 339 No-Affinity Pool Rotation

## Context

Plans 325 through 327 added deterministic credential ordering, request affinity,
and in-flight pressure tracking. The current implementation preserves
cache/session stickiness when a request supplies an affinity key, and uses
least-in-flight selection when affinity is empty.

The remaining gap is ordinary no-affinity traffic. When no requests are
currently in flight, every credential has pressure count zero. `reserveLeast`
then selects the first candidate in the deterministic ring. For one local API
token, one provider, and one model, repeated generic requests with no affinity
can still keep starting on the same upstream account.

Recent bounded local captures on 2026-06-03 showed:

- Codex CLI 0.135.0 sends Responses `prompt_cache_key` in the JSON body, plus
  `session-id`, `x-client-request-id`, and `x-codex-window-id` headers.
- Claude Code 2.1.159 sends Anthropic `metadata.user_id` as a JSON string
  containing `session_id`, which ilonasin already extracts.
- Generic OpenAI Chat clients may send only `model` and `messages`; `user`,
  `session_id`, and metadata are optional, not out-of-box signals.

This slice improves the generic no-affinity case without changing explicit
affinity behavior.

## Scope

1. Add daemon-local round-robin tie-breaking inside the existing
   `credentialPressureTracker`.
   - Key the tie-breaker by resolved provider instance ID and provider model ID.
   - Use credential ID only as a candidate identity, never bearer material or
     upstream account IDs.
   - Store the cursor as the next local credential ID, not as a transient slice
     index, so quota-filtered lists, retry-filtered lists, and token-dependent
     candidate ordering do not make the cursor point at unrelated credentials.
   - If the cursor credential is present in the current slot list but is not a
     lowest-pressure candidate, choose the first lowest-pressure candidate at or
     after that cursor position.
   - If the cursor credential is absent from the current slot list, fall back to
     the first lowest-pressure candidate in the current stable order.
   - Keep all state in memory only.
2. Apply the tie-breaker only when request affinity is empty.
   - Non-empty affinity must keep the current sticky first-slot behavior.
   - No-affinity reservation should still prefer the lowest in-flight count.
   - When multiple candidates share the lowest in-flight count, rotate among
     those candidates rather than always choosing the first candidate.
3. Preserve existing deterministic credential ordering as the stable candidate
   order passed into the tracker.
   - The tracker can use that order to make tie rotation deterministic within a
     process.
   - Quota-filtered attempt lists must remain the candidate source, so blocked
     credentials are not selected.
4. Preserve retry and fallback semantics.
   - Each selected attempt is still acquired and released exactly once.
   - No-affinity retry attempts may advance the provider/model cursor because
     they reserve another upstream credential; this is acceptable because they
     consume real pool capacity and remain visible through fallback metadata.
   - Fallback events are still recorded only for actual retry movement.
   - Health and quota observations are unchanged.
   - `modelCredential` must still match the first actual selected attempt after
     quota filtering and no-affinity rotation, because providers may use it for
     model metadata resolution.
5. Do not change provider adapters, request parsing, storage, management API,
   TUI, config, metadata schema, or logging.

## Out Of Scope

- Persisted round-robin state across daemon restarts.
- Weighting by remaining subscription quota.
- Latency-based or success-rate-based routing.
- Header-derived affinity for Codex `session-id`.
- Cross-provider or cross-model fallback.
- Any change to how prompts, bodies, tool payloads, or account IDs are handled.

## Implementation Steps

1. Extend `credentialPressureTracker` with a small in-memory cursor map keyed by
   provider instance and provider model. Store cursor values as local
   credential IDs.
2. Update `reserveLeast` so it:
   - computes the lowest in-flight count across candidates;
   - keeps only candidates with that count;
   - chooses the first lowest-pressure candidate at or after the cursor;
   - advances the cursor to the next credential position after the chosen slot;
   - increments the selected credential's in-flight count under the same lock.
3. Keep nil tracker and explicit-affinity paths unchanged.
4. Review the diff for privacy, locking, and unchanged retry behavior before
   running checks.

## Verification

Use temporary focused checks, then remove them before commit:

- repeated no-affinity reservations over three credentials distribute across
  the pool when each iteration releases before the next reservation, proving
  zero-pressure tie rotation rather than pressure-only balancing;
- rotation is scoped by provider instance and provider model;
- when one credential has a higher in-flight count, the tracker chooses a
  lower-pressure credential regardless of cursor;
- release returns pressure counts to zero;
- explicit affinity still bypasses round-robin and selects the first candidate;
- quota-filtered candidate lists remain respected.
- a no-affinity retry after a failed first attempt selects an untried remaining
  credential, records fallback from the actual first selected credential to the
  actual second selected credential, and does not duplicate attempts;
- the first actual selected no-affinity credential remains the execution
  `modelCredential`.
- concurrent no-affinity reservations over equal idle candidates serialize
  under the tracker lock and spread initial selections rather than all choosing
  the first candidate.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting
`ilonasin serve` with a temporary `ILONASIN_HOME`, checking management health
over the Unix socket, running bounded `ilonasin manage`, and cleaning up all
temporary files and processes.

## Acceptance

- Generic no-affinity traffic no longer repeatedly starts from the same
  upstream credential when all candidates are idle.
- Explicit Codex/Claude/session/cache affinity remains sticky.
- Same-provider-instance and same-model pooling constraints remain unchanged.
- No new persistent identifiers, metadata fields, logs, or TUI surfaces are
  added.

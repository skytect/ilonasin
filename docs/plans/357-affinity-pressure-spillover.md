# 357 Affinity Pressure Spillover

## Context

`docs/ilonasin-architecture.md` requires credential pooling to stay constrained
to the requested provider instance, requested provider model, and eligible
credentials attached to that provider instance. It also allows switching to
another eligible credential on availability or quota pressure before a response
is committed.

Current pooling has three useful pieces:

- deterministic credential ring ordering from local token, provider, model, and
  optional request affinity;
- local-only affinity extraction from real client signals such as Responses
  `prompt_cache_key`, Chat `session_id`, Chat `user`, selected metadata, and
  Anthropic session metadata;
- no-affinity least-in-flight selection with token-scoped round-robin
  tie-breaking.

The remaining concurrency gap is the explicit-affinity path. If a Codex thread
or other session has a stable affinity key, `reserveCredentialAttempt` always
selects the first planned slot. That preserves cache locality, but concurrent
requests with the same affinity key can all pile onto one upstream account while
other same-provider/model credentials are idle. The user goal is to preserve
session/cache affinity where practical while still avoiding account-level
concurrency and speed limits.

## Goal

Make explicit-affinity requests pressure-aware without losing deterministic
cache affinity as the tie-breaker.

## Scope

1. Extend credential reservation so both affinity and no-affinity requests use
   the same in-flight pressure view.
   - Pressure remains keyed only by provider instance, provider model, and
     upstream credential ID.
   - The pressure tracker remains daemon-local and in-memory.
   - No affinity key, prompt, message, request body, response body, provider
     payload, bearer token, or full upstream account ID is stored, logged, or
     rendered.
2. Preserve deterministic affinity ordering as the tie-breaker.
   - For requests with non-empty affinity, the first planned credential still
     wins when all eligible candidates have equal pressure.
   - If the sticky credential has higher in-flight pressure than another
     eligible candidate, pick the lowest-pressure candidate instead.
   - If multiple lowest-pressure candidates exist, pick the earliest one in the
     current planned ring.
3. Preserve no-affinity behavior.
   - No-affinity requests still choose the lowest in-flight candidates first.
   - No-affinity equal-pressure ties still use the token-scoped round-robin
     cursor from plan 354.
4. Preserve quota filtering and retry behavior.
   - Reservation only sees the already planned, quota-filtered, untried slots.
   - Retry attempts still reserve from remaining untried credentials.
   - Exhausted-pool behavior and retry-after handling remain unchanged.
5. Preserve Codex model credential behavior.
   - The first actual attempted credential remains the `modelCredential` for
     Codex model metadata.
   - Auth-refresh behavior for model credentials and attempt credentials remains
     unchanged.
6. Do not change request parsing, provider adapters, storage, schema,
   management routes, TUI, config, metadata fields, logging, model discovery, or
   fallback policy rows.

## Out Of Scope

- Persisted routing state.
- Weighting by remaining subscription quota, latency, success rate, or cost.
- Cross-provider or cross-model fallback.
- Deriving affinity from prompt or message content.
- Exposing affinity or pressure state in the management API or TUI.
- Changing Codex upstream `prompt_cache_key` forwarding.

## Implementation Steps

1. Add a pressure-aware reservation helper for affinity requests, for example
   `reserveLeastStable`, that:
   - scans current slots for the lowest in-flight count;
   - chooses the first lowest-pressure slot in current order;
   - increments the chosen credential under the tracker lock;
   - returns a release function for the selected attempt.
2. Keep the existing no-affinity cursor helper for no-affinity tie-breaking.
3. Update `reserveCredentialAttempt`:
   - non-empty affinity uses the stable pressure-aware helper;
   - empty affinity keeps the token-cursor least-pressure path;
   - nil tracker fallback still selects the first slot and uses normal
     `trackCredentialAttempt`.
4. Review call sites in non-streaming and streaming execution to confirm first
   actual attempt still sets `modelCredential`.
5. Review the diff for privacy, same-provider/model constraints, and unchanged
   metadata/logging surfaces.

## Verification

Use temporary focused checks, then remove them before commit:

- explicit affinity with all zero pressure selects the first planned slot;
- explicit affinity with the first slot busy and a later slot idle selects the
  later idle slot;
- explicit affinity with multiple lowest-pressure candidates selects the first
  candidate in the affinity-ordered slot list;
- explicit affinity reservation increments pressure and release decrements it;
- no-affinity reservations still rotate using the token-scoped cursor when
  pressure is equal;
- no-affinity reservations still prefer lower-pressure candidates before cursor
  tie-breaking;
- retry-filtered slot lists preserve current-slot ordering and only select
  remaining untried credentials;
- first actual non-streaming and streaming attempts still become
  `modelCredential`.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage`, and cleaning up all temporary
files and processes.

## Acceptance

- Explicit request affinity no longer forces concurrent same-session traffic to
  a busy upstream credential when another same-provider/model credential is
  idle.
- Cache/session affinity remains the deterministic tie-breaker when pressure is
  equal.
- Generic no-affinity balancing behavior from plans 327, 339, 340, and 354 is
  unchanged.
- Quota filtering, retry/fallback metadata, health recording, provider
  behavior, storage, management, TUI, config, and logging are unchanged.

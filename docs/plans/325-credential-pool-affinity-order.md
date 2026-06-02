# 325 Credential Pool Affinity Order

## Context

`docs/ilonasin-architecture.md` requires same-provider-instance,
same-provider-model credential pooling to be default and auditable through local
metadata. Current serving already resolves a credential pool, filters active
quota blocks, and falls back across eligible credentials before a response is
committed.

The remaining issue is initial ordering. `planCredentialAttempts` preserves
resolver order, which is stable credential ID order for API keys and OAuth
accounts. That means the first eligible credential can dominate normal traffic
until it hits quota or availability pressure.

The user goal is to avoid putting every request on the same account while still
preserving cache affinity where practical. There is no explicit session ID in
the public routes today. The stable local signal available at routing time is
the verified ilonasin client token. This slice uses that token as the first
affinity boundary: different local API keys can land on different upstream
credentials for the same provider/model, while the same local API key keeps a
stable first credential for that provider/model.

This does not fully solve the common single-local-token case. It establishes the
serving policy boundary and removes resolver-order dominance across distinct
local clients. Later slices should add an explicit session-affinity signal or
adaptive load metric so one busy local token can still spread work without
discarding cache affinity.

## Scope

1. Add a small credential-pool ordering policy in `internal/server`.
   - Inputs: resolved provider instance/model, verified local token ID, and the
     currently eligible credential list.
   - Output: the same credentials rotated into a deterministic affinity order.
   - Use only local IDs and model/provider strings, not bearer tokens, upstream
     account IDs, prompts, request bodies, or provider payloads.
2. Apply affinity ordering to the full resolved credential pool, then filter
   active quota blocks while preserving that rotated order.
   - A blocked affinity credential must not remain first.
   - If the affinity credential is blocked, the deterministic ring should start
     from the next eligible credential.
   - If quota is unavailable, fails, or returns no active blocks, apply affinity
     ordering to the full resolved credential pool.
3. Keep `modelCredential` aligned with the first actual planned attempt after
   quota filtering and affinity rotation.
4. Apply the same policy to OpenAI Chat, Responses, and Anthropic Messages
   because they share `nonStreamContext` or `streamContext` and the same
   provider/model credential pool.
5. Preserve current retry behavior.
   - Availability, auth, and quota retries still continue through the planned
     order.
   - Existing fallback events still record only actual retry/fallback movement,
     not the initial affinity rotation.
6. Keep model discovery unchanged in this slice.
   - Model discovery has no client token or requested provider model.
7. Do not add config, management API fields, storage schema, provider adapter
   changes, TUI changes, or permanent tests.

## Out Of Scope

- Adaptive least-used or least-latency selection.
- Per-session headers or request-body session affinity.
- Persisted concurrency counters.
- Subscription remaining-quota weighting.
- Cross-provider or cross-model fallback.
- Changing fallback policy rows.

Those should be later slices once this stable ordering boundary exists.

## Implementation Steps

1. Extend `credentialAttemptPlan` planning to accept a local token ID.
2. Add a deterministic hash helper for `(token ID, provider instance, provider
   model)` and rotate credentials by `hash % len(credentials)`.
3. Update non-streaming and streaming execution call sites to pass the verified
   local token ID.
4. Keep exhausted-pool behavior unchanged when every credential is quota-blocked.
5. Review code for privacy and metadata-only behavior.

## Verification

Run a temporary focused smoke, then remove it before commit. It should prove:

- two credentials with different token IDs can produce different first attempts;
- deterministic helper inputs are chosen so the spread assertion is not
  probabilistic;
- the same token/provider/model produces the same first attempt repeatedly;
- changing provider model changes the affinity key;
- quota filtering preserves the original affinity ring order;
- when the affinity-selected credential is quota-blocked, the first planned
  attempt is an eligible credential and `modelCredential` matches it;
- all eligible credentials remain in the attempt plan exactly once;
- exhausted-pool behavior still returns no attempts and preserves retry-after;
- non-streaming execution sends the affinity-selected credential first;
- streaming execution sends the affinity-selected credential first;
- fallback events are not emitted merely because affinity rotated the initial
  order.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

The `find` output should be compared against the pre-existing permanent test
file list. This slice must not add a permanent test file.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temporary directory.

## Acceptance

- The first credential attempted for serving is no longer always resolver order.
- Different local ilonasin client tokens can spread across upstream credentials
  for the same provider/model.
- The same local ilonasin client token remains sticky for the same
  provider/model while credentials remain eligible.
- The single-local-token case is explicitly left for a follow-up adaptive or
  session-affinity slice.
- Quota-block filtering, retry/fallback recording, metadata privacy, and
  same-provider/model constraints remain unchanged.

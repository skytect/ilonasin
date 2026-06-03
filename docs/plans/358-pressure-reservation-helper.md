# 358 Pressure Reservation Helper

## Context

Plan 357 made explicit request affinity pressure-aware. That left
`internal/server/credential_pool.go` with duplicated reservation completion
logic in `reserveLeast` and `reserveLeastStable`:

- read the selected slot;
- build the provider/model/credential pressure key;
- increment `inFlight` under the pressure tracker mutex;
- return the original attempt index, credential, release callback, and success
  flag.

The behavior is correct, but this is now a small duplication in a hot routing
boundary. `docs/ilonasin-architecture.md` expects credential pooling to stay
modular, constrained to the requested provider/model, and auditable through
metadata-only surfaces. Removing the duplication makes the routing boundary
easier to maintain without changing selection policy.

## Goal

Extract the shared pressure reservation completion logic into one helper while
preserving all credential selection behavior exactly.

## Scope

1. Add a private helper on `credentialPressureTracker`, for example
   `reserveSlotLocked(addr, slots, slotPosition)`.
   - It must be called while `t.mu` is held.
   - It must return the selected attempt index, credential, release callback,
     and success flag.
   - It must increment `inFlight` only for non-zero credential IDs.
   - It must release only non-zero credential IDs.
2. Update `reserveLeast` to use the helper after the no-affinity cursor selects
   the slot position.
3. Update `reserveLeastStable` to use the helper after selecting the first
   lowest-pressure slot position.
4. Preserve behavior exactly:
   - explicit affinity still chooses the first lowest-pressure slot in current
     ring order;
   - no-affinity still chooses lowest pressure first and uses the token-scoped
     cursor for equal-pressure candidates;
   - nil tracker fallback still selects the first slot through
     `trackCredentialAttempt`;
   - quota-filtered retry slot handling is unchanged;
   - `modelCredential` call-site behavior is unchanged.
5. Do not change request parsing, provider adapters, storage, schema,
   management routes, TUI, config, metadata fields, logging, model discovery, or
   fallback policy rows.

## Out Of Scope

- Any routing policy change.
- Weighting by quota, latency, success rate, or cost.
- Persisted pressure or affinity state.
- Cross-provider or cross-model fallback.
- New management or TUI surfaces.
- Permanent tests.

## Implementation Steps

1. Add the locked reservation helper near the pressure tracker reservation
   methods.
2. Replace the duplicated reservation completion blocks in `reserveLeast` and
   `reserveLeastStable`.
3. Review the diff for lock ownership and release pairing.
4. Verify the change is behavior-preserving with temporary focused checks, then
   remove those checks before commit.

## Verification

Use temporary focused checks, then remove them before commit:

- no-affinity reservation still increments pressure and release decrements it;
- explicit affinity reservation still increments pressure and release
  decrements it;
- explicit affinity with equal pressure still chooses the first slot;
- explicit affinity with the first slot busy still spills to an idle later slot;
- no-affinity equal-pressure rotation still follows the token-scoped cursor.

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

- Pressure reservation completion logic has one implementation.
- Plan 357 affinity spillover behavior is unchanged.
- No-affinity token-cursor behavior is unchanged.
- Privacy, same-provider/model routing, quota filtering, fallback metadata,
  health recording, provider behavior, storage, management, TUI, config, and
  logging are unchanged.

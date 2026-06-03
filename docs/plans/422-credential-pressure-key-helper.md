# 422 Credential Pressure Key Helper

## Context

`internal/server/credential_pool.go` now implements credential pooling with:

- deterministic credential ordering from local token, provider instance,
  provider model, and optional request affinity;
- active quota filtering before attempts;
- in-memory in-flight pressure tracking;
- explicit-affinity pressure spillover with affinity order as the tie-breaker;
- no-affinity least-pressure selection with token-scoped cursor tie-breaking.

The pressure tracker constructs the same private key shape in several places:

- `credentialPressureTracker.acquire`;
- `credentialPressureTracker.reserveSlotLocked`;
- `credentialPressureTracker.inFlightCount`.

This is not behaviorally wrong, but it is duplicated boundary code in the core
pool selector. The architecture asks for auditable, same-provider/model
credential pooling. A single helper makes the scope of pressure accounting
harder to drift accidentally.

## Goal

Consolidate credential pressure key construction behind one private helper
without changing credential ordering, pressure counts, affinity behavior, quota
filtering, retry behavior, metadata, storage, logging, management, TUI, config,
or provider behavior.

## Scope

1. Add a private helper in `internal/server/credential_pool.go`, for example
   `credentialPressureKeyFor(addr, credential)`.
2. Use that helper anywhere the pressure tracker builds a
   `credentialPressureKey`.
3. Keep the key fields exactly the same:
   - provider instance ID;
   - provider model ID;
   - upstream credential ID.
4. Keep zero-credential behavior unchanged.
   - `acquire` still returns a no-op release for credential ID `0`.
   - `reserveSlotLocked` still increments only when credential ID is non-zero.
   - `inFlightCount` still returns zero for credential ID `0`.
5. Do not change `credentialPressureScope`, token cursor behavior, affinity
   hashing, quota planning, retry/fallback metadata, route handling, request
   parsing, storage, management routes, TUI, config, logging, or provider
   adapters.
6. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- acquiring a non-zero credential increments the same provider/model/credential
  key and release decrements it;
- reserving a non-zero slot increments the same key and release decrements it;
- `inFlightCount` remains scoped by provider instance and provider model;
- zero credential IDs still do not create pressure state;
- explicit-affinity equal-pressure selection still picks the first planned
  slot;
- no-affinity equal-pressure selection still uses the token-scoped cursor.

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
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Pressure key construction has one implementation point.
- Credential pressure remains scoped only to provider instance, provider model,
  and upstream credential ID.
- Affinity, no-affinity, quota filtering, retry/fallback behavior, metadata,
  storage, logging, management, TUI, config, and providers are unchanged.

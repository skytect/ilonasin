# 423 Credential Pressure Scope Helper

## Context

`internal/server/credential_pool.go` uses two private key shapes for credential
pooling:

- `credentialPressureKey`, keyed by provider instance, provider model, and
  upstream credential ID for in-flight counts;
- `credentialPressureScope`, keyed by provider instance, provider model, and
  local token ID for no-affinity cursor tie-breaking.

Slice 422 consolidated `credentialPressureKey` construction behind
`credentialPressureKeyFor`. The cursor scope still constructs the same
provider/model/token boundary inline in `reserveLeastCandidate`.

The architecture requires pooling to stay scoped to the requested provider
instance, requested provider model, and verified local token identity when no
client affinity exists. A helper gives the cursor scope one implementation
point, matching the pressure key boundary.

## Goal

Consolidate no-affinity cursor scope construction behind one private helper
without changing credential ordering, pressure selection, affinity behavior,
quota filtering, retry behavior, metadata, storage, logging, management, TUI,
config, or provider behavior.

## Scope

1. Add a private helper in `internal/server/credential_pool.go`, for example
   `credentialPressureScopeFor(addr, tokenID)`.
2. Use that helper where the no-affinity cursor map is keyed.
3. Keep the scope fields exactly the same:
   - provider instance ID;
   - provider model ID;
   - verified local token ID.
4. Preserve token cursor behavior exactly.
   - Same local token, provider instance, and provider model share a cursor.
   - Different local tokens do not share a cursor.
   - Different provider instances or provider models do not share a cursor.
   - Explicit-affinity requests still do not use the no-affinity cursor path.
5. Do not change `credentialPressureKey`, affinity hashing, in-flight pressure
   counting, quota planning, retry/fallback metadata, route handling, request
   parsing, storage, management routes, TUI, config, logging, or provider
   adapters.
6. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- no-affinity equal-pressure selection rotates for the same
  provider/model/token scope;
- a different token has an independent cursor;
- a different provider instance has an independent cursor;
- a different provider model has an independent cursor;
- explicit-affinity selection remains independent of the no-affinity cursor;
- pressure preference still beats cursor tie-breaking for no-affinity requests.

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

- No-affinity cursor scope construction has one implementation point.
- Cursor scope remains provider instance plus provider model plus local token.
- Affinity, no-affinity, quota filtering, retry/fallback behavior, metadata,
  storage, logging, management, TUI, config, and providers are unchanged.

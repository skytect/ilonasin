# 424 Credential Affinity Hash Helper

## Context

`internal/server/credential_pool.go` uses `credentialAffinityStart` to rotate
eligible same-provider/model credentials. Its hash input is intentionally
limited to:

- a version prefix;
- verified local token ID;
- requested provider instance ID;
- requested provider model ID;
- optional safe request affinity.

That matches `docs/ilonasin-architecture.md`: pooling should fall back to local
inputs the daemon always has, and must not derive affinity from prompts,
messages, request bodies, bearer tokens, upstream account IDs, device IDs,
installation IDs, or request IDs.

The current hash-feed code is small but embedded directly inside
`credentialAffinityStart`. Extracting it behind a private helper makes the
allowed inputs explicit and keeps the rotation function focused on indexing.

## Goal

Move credential-affinity hash input writing into one private helper without
changing the hash version, hash algorithm, field order, delimiter bytes,
trimming behavior, rotation output, pressure selection, quota filtering, retry
behavior, metadata, storage, logging, management, TUI, config, or provider
behavior.

## Scope

1. Add a private helper in `internal/server/credential_pool.go`, for example
   `writeCredentialAffinityHash(h hash.Hash64, addr routing.ModelAddress,
   tokenID int64, affinityKey string)`.
2. Keep `credentialAffinityStart` responsible for:
   - returning `0` for size `<= 1`;
   - trimming `affinityKey`;
   - creating the FNV-1a 64-bit hash;
   - converting the final sum to `hash % size`.
3. The helper must write exactly the same sequence as today:
   - `ilonasin-credential-affinity-v1\x00`;
   - decimal token ID;
   - `\x00`;
   - provider instance ID;
   - `\x00`;
   - provider model ID;
   - if non-empty affinity exists, `\x00` then the affinity key.
4. Do not add new affinity inputs or change any safety filtering.
5. Do not change `credentialPressureKey`, `credentialPressureScope`, cursor
   behavior, in-flight pressure accounting, quota planning, retry/fallback
   metadata, route handling, request parsing, storage, management routes, TUI,
   config, logging, or provider adapters.
6. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- the helper produces the same rotation index as the pre-refactor formula for
  empty affinity;
- the helper produces the same rotation index as the pre-refactor formula for
  non-empty affinity;
- whitespace-only affinity still behaves like empty affinity after trimming;
- changing token ID, provider instance, provider model, or safe affinity can
  affect the hash inputs independently;
- no credential secret, upstream account ID, prompt, message, request body, or
  request-id field is introduced as a hash input.

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

- Credential-affinity hash inputs have one private implementation point.
- The exact credential rotation behavior is unchanged.
- The helper makes the allowed affinity inputs auditable against the
  architecture doc.
- Affinity, no-affinity, quota filtering, retry/fallback behavior, metadata,
  storage, logging, management, TUI, config, and providers are unchanged.

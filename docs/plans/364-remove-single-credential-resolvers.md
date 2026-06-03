# 364 Remove Single Credential Resolvers

## Context

`docs/ilonasin-architecture.md` makes same-provider, same-model credential
pooling the default serving behavior. Current serving, keepalive, and model
discovery use plural credential resolution and then apply local routing,
quota, affinity, and pressure policy.

Stale single-credential resolver APIs remain:

- `ResolveAPIKey` on the upstream resolver and service;
- `ResolveAPIKeyCredential` in the SQLite repository boundary;
- `ResolveOAuthBearer` on the OAuth resolver and service;
- `ResolveOAuthBearerCredential` in the SQLite repository boundary.

These paths preserve pre-pooling architecture and make it easier for future
code to accidentally reintroduce first-credential dominance.

## Goal

Remove stale single-credential resolver APIs while preserving plural pooling
resolution and by-ID OAuth refresh resolution.

## Scope

1. Remove `ResolveAPIKey` from `UpstreamCredentialResolver`,
   `UpstreamService`, `UpstreamCredentialRepository`, and SQLite storage.
2. Remove `ResolveOAuthBearer` from `OAuthBearerResolver`,
   `UpstreamService`, `UpstreamCredentialRepository`, and SQLite storage.
3. Update provider-level refresh precheck to use `ResolveOAuthBearers` so it
   still skips refresh when at least one eligible bearer exists.
4. Keep by-ID OAuth resolution:
   - `ResolveOAuthBearerByID` remains for subscription usage and refresh
     retry flows;
   - `ResolveOAuthBearerCredentialByID` remains in SQLite.
5. Preserve plural resolver behavior, eligibility filtering, refresh behavior,
   storage schema, management DTOs, TUI, provider adapters, routing, logging,
   and metadata.

## Out Of Scope

- Removing fallback policy storage or TUI controls.
- Removing permanent tests.
- Changing resolver ordering or credential eligibility rules.
- Changing schema or migrations.
- Changing subscription usage resolution.

## Implementation Steps

1. Remove single resolver methods from credentials interfaces.
2. Delete the single service methods.
3. Change `RefreshOAuthProviderCredential` to call `ResolveOAuthBearers`.
4. Delete the single SQLite API-key and OAuth bearer resolver methods.
5. Adjust imports and run a full compile to catch stale call sites.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/credentials
go test ./internal/storage/sqlite
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide widths, and
cleaning up all temporary files and processes.

## Acceptance

- No single provider-instance API-key resolver remains.
- No single provider-instance OAuth bearer resolver remains.
- Plural resolution remains the only provider-instance credential path.
- By-ID OAuth bearer resolution remains available for refresh and subscription
  usage.
- Compile, vet, and direct serve/manage smokes pass.

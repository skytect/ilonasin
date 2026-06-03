# 368 Fallback Policy Metadata Semantics

## Context

`docs/ilonasin-architecture.md` says same-provider, same-model credential
pooling is the default serving behavior. It also says fallback-policy rows are
operator/display metadata, while serving eligibility is the default
same-provider credential pool.

Plans 365 and 367 removed fallback-policy mutations and dead credentials
fallback helpers, but the snapshot path still reads stale
`credential_fallback_policies.enabled` and `explicit` state. The TUI renders
those values as `enabled` or `disabled` policy controls even though serving no
longer consults them.

## Goal

Make fallback-policy snapshot and TUI semantics match current routing:
fallback metadata is an active same-provider credential group summary, not a
mutable policy state.

## Scope

1. Remove `Enabled` and `Explicit` from credentials fallback metadata.
2. Remove `enabled` and `explicit` from the management `FallbackPolicy` DTO.
3. Change SQLite fallback metadata listing to summarize active credential
   groups directly from `provider_credentials`.
4. Keep `credential_fallback_policies` schema and historical rows untouched for
   a later migration slice.
5. Update TUI fallback metadata rendering so it shows group, kind, credential
   count, and same-provider pool scope without enabled/disabled or
   explicit/default policy language.
6. Keep management visibility filtering unchanged: only supported provider
   credential kinds with at least two active credentials are shown.
7. Do not change serving routing, credential resolution, quota handling,
   fallback event recording, health metadata, config, provider adapters,
   logging, or subscription usage.

## Verification

Review the diff before checks for:

- no management snapshot field still advertises fallback `enabled` or
  `explicit` policy state;
- SQLite no longer joins `credential_fallback_policies` for snapshot metadata;
- the legacy table and migrations remain present;
- the TUI no longer renders fallback policy toggles or state.

Then run:

```sh
rg -n "row\\.Enabled|row\\.Explicit|json:\\\"enabled\\\"|json:\\\"explicit\\\"|COALESCE\\(p\\.enabled|LEFT JOIN credential_fallback_policies" internal
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/storage/sqlite
go test ./internal/management
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Fallback metadata snapshots no longer expose stale enabled/explicit policy
  state.
- The TUI presents fallback metadata as active credential-group pooling
  metadata.
- Historical fallback-policy schema remains intact for compatibility.
- Serving behavior and credential-pool eligibility are unchanged.

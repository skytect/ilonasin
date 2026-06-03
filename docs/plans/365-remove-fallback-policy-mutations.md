# 365 Remove Fallback Policy Mutations

## Context

`docs/ilonasin-architecture.md` says same-provider, same-model credential
pooling is the default serving behavior. It also says fallback-policy rows are
operator/display metadata, while serving eligibility is the default
same-provider credential pool.

Current management and TUI still expose active fallback policy toggles:

- management `POST /_ilonasin/manage/fallback-policies/enable`;
- management `POST /_ilonasin/manage/fallback-policies/disable`;
- management client methods and mutation interfaces for those endpoints;
- TUI hotkeys `f` and `F` that mutate fallback policy rows.

Those toggles no longer affect serving, so they are stale control-plane
surface. The snapshot display of fallback-policy rows can remain metadata-only
for audit and historical visibility.

## Goal

Remove fallback policy mutation surfaces while preserving fallback-policy
metadata listing in management snapshots.

## Scope

1. Remove fallback policy enable/disable methods from management client and
   service interfaces.
2. Remove fallback policy enable/disable HTTP routes.
3. Remove TUI fallback policy mutation actions and hotkeys.
4. Keep fallback-policy snapshot DTOs, visibility filters, SQLite listing, and
   TUI metadata rendering.
5. Keep storage schema and existing rows unchanged.
6. Do not change serving routing, credential pooling, fallback event recording,
   health/quota metadata, provider adapters, config, or logging.

## Out Of Scope

- Dropping `credential_fallback_policies` from SQLite.
- Removing fallback-policy metadata from snapshots.
- Removing fallback metadata/fallback event views.
- Refactoring duplicated fallback credential-kind visibility rules.
- Changing credential pool eligibility.

## Implementation Steps

1. Remove `FallbackPolicyRequest`, `FallbackPolicyResponse`, and
   enable/disable methods from `internal/management/upstreams.go` unless still
   needed after route/client removal.
2. Remove fallback policy mutation methods from `internal/management/client_upstreams.go`.
3. Remove fallback policy mutation routes from `internal/management/http.go`.
4. Remove fallback policy mutation methods from the credentials mutation
   interfaces and service.
5. Remove TUI `f`/`F` key actions and delete fallback policy action helpers.
6. Leave metadata rendering and snapshot conversion intact.
7. Search for stale mutation symbols and compile.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/management
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide widths, and
cleaning up all temporary files and processes.

## Acceptance

- No fallback policy enable/disable route remains.
- No management client/service mutation method remains for fallback policies.
- No TUI fallback policy mutation hotkey or action remains.
- Fallback-policy metadata can still appear in snapshots and provider panes.
- Serving pooling behavior is unchanged.

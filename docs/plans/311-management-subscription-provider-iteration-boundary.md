# 311 Management Subscription Provider Iteration Boundary

## Context

`docs/ilonasin-architecture.md` keeps provider adapters and daemon management
interfaces as separate boundaries. Plan 310 moved subscription usage
request/result DTOs out of management provider types, but management still owns
provider registry iteration for subscription usage refresh:

- `management.Service.Registry` is `provider.Registry`;
- `RefreshSubscriptionUsage` iterates `s.Registry.List()`;
- `refreshCredentialUsage` and `subscriptionUsageFetchRequest` still accept
  `provider.Instance`;
- `management/tokens.go` and `subscription_usage.go` still import
  `internal/provider`.

Plan 309 already added `Service.Providers []ProviderInstance` as a
management-owned provider catalog for snapshot construction. This slice should
reuse that catalog for subscription usage provider iteration too.

This slice must preserve subscription usage refresh behavior, auth retry,
stored `metadata.SubscriptionUsageSnapshot` rows, management subscription usage
JSON, app wiring, provider adapter behavior, and keepalive behavior.

## Plan

1. Add internal-only `AuthIssuer` to the management-owned `ProviderInstance`
   catalog so the catalog preserves all provider fields needed by subscription
   usage refresh without changing management snapshot JSON.
2. Remove `management.Service.Registry` and use `Service.Providers` as the
   management-owned provider catalog for subscription usage refresh.
3. Change `RefreshSubscriptionUsage`, `refreshCredentialUsage`, and
   `subscriptionUsageFetchRequest` to use `management.ProviderInstance`.
4. Keep the same provider selection behavior: refresh only providers with
   `Type == "codex"` and `OAuth == true`.
5. Preserve provider field fidelity in the management usage fetch request:
   provider ID, type, base URL, auth issuer, auth style, and capability flags
   must still reach the app adapter.
6. Keep the app adapter from plan 310 as the only place where management usage
   provider rows become provider instances for the provider client.
7. Leave OAuth provider error-shape conversion and fallback credential-kind
   policy consolidation out of scope for this slice.
8. Review code before checks for behavior drift, provider imports left in
   management subscription usage files, and accidental changes to keepalive or
   management snapshot behavior.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/management
go test ./internal/app
go test ./...
go vet ./...
! rg -n '"ilonasin/internal/provider"|provider\.' internal/management/tokens.go internal/management/subscription_usage.go
```

Run a temporary focused smoke, then remove it before commit. It must:

- construct a management service with `Providers` containing a Codex OAuth
  provider with non-default base URL and non-default auth issuer, plus a
  non-Codex or non-OAuth provider;
- assert subscription usage refresh calls the usage fetcher only for the Codex
  OAuth provider;
- assert provider fields in the usage request match the management provider
  catalog row, including non-default base URL and auth issuer;
- assert app wiring copies `AuthIssuer` from provider registry rows into
  management provider rows;
- assert marshaled management snapshot provider JSON does not include an
  `auth_issuer` key;
- assert `auth_failed` or `upstream_auth_failed` refreshes the bearer and
  retries once with refreshed bearer/account fields;
- assert OAuth rows for unsupported provider rows are ignored;
- assert stored snapshots and response JSON remain compatible.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temp directory.

## Acceptance

- Management subscription usage refresh iterates management-owned provider
  rows instead of provider registry objects.
- `management/tokens.go` and `management/subscription_usage.go` no longer
  import `internal/provider`.
- Existing subscription usage refresh behavior and JSON remain compatible.
- No keepalive, provider adapter, storage schema, TUI, config, or local API
  behavior changes are introduced.

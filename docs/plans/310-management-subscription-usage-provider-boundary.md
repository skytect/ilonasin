# 310 Management Subscription Usage Provider Boundary

## Context

`docs/ilonasin-architecture.md` keeps provider adapters and the daemon
management API as separate boundaries. Recent slices moved model-cache and
snapshot provider display state away from provider DTOs, but subscription usage
refresh still embeds provider request and response shapes inside management:

- `management.Service.UsageClient` is `provider.CodexSubscriptionUsageClient`;
- `internal/management/subscription_usage.go` constructs
  `provider.BearerCredential` and `provider.CodexSubscriptionUsageRequest`;
- `internal/management/subscription_usage_provider.go` reads
  `provider.CodexRateLimitWindow`.

This slice should make management own subscription usage refresh DTOs while app
wiring adapts the provider client. It must preserve refresh behavior, OAuth
auth retry behavior, stored `metadata.SubscriptionUsageSnapshot` rows, and
`/_ilonasin/manage/subscription-usage` JSON.

## Plan

1. Add management-owned subscription usage refresh DTOs:
   - request with provider instance ID, type, base URL, auth issuer, auth
     style, and capability flags;
   - OAuth credential ID, provider instance ID, bearer token, ChatGPT account
     ID, and FedRAMP flag;
   - result with error class, status code, and safe snapshots;
   - snapshot windows with used percent, window minutes, and reset time.
2. Change `management.Service.UsageClient` to a management-owned interface
   accepting those DTOs.
3. Update `internal/management/subscription_usage.go` and
   `subscription_usage_provider.go` so management no longer imports provider
   DTOs for usage refresh request/result/window handling.
4. Add an app-owned adapter in `internal/app` that converts management usage
   DTOs to provider `CodexSubscriptionUsageRequest`, calls the provider client,
   and converts provider results back to management DTOs.
   The management request must carry enough provider instance fields for the
   adapter to reconstruct the provider request without dropping instance ID,
   type, base URL, auth issuer, auth style, or capability flags.
5. Keep the existing provider registry field in management for identifying
   Codex OAuth provider instances. This slice does not remove provider instance
   iteration from subscription refresh.
6. Preserve the existing auth retry flow:
   - first usage fetch;
   - on `auth_failed` or `upstream_auth_failed`, refresh bearer;
   - retry usage fetch once with refreshed bearer fields.
7. Review code before checks for behavior drift, raw provider payload leakage,
   provider DTO references left in management subscription usage files, and
   unintended app keepalive changes.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/management
go test ./internal/app
go test ./...
go vet ./...
! rg -n 'provider\.Codex|provider\.BearerCredential|provider\.CodexRateLimitWindow' internal/management/subscription_usage.go internal/management/subscription_usage_provider.go internal/management/tokens.go
```

Run a temporary focused smoke, then remove it before commit. It must:

- construct a management service with a fake usage client and fake OAuth
  resolver;
- use a non-default Codex base URL and assert the usage client receives it;
- assert the app adapter converts the management request to the same provider
  request shape currently used by subscription usage refresh;
- refresh one Codex OAuth account and assert stored subscription snapshots
  match the fake usage result windows;
- assert auth failure triggers exactly one bearer refresh and one retry;
- assert error-class snapshots preserve existing stale/error behavior;
- assert management DTO sanitization still removes unsafe limit/account text.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temp directory.

## Acceptance

- Management subscription usage refresh no longer builds provider usage request
  DTOs or reads provider usage result/window DTOs directly.
- App wiring owns provider-to-management subscription usage conversion.
- Refresh behavior, retry behavior, persisted usage snapshots, and management
  subscription usage JSON remain compatible.
- No keepalive, provider adapter, storage schema, TUI, config, or local API
  behavior changes are introduced.

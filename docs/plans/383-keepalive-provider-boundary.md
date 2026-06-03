# 383 Keepalive Provider Boundary

## Context

`docs/ilonasin-architecture.md` says provider adapters own provider-specific
behavior, while app orchestration should pass typed route options rather than
embed provider quirks. Prior review flagged `internal/app/keepalive.go` because
the keepalive runner constructs provider chat and subscription-usage DTOs
directly:

- `provider.ChatCredential`;
- `provider.ChatRequest`;
- `provider.BearerCredential`;
- `provider.CodexSubscriptionUsageRequest`.

Slice 382 removed provider coupling from `internal/credentials`. This slice
keeps moving provider DTO construction to app adapter boundaries without
changing keepalive behavior.

## Goal

Make `internal/app/keepalive.go` depend on app-local keepalive interfaces and
DTOs rather than provider chat and usage DTOs. Convert those app-local DTOs to
provider DTOs in adapter code at the app/provider boundary.

## Scope

1. Add app-local keepalive provider DTOs:
   - provider instance fields needed for filtering, chat, and usage refresh;
   - bearer credential fields needed for Codex OAuth chat and usage;
   - chat request/result fields the keepalive runner needs.
2. Add app-local interfaces for:
   - keepalive provider registry/listing;
   - keepalive chat completion;
   - keepalive subscription usage refresh.
3. Add app adapters from `provider.Registry`, `provider.ChatAdapter`, and
   `provider.CodexSubscriptionUsageClient` to the keepalive interfaces.
4. Change `startSubscriptionKeepalive`, `keepaliveRunner`, `runCredential`, and
   `refreshUsage` to use the app-local keepalive DTOs/interfaces.
5. Preserve existing behavior:
   - disabled keepalive still returns a no-op;
   - unverified output cap still logs unavailable and returns a no-op;
   - nil resolver or nil chat adapter still returns a no-op;
   - only Codex OAuth provider instances are considered;
   - bearer resolver calls use provider instance ID and current UTC time;
   - keepalive request body, default model, one-token default, reasoning
     options, and max token cap are unchanged;
   - completion tracking key remains date, slot, provider instance ID, and
     credential ID;
   - chat failure/completion log fields are unchanged;
   - usage refresh remains best-effort and ignored on error.

## Out Of Scope

- No schedule, config, keepalive prompt, model, max-token, or output-cap policy
  changes.
- No changes to provider HTTP adapters, management DTOs, subscription usage
  response shape, storage schema, routing, TUI layout, logging policy, or
  credential resolution behavior.
- No new permanent tests.

## Verification

Run:

```sh
rg -n '"ilonasin/internal/provider"|provider\.' internal/app/keepalive.go
rg -n "provider\\.ChatCredential|provider\\.ChatRequest|provider\\.BearerCredential|provider\\.CodexSubscriptionUsageRequest|provider\\.CodexSubscriptionUsageClient|provider\\.ChatAdapter" internal/app/keepalive.go
rg -n "type keepaliveProvider|type keepaliveCredential|type keepaliveChatClient|type keepaliveUsageClient|keepalive.*Adapter" internal/app
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/app
go test ./...
go vet ./...
```

The first two `rg` commands should produce no hits in
`internal/app/keepalive.go`.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

Also run a temporary focused in-package smoke without keeping a permanent test
file proving:

- disabled and output-cap-unverified keepalive still do not construct a runner;
- `runDue` filters to Codex OAuth providers only;
- the keepalive chat adapter receives the same model, request, provider fields,
  and credential fields;
- app provider adapters preserve full provider instance field mapping for chat
  and subscription usage, including `BaseURL`, `AuthIssuer`, `AuthStyle`,
  `APIKey`, `OAuth`, `OAuthRefresh`, `Chat`, and `ModelDiscovery`;
- successful keepalive marks the same completion key and calls usage refresh;
- failed keepalive does not mark completion;
- app provider adapters preserve provider DTO field mapping for chat
  credentials and subscription usage credentials.

Remove any temporary check before commit.

## Acceptance

- `internal/app/keepalive.go` no longer constructs provider chat or
  subscription-usage DTOs directly.
- Provider DTO conversion for keepalive lives in app adapter code.
- Keepalive runtime behavior and logs are unchanged.
- Compile/package checks, vet, direct serve/manage smokes, and the temporary
  focused keepalive smoke pass.

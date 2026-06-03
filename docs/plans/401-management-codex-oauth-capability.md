# 401 Management Codex OAuth Capability

## Context

The previous slices centralized Codex OAuth policy in the server and
credentials package. The remaining management-facing code still repeats the
same `ProviderInstance` Codex OAuth check in:

- management subscription usage refresh;
- management credential pool group visibility;
- TUI OAuth login provider selection.

All three sites already use `management.ProviderInstance`, so this is a clean
local boundary for the next step. App keepalive uses a separate app-local DTO
and should be handled in a later slice.

## Goal

Centralize management-facing Codex OAuth capability checks on
`management.ProviderInstance` without changing behavior.

## Scope

1. Add a small exported helper in `internal/management` for Codex OAuth
   provider capability.
2. Use it in:
   - `Service.RefreshSubscriptionUsage`;
   - `poolGroupCredentialKinds`;
   - TUI `firstOAuthLoginProvider`.
3. Preserve current behavior:
   - Codex plus OAuth qualifies;
   - `OAuthRefresh` is not required for these three paths;
   - API-key-only providers, non-Codex OAuth providers, and Codex without OAuth
     do not qualify.
4. Do not change app keepalive, credentials package, provider adapters,
   storage, routing, quota, logging, request/response shapes, or TUI layout.

## Verification

Add a temporary focused management/TUI check, then remove it before commit. It
must prove:

- `management.SupportsCodexOAuth` returns true only for Codex OAuth providers;
- management subscription usage refresh still includes Codex OAuth providers
  without requiring `OAuthRefresh`, and still skips API-key-only, non-Codex
  OAuth, and Codex without OAuth providers;
- TUI OAuth login provider selection uses the helper behavior;
- management pool group visibility still allows OAuth pool groups only for
  Codex OAuth providers.

Run:

```sh
rg -n 'instance\\.Type == "codex"|instance\\.Type != "codex"|instance\\.OAuth && instance\\.Type|SupportsCodexOAuth|firstOAuthLoginProvider|poolGroupCredentialKinds' internal/management internal/tui
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/management ./internal/tui
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke at narrow and wide
terminal widths.

## Acceptance

- Management-facing Codex OAuth capability checks share one helper.
- Existing subscription usage, pool group visibility, and TUI OAuth provider
  selection behavior are unchanged.
- App keepalive's separate DTO remains explicitly out of scope for this slice.

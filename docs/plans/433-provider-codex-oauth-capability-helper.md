# 433 Provider Codex OAuth Capability Helper

## Context

Plan 426 found that Codex OAuth capability checks are still duplicated across
package-local boundaries:

- `internal/credentials/provider_boundary.go`;
- `internal/management/snapshot_dto.go`;
- `internal/app/keepalive_provider_adapters.go`;
- `internal/server/provider_policy.go`.

Earlier slices intentionally centralized this behavior within individual
packages first. The remaining duplication now obscures the fact that Codex
OAuth eligibility is provider capability policy, not a separate rule in each
consumer.

## Goal

Add a dependency-neutral provider capability primitive for Codex OAuth support
and replace duplicated `type == "codex" && oauth` predicates while preserving
each package's local DTO boundary.

## Scope

1. Add small helpers in dependency-neutral `internal/metadata`, for example:
   `SupportsCodexOAuth(providerType string, oauth bool) bool`.
2. Add a refresh variant in `internal/metadata`, for example:
   `SupportsCodexOAuthRefresh(providerType string, oauth, oauthRefresh bool)
   bool`.
3. Update existing package-local helpers to delegate to the provider helper:
   - credentials `supportsCodexOAuthCredentials`;
   - credentials `supportsCodexOAuthRefresh`;
   - management `SupportsCodexOAuth`;
   - app `supportsCodexOAuthKeepalive`;
   - server `canRefreshCodexOAuth`.
4. Preserve package DTO boundaries. Do not replace local DTOs with
   `provider.Instance`, and do not import `internal/provider` into
   `internal/credentials`.
5. Preserve behavior exactly:
   - Codex plus OAuth supports bearer/device-login/usage visibility;
   - Codex plus OAuth plus OAuthRefresh supports refresh;
   - API-key-only providers, non-Codex OAuth providers, and Codex without OAuth
     remain unsupported;
   - server refresh still additionally requires `s.refresh != nil`.
6. Do not change provider defaults, config loading, credential resolution,
   keepalive scheduling, subscription usage semantics, management DTOs, TUI
   rendering, storage schema, routing, logging, request/response shapes, or
   provider adapters.
7. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- provider helpers return the exact truth table for Codex OAuth and Codex OAuth
  refresh;
- credentials helpers preserve bearer/device-login and refresh truth tables;
- management helper preserves Codex OAuth behavior;
- app keepalive helper preserves Codex OAuth behavior;
- server `canRefreshCodexOAuth` still requires Codex OAuth and an available
  refresh service.

Then run:

```sh
rg -n 'Type == "codex"|Type != "codex"|type == "codex"|SupportsCodexOAuth|supportsCodexOAuth|canRefreshCodexOAuth' internal/credentials internal/management internal/app internal/server internal/provider
! rg -n '"ilonasin/internal/provider"|provider\\.' internal/credentials
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/metadata ./internal/provider ./internal/credentials ./internal/management ./internal/app ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health and
snapshot over the Unix socket, running bounded `ilonasin manage` at narrow and
wide terminal widths, and cleaning up all temporary files and processes.

## Acceptance

- Codex OAuth capability policy has one dependency-neutral primitive helper.
- Package-local helpers preserve modular DTO boundaries while delegating to the
  shared provider capability policy.
- Existing runtime behavior is unchanged.

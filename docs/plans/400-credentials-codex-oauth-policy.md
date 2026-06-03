# 400 Credentials Codex OAuth Policy

## Context

After centralizing server-local Codex OAuth refresh predicates, the credentials
package still repeats Codex OAuth capability checks in several credential
operations:

- OAuth bearer pooling,
- OAuth bearer lookup by credential ID,
- OAuth device login,
- provider-wide OAuth refresh,
- credential-specific OAuth refresh.

These checks are behaviorally correct, but each site repeats the same
`instance.OAuth`, `instance.OAuthRefresh`, and `instance.Type == "codex"`
policy by hand. That makes it harder to audit the credential boundary and later
move provider capability policy into a clearer adapter/provider surface.

## Goal

Centralize credentials-package Codex OAuth support checks without changing
runtime behavior.

## Scope

1. Add package-local helper functions in `internal/credentials`:
   - one for Codex OAuth bearer/device-login support;
   - one for Codex OAuth refresh support.
2. Replace repeated predicates in:
   - `ResolveOAuthBearers`;
   - `ResolveOAuthBearerByID`;
   - `StartOAuthDeviceLogin`;
   - `RefreshOAuthProviderCredential`;
   - `refreshOAuthCredential`.
3. Preserve existing error messages and error wrapping.
4. Do not change server, app keepalive, management subscription usage,
   management pool visibility, provider adapters, storage schema, routing,
   quota behavior, logging, TUI, config, or request/response shapes.

## Verification

Add a temporary focused credentials package check, then remove it before commit.
It must prove the extracted helpers preserve the current truth table:

- bearer/device-login support requires Codex and OAuth;
- bearer/device-login support does not require `OAuthRefresh`;
- refresh support requires Codex, OAuth, and `OAuthRefresh`;
- API-key-only, non-Codex OAuth, and Codex without OAuth remain unsupported;
- Codex OAuth without refresh remains bearer/device-login-capable but not
  refresh-capable.

Run:

```sh
rg -n 'instance\\.OAuth|instance\\.OAuthRefresh|instance\\.Type == "codex"|instance\\.Type != "codex"|supportsCodexOAuth' internal/credentials
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/credentials
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke at narrow and wide
terminal widths.

## Acceptance

- Credentials package Codex OAuth support checks share helper functions.
- Existing unsupported-credential behavior and messages are unchanged.
- No cross-package DTO or provider behavior changes are introduced.
- Remaining app/management capability duplication is left for a later planned
  slice.

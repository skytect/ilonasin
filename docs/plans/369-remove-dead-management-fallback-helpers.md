# 369 Remove Dead Management Fallback Helpers

## Context

Plans 365, 367, and 368 removed fallback-policy mutation semantics and stopped
rendering fallback rows as enabled or explicit routing policies. Management
still exports two fallback-policy helpers:

- `ProviderAllowsFallbackCredentialKind`;
- `VisibleFallbackPolicies`.

Current search shows no production call sites for either helper. Snapshot
loading already uses unexported metadata visibility helpers. Keeping exported
fallback-policy helpers makes the management package look like it still owns a
public policy-control API for fallback rows.

## Goal

Remove dead exported management fallback-policy helper surface while preserving
snapshot visibility behavior.

## Scope

1. Delete `management.ProviderAllowsFallbackCredentialKind`.
2. Delete `management.VisibleFallbackPolicies`.
3. Keep the unexported snapshot visibility path unchanged:
   - `visibleFallbackPolicies`;
   - `visibleFallbackPolicyMetadata`;
   - `allowedFallbackCredentialKindsByProvider`;
   - `fallbackCredentialKinds`.
4. Keep management snapshot JSON, TUI rendering, SQLite schema, fallback
   metadata listing, serving routing, credential pooling, quota handling,
   fallback events, provider adapters, config, and logging unchanged.
5. Do not rename the `FallbackPolicy` DTO in this slice.

## Verification

Run:

```sh
rg -n "ProviderAllowsFallbackCredentialKind|VisibleFallbackPolicies" internal
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/management
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- No exported management fallback-policy visibility helper remains.
- Snapshot fallback metadata visibility remains internal to management.
- Public management DTO behavior and TUI output remain unchanged.
- Compile, vet, and direct serve/manage smokes pass.

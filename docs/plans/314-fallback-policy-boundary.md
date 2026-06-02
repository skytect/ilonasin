# 314 Fallback Policy Boundary

## Context

`docs/ilonasin-architecture.md` keeps provider configuration, upstream
credential mutation, management DTOs, TUI action filtering, and SQLite storage
as separate boundaries. Fallback credential-kind eligibility is currently
duplicated across layers:

- `credentials.UpstreamService.setFallbackGroup` checks provider registry
  instances directly;
- `management.ProviderAllowsFallbackCredentialKind` repeats the same API-key
  and Codex OAuth checks;
- `management.fallbackPolicyProviderKinds` repeats the checks again for
  snapshot visibility;
- the TUI depends on the management helper for action filtering.

This duplication makes it easy for fallback mutation, management snapshot
visibility, and TUI affordances to drift.

## Plan

1. Add a small credential-domain fallback eligibility helper for provider
   registry instances, used by `UpstreamService.setFallbackGroup`.
2. Add management-owned fallback eligibility helpers for
   `management.ProviderInstance`, used by both management snapshot visibility
   and TUI action filtering.
3. Replace `fallbackPolicyProviderKinds` with a helper that derives allowed
   provider/kind maps from the single management eligibility function.
4. Preserve exact current policy:
   - API-key fallback is allowed only when the provider instance supports API
     keys;
   - OAuth fallback is allowed only when the provider instance supports OAuth
     and has type `codex`;
   - all other credential kinds are rejected;
   - fallback policies remain visible only when the provider/kind is allowed
     and `CredentialCount >= 2`.
5. Keep management API request/response JSON, SQLite schema, TUI behavior,
   provider registry behavior, fallback mutation behavior, and routing behavior
   unchanged.
6. Review code before checks for policy drift, duplicated checks left behind,
   and accidental direct config or SQLite mutation from the TUI.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/credentials
go test ./internal/management
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary focused smoke, then remove it before commit. It must prove:

- credentials mutation eligibility allows API-key providers for API-key
  fallback and rejects OAuth-only providers for API-key fallback;
- credentials mutation eligibility allows Codex OAuth providers for OAuth
  fallback and rejects non-Codex OAuth providers for OAuth fallback;
- credentials mutation eligibility rejects unknown credential kinds;
- management eligibility returns the same provider/kind matrix;
- management eligibility returns false for unknown credential kinds;
- visible fallback policies include only allowed provider/kind rows with at
  least two credentials;
- TUI-visible fallback policies match management visibility for the seeded
  rows.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temporary directory.

## Acceptance

- Fallback eligibility policy has one helper per domain boundary instead of
  repeated ad hoc checks.
- Credentials mutation, management visibility, and TUI filtering preserve the
  existing fallback policy exactly.
- No management route, storage, provider adapter, config, local API, or TUI
  mutation behavior changes are introduced.

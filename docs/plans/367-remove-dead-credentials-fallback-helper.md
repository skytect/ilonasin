# 367 Remove Dead Credentials Fallback Helper

## Context

Plan 365 removed fallback-policy mutation routes, TUI actions, credentials
mutation methods, and SQLite fallback-policy writes. After that cleanup,
`internal/credentials` still exports `ProviderAllowsFallbackCredentialKind`.

Current search shows no call sites for that helper. The management package has
its own fallback-kind visibility helper for snapshot metadata, and serving no
longer consults fallback-policy rows for credential eligibility.

The credentials helper is now dead code. It also keeps one unnecessary exported
provider-instance fallback policy API in the credentials package.

## Goal

Remove the unused credentials fallback-kind helper without changing management
metadata visibility or serving behavior.

## Scope

1. Delete `credentials.ProviderAllowsFallbackCredentialKind`.
2. Keep `management.ProviderAllowsFallbackCredentialKind` and related snapshot
   visibility behavior unchanged.
3. Do not attempt the larger credentials/provider DTO decoupling in this
   slice; credentials still uses provider registry and OAuth DTOs elsewhere.
4. Do not change storage schema, management DTOs, TUI rendering, serving
   routing, credential pooling, provider adapters, logging, or config.

## Verification

Run:

```sh
rg "ProviderAllowsFallbackCredentialKind" internal -n
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/credentials
go test ./...
go vet ./...
```

The search should show only the management helper.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide widths, and
cleaning up all temporary files and processes.

## Acceptance

- The dead credentials helper is gone.
- No credentials package fallback policy mutation/visibility helper remains.
- Management snapshot fallback-policy visibility remains unchanged.
- Compile, vet, and direct serve/manage smokes pass.

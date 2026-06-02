# 305 Provider Registry Config Boundary

## Context

`docs/ilonasin-architecture.md` separates static TOML config from provider
adapter behavior. Provider adapters should own provider-specific defaults and
validation, while app bootstrap should translate config rows into provider-owned
registry inputs.

`internal/provider` currently imports `internal/config` only because
`NewRegistry` accepts the whole `config.Config`. That couples the provider
boundary to the TOML config model and exposes more bootstrap state than the
registry needs.

This slice is boundary-only. It must not change provider defaults, instance ID
validation, base URL validation, auth issuer validation, provider ordering,
model addressing, credentials, server routes, management, TUI, storage, or
config parsing behavior.

## Plan

1. Add provider-owned registry input types:
   - `RegistryConfig` with a `Providers map[string]ProviderConfig`;
   - `ProviderConfig` with `Type`, `BaseURL`, and `AuthIssuer`.
2. Change `provider.NewRegistry` to accept `provider.RegistryConfig`.
3. Move only the provider fields needed for registry construction across the
   app boundary in `internal/app/runtime_core.go`.
4. Remove the `internal/config` import from `internal/provider`.
5. Keep validation, sorting, defaults, and error messages unchanged.
6. Review code before checks for accidental provider behavior changes and
   callsite drift.

## Verification

Run:

```sh
! rg -n '"ilonasin/internal/config"' internal/provider
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/provider
go test ./...
go vet ./...
```

Run a temporary focused registry smoke, then remove it before commit:

- load or construct an app-side config with multiple providers, explicit
  `base_url`, and explicit Codex `auth_issuer`, and assert app-side translation
  preserves every provider row before registry construction;
- construct a registry with DeepSeek, OpenRouter, and Codex provider rows;
- assert deterministic ordering by instance ID;
- assert built-in defaults for base URLs, auth styles, API key/OAuth support,
  chat support, and model discovery support match current behavior;
- assert explicit HTTPS base URL override and Codex auth issuer override still
  work;
- assert invalid instance IDs, unknown types, bad base URLs, and unsupported
  auth issuer overrides still fail.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME` and config, checking management health over the
Unix socket, running `ilonasin manage` under bounded narrow and wide terminals,
and cleaning up the daemon and temp directory.

## Acceptance

- `internal/provider` no longer imports `internal/config`.
- App bootstrap is the only changed callsite translating config provider rows
  into provider registry inputs.
- Provider registry behavior remains unchanged.
- No server, credential, management, storage, TUI, logging, or config parser
  behavior changes are introduced.

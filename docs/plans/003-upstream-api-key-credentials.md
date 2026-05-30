# Plan 003: Upstream API-Key Credentials

## Goal

Add the first upstream credential lifecycle for configured provider instances so
future DeepSeek and OpenRouter adapters can resolve eligible API-key
credentials without crossing local API auth boundaries or leaking secret
material.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- Slice 001 and 002 plans/code.

## Scope

1. Add a provider-instance registry built from static config:
   - validates configured provider instance IDs,
   - applies built-in provider defaults and config overrides,
   - rejects unknown provider types at bootstrap,
   - rejects invalid instance IDs,
   - exposes provider instance metadata to server/TUI without mutating config.
2. Add an upstream API-key credential service for provider instances:
   - add API-key credential for `deepseek` and `openrouter` provider instances,
   - store secret material only in `credential_secrets`,
   - store credential metadata in `provider_credentials`,
   - list credential metadata without secret material,
   - disable credentials idempotently,
   - resolve one eligible credential for a provider instance.
3. Keep Codex credential behavior placeholder-only:
   - no Codex auth import,
   - no `CODEX_HOME`, keyring, cookie, `auth.json`, or `.credentials.json`
     inspection,
   - no `CODEX_API_KEY`, `CODEX_ACCESS_TOKEN`, `OPENAI_API_KEY`, provider
     `env_key`, command-backed bearer, or other Codex/OpenAI auth environment
     import,
   - no agent identity or subscription fallback behavior.
4. Extend `ilonasin manage` with provider/credential views:
   - show configured provider instances from config,
   - show credential labels, provider instance, type, created time, and disabled
     state,
   - exercise add/disable only through an isolated temporary DB in
     `--check`,
   - support interactive add/disable in model update logic without writing
     secrets to config.
5. Extend `serve --check` to seed an upstream API key in an isolated temporary
   DB, resolve it for a configured provider instance, disable it, and verify
   resolution fails after disable. The selected home DB must remain free of
   check-created provider credentials.
6. Add focused tests for:
   - provider registry defaults and overrides,
   - unknown provider type rejection,
   - API-key add/list/disable/resolve lifecycle,
   - disabled credentials excluded from resolution,
   - local API tokens cannot be resolved as upstream credentials,
   - upstream API keys cannot authenticate as local API tokens,
   - TUI credential metadata does not include secret material,
   - no config mutation from TUI credential operations.

## Out of Scope

- Real upstream HTTP calls.
- OAuth browser/device flows.
- Command-backed bearer credentials.
- AWS SigV4.
- Credential fallback across multiple accounts.
- Provider rate-limit policy.
- Codex credential import or subscription auth behavior.
- Encryption beyond local file permissions.

## Design Constraints

- Provider instances are config-defined. Adding a new provider instance requires
  editing `config.toml`, not the TUI.
- The TUI may add credentials only for provider instance IDs that exist in the
  loaded config.
- Local ilonasin client tokens and upstream provider credentials remain separate
  domains:
  - local client tokens live only in `client_tokens`,
  - provider API keys live only in `credential_secrets`,
  - neither domain can authenticate or resolve as the other.
- Credential list and TUI state must never include secret material, full bearer
  values, OAuth tokens, raw account IDs, provider request IDs, or balances.
- `internal/server` may depend on a credential resolver interface but must not
  import SQLite or receive credential mutation methods.
- `internal/tui` may depend on credential management interfaces but must not
  import SQLite.
- `internal/app` wires config registry, SQLite repositories, services, server,
  TUI, and smoke checks.
- `serve --check` and `manage --check` must not print upstream API keys.
- `serve --check` and `manage --check` must not leave check-created
  `provider_credentials` or `credential_secrets` rows in the selected home DB.
- `codex` remains listed as a configured provider type if present, but API-key
  credential add/resolve must reject it clearly until a specific Codex
  credential design is approved.
- Outside `credential_secrets`, the add transaction, repository secret scan,
  resolver result, and future adapter request construction, plaintext provider
  API keys must not enter durable state, TUI list state, rendered views,
  snapshots, logs, config, metadata structs, request metadata, health events,
  fallback events, errors, or smoke output.
- TUI input/update state may hold a plaintext provider API key only transiently
  while submitting an add operation. It must be cleared on submit, cancel,
  navigation, error, and quit.
- Upstream API-key add must reject local-token-looking values such as `iln_...`.
- API-key provider base URLs must be HTTPS for this slice. Local HTTP and other
  unsafe provider bases require a future explicit unsafe/local design.

## Proposed Package Changes

```text
internal/provider/
  registry.go        # provider instance registry from config
internal/credentials/
  upstream.go        # upstream credential manager/resolver interfaces/service
internal/storage/sqlite/
  provider_credentials.go
internal/tui/
  provider_credentials.go or expanded model state
```

Interface shape:

```go
type UpstreamCredentialManager interface {
    AddAPIKey(ctx context.Context, providerInstanceID, label, apiKey string) (UpstreamCredentialMetadata, error)
    List(ctx context.Context) ([]UpstreamCredentialMetadata, error)
    Disable(ctx context.Context, id int64) error
}

type UpstreamCredentialResolver interface {
    ResolveAPIKey(ctx context.Context, providerInstanceID string) (ResolvedAPIKeyCredential, error)
}
```

The concrete service may implement both. TUI receives manager capability. Server
and future router code receive resolver capability only.

Provider registry semantics:

- Registry construction happens in `internal/provider`, from loaded static
  config and built-in defaults.
- Instance IDs must be non-empty, lowercase ASCII identifiers made of
  `a-z`, `0-9`, `_`, and `-`; they must not contain `/`, whitespace, control
  characters, or model-address delimiters.
- Config maps already make exact duplicate keys impossible; case variants are
  rejected by requiring lowercase IDs.
- Unknown provider types fail bootstrap.
- Registry entries include instance ID, provider type, effective base URL, auth
  style, placeholder flag, and credential capabilities.
- Config `base_url` overrides are allowed only when valid HTTPS URLs for this
  slice.
- The registry is an immutable snapshot. SQLite repositories must not validate
  provider type, provider capabilities, or config membership.

Repository semantics:

- Add validation lives in the credential service using the provider registry.
  SQLite only performs transactional CRUD.
- Add stores the plaintext provider API key only in `credential_secrets`.
- Add stores redacted display metadata only in `provider_credentials`.
- Add writes `provider_credentials` and the `credential_secrets` row in one
  transaction with `secret_kind = "api_key"` and rolls back both rows on failure.
- Duplicate enabled labels for the same provider instance are rejected. Disabled
  credentials keep their secret material and label, so labels remain reserved in
  this slice.
- List returns deterministic metadata without secret material.
- Disable is idempotent for already disabled rows and not-found for missing IDs.
- Resolve returns one enabled API-key credential for the requested provider
  instance, deterministic by oldest enabled credential for this slice.
- Eligible means: configured instance, provider type supports API-key
  credentials, credential kind is `api_key`, row is enabled, and exactly one
  `credential_secrets` row exists with `secret_kind = "api_key"`.
- Resolve returns typed not-found/no-eligible, unsupported-provider,
  duplicate-label, and invalid-secret-domain errors.
- Resolve returns secret material only to adapter-bound resolver callers; it is
  never exposed to TUI or request metadata.
- `ResolvedAPIKeyCredential` is not JSON-facing, is never accepted by TUI state,
  and must not implement debug/string formatting that includes the key.
- List queries must not select or scan `credential_secrets.secret_material`.
- If new indexes, constraints, or columns are required, add a real migration 002
  and extend the migration runner; do not edit migration 001 semantics for
  already-applied databases.

## Verification

Run:

```text
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
```

Automated checks must prove:

- no upstream API key appears in `manage --check` output,
- no upstream API key appears in errors or smoke-check output,
- selected home DB has no check-created provider credential rows after
  `serve --check` or `manage --check`,
- provider credential secret rows do not authenticate to the local API,
- local API token rows do not resolve as upstream credentials,
- disabled upstream credentials are not resolved,
- TUI credential operations leave `config.toml` unchanged.
- provider registry rejects unknown provider types, invalid IDs, non-HTTPS
  API-key provider base URLs, and Codex API-key credential attempts including
  base URL override variants such as `codex-dev`,
- disabled credential resolution returns no secret,
- list paths do not select `credential_secrets.secret_material`.

## Review Questions

1. Is API-key credential lifecycle the right next slice before provider HTTP
   adapters?
2. Are the manager/resolver boundaries narrow enough to prevent server/TUI
   privilege creep?
3. Does the plan keep Codex-specific auth safely deferred?
4. Are the smoke checks strong enough without contacting real providers?

# Plan 008: OAuth Account State

## Goal

Add the SQLite/service/TUI foundation for OAuth-style upstream credentials and
provider account metadata without implementing Codex subscription inference or
inspecting external Codex credential stores.

This moves the codebase toward the architecture's OAuth-capable account model
while keeping provider credentials, local API auth, TUI, config, transport, and
storage boundaries separate.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/plans/001-initial-go-scaffold.md`
- `docs/plans/002-local-api-tokens.md`
- `docs/plans/003-upstream-api-key-credentials.md`
- `docs/plans/004-nonstreaming-chat-adapters.md`
- `docs/plans/005-streaming-chat-adapters.md`
- `docs/plans/006-model-discovery-cache.md`
- `docs/plans/007-credential-fallback-health.md`
- `AGENTS.md`

## Scope

1. Extend provider registry metadata with an explicit OAuth capability flag.
   - `codex` is OAuth-capable but remains chat/model-discovery placeholder-only.
   - `deepseek` and `openrouter` remain API-key providers in this slice.
2. Add upstream OAuth credential service methods for provider instances:
   - add a managed OAuth credential record,
   - store access and refresh token secrets only in `credential_secrets`,
   - store expiry/scopes/refresh metadata only in `oauth_tokens`,
   - store account metadata only in `provider_accounts`,
   - list credential metadata without token material,
   - list provider account metadata without full account IDs,
   - mark refresh failure and update token expiry metadata without exposing
     token material.
3. Add typed repository boundaries for OAuth state.
   - TUI receives only metadata/account list reader methods.
   - Server/chat adapter paths do not receive OAuth secret resolution yet.
   - API-key resolver behavior must be unchanged.
4. Add account identity hashing:
   - full upstream account IDs are never stored,
   - account hashes are domain-separated and deterministic,
   - hash input is exactly
     `sha256("ilonasin-provider-account-v1\x00" + provider_type + "\x00" + provider_instance_id + "\x00" + canonical_account_id)`,
   - canonical account IDs are trimmed, Unicode-control-free strings and are
     rejected if empty after trimming,
   - account hashes are storage keys, not display identity, and are not shown in
     the TUI by default,
   - account display labels are optional, sanitized, and not treated as
     authoritative identity.
5. Extend `ilonasin manage`:
   - show OAuth-capable provider instances,
   - show OAuth credential/account metadata rows,
   - show token expiry and refresh failure state when present,
   - never show access tokens, refresh tokens, full account IDs, raw JWTs,
     OAuth callback URLs, bearer headers, or token endpoint payloads.
6. Extend `manage --check`:
   - seed an OAuth credential/account in an isolated temporary DB,
   - include unsafe labels, token-looking strings, JWT-looking values, and raw
     account/request markers in seed data,
   - render through the real TUI model,
   - assert useful safe metadata appears,
   - assert forbidden secret/account/provider markers do not render,
   - assert seeded OAuth access/refresh token markers appear only in allowed
     `credential_secrets.secret_material` rows and never in
     `provider_credentials`, `oauth_tokens`, `provider_accounts`, TUI metadata,
     logs, or errors,
   - assert seeded raw account IDs, JWTs, callback URLs, bearer headers, token
     endpoint body markers, cookies, command stdout markers, and provider
     request markers appear in no table, TUI metadata, logs, or errors,
   - assert the selected home DB is not mutated.
7. Extend `serve --check`:
   - prove OAuth credential rows cannot authenticate as local API tokens,
   - prove OAuth credential rows are not resolved by API-key resolver methods,
   - keep chat/model discovery behavior unchanged for API-key providers.

## Out of Scope

- Browser OAuth login.
- Device-code OAuth login.
- Refresh token HTTP calls.
- Codex chat, model discovery, Responses API, or subscription inference.
- Importing Codex `auth.json`, keyrings, cookies, `.credentials.json`,
  `CODEX_API_KEY`, `CODEX_ACCESS_TOKEN`, or `OPENAI_API_KEY`.
- Agent identity, signed assertions, command-backed bearer credentials,
  AWS SigV4, MCP auth, or plugin auth.
- Cross-provider/model fallback.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` remains a compile/package check only.
- TUI may mutate SQLite but must not mutate `config.toml`.
- Provider adapters must not import SQLite or TUI.
- Server must not receive OAuth mutation methods or OAuth token secrets in this
  slice.
- Full OAuth access/refresh token values may exist only in process memory
  during the add/update call and in `credential_secrets.secret_material`.
  They are forbidden everywhere else.
- `provider_credentials` stores only metadata: provider instance, kind,
  label, fallback group, timestamps, and disabled state.
- OAuth credentials must leave `secret_prefix` and `secret_last4` blank or use
  fixed non-secret redaction markers. They must never be derived from OAuth
  access tokens, refresh tokens, ID tokens, JWTs, bearer tokens, callback URLs,
  or account IDs.
- `oauth_tokens` stores only secret row references, expiry, scopes,
  last-refresh timestamp, and normalized refresh failure class.
- `provider_accounts` stores account hashes and safe display metadata only.
- Allowed refresh failure classes are exactly `refresh_token_expired`,
  `refresh_token_invalidated`, `refresh_token_reused`,
  `refresh_unauthorized`, `refresh_network_error`, `refresh_timeout`,
  `refresh_unavailable`, and `refresh_invalid_response`. Unknown values are
  rejected or coerced to `refresh_unavailable` before persistence/display.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens outside `credential_secrets.secret_material`, full provider request
  IDs, full account IDs, balances, credit totals, raw JWTs, OAuth callback URLs,
  token endpoint bodies, cookies, or provider command stdout.
- OAuth input must accept only access and refresh bearer token material. It must
  reject ID tokens, agent identity JWTs/private keys, OAuth callback URLs, token
  endpoint request/response bodies, cookies, and command stdout as inputs or
  metadata.
- Listing methods must not select `credential_secrets.secret_material`.
- OAuth add must reject local-token-looking values such as `iln_...` as token
  material.
- OAuth add must reject API-key-only providers unless a future provider plan
  explicitly enables OAuth for them.
- Codex remains placeholder-only for serving: OAuth credentials can be stored
  and displayed, but no Codex upstream request uses them in this slice.

## Proposed Package Changes

```text
internal/provider/
  provider.go      # OAuth capability metadata
internal/credentials/
  upstream.go      # OAuth/account manager and metadata interfaces/types
internal/storage/sqlite/
  db.go            # OAuth/account insert/list/update repository methods
internal/tui/
  tui.go           # OAuth/account metadata view
internal/app/
  app.go           # isolated smoke checks for OAuth/account safety
```

Interface shape:

```go
type OAuthCredentialManager interface {
    AddOAuthCredential(ctx context.Context, input NewOAuthCredentialInput) (OAuthCredentialMetadata, error)
    MarkOAuthRefreshFailure(ctx context.Context, credentialID int64, failureClass string) error
}

type OAuthMetadataReader interface {
    ListOAuthCredentials(ctx context.Context) ([]OAuthCredentialMetadata, error)
    ListProviderAccounts(ctx context.Context) ([]ProviderAccountMetadata, error)
}
```

The concrete upstream service may implement API-key management, OAuth mutation,
and OAuth metadata reading, but TUI receives only `OAuthMetadataReader`.
Server-facing resolver interfaces remain narrower and API-key-specific.

## Verification

Run:

```text
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Smoke checks must prove:

- no permanent test files exist,
- OAuth token material does not appear in `manage --check` output,
- full account IDs and JWT-like strings do not appear in `manage --check`
  output,
- direct DB smoke scans prove seeded OAuth access/refresh tokens appear only in
  `credential_secrets.secret_material`,
- direct DB smoke scans prove raw account IDs, JWTs, callback URLs, bearer
  headers, token endpoint body markers, cookies, command stdout markers, and
  provider request markers appear in no table,
- selected home DB has no check-created OAuth credentials/accounts after
  `manage --check`,
- OAuth credentials cannot authenticate `/v1/models`,
- OAuth credentials are not returned by `ResolveAPIKey` or `ResolveAPIKeys`,
- API-key chat/model behavior is unchanged,
- no config file is mutated by the TUI path,
- before/after selected-home snapshots for `provider_credentials`,
  `credential_secrets`, `oauth_tokens`, `provider_accounts`, and config
  bytes/mtime are identical across `manage --check`,
- selected-home snapshots include row IDs plus safe metadata for the relevant
  tables rather than row counts alone.

## Review Questions

1. Is this the right OAuth/account foundation before a real Codex auth flow?
2. Are the OAuth service interfaces narrow enough to avoid leaking token
   material into server, TUI, or provider adapters?
3. Does storing account hashes plus safe display labels satisfy the no-full
   account ID rule?
4. Are the smoke checks strong enough without real OAuth network calls or
   permanent tests?

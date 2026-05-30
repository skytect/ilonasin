# Plan 014: Codex Device OAuth Login

## Goal

Make `ilonasin manage` able to create a Codex OAuth credential through the
Codex device-code OAuth flow.

Previous slices can store, refresh, discover models with, and use an existing
Codex OAuth credential for non-streaming chat. This slice adds the missing
login entry point for terminal/TUI users while keeping browser callback login,
automatic 401 refresh, Codex streaming, revocation, and account fallback out of
scope.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- prior plans `001` through `013`
- Codex source snapshot `/tmp/codex-src-0.133.0/codex-rs`
- `AGENTS.md`

## Scope

1. Add a provider-owned Codex device OAuth adapter:
   - use only HTTPS `auth_issuer` values already validated by provider
     registry rules: no userinfo, query, or fragment,
   - join paths by replacing any issuer path with the exact documented auth
     paths rather than concatenating attacker-controlled query/fragment data,
   - use per-request timeout, total login timeout, maximum poll count, and
     min/max poll interval clamps,
   - use a bounded max response body size for all three auth endpoints,
   - disable cross-origin redirects for these secret-bearing auth requests,
   - require expected response content types and strict JSON with EOF for JSON
     endpoints,
   - honor context cancellation and return marker-free normalized failures,
   - request device code with
     `POST {auth_issuer}/api/accounts/deviceauth/usercode`,
   - send strict JSON `{"client_id":"app_EMoamEEZ73f0CkXaXp7hrann"}`,
   - parse strict JSON with non-empty string `device_auth_id`, non-empty string
     `user_code` or `usercode`, and string-or-number `interval`,
   - compute verification URL as `{auth_issuer}/codex/device`,
   - poll `POST {auth_issuer}/api/accounts/deviceauth/token`,
   - send strict JSON fields exactly `device_auth_id` and `user_code`,
   - treat 403 and 404 as pending until the configured timeout,
   - parse strict JSON with non-empty string `authorization_code`,
     non-empty string `code_challenge`, and non-empty string `code_verifier`,
   - exchange the authorization code at
     `POST {auth_issuer}/oauth/token`,
   - send `application/x-www-form-urlencoded` fields exactly
     `grant_type=authorization_code`, `code`, `redirect_uri`,
     `client_id`, and `code_verifier`,
   - use redirect URI `{auth_issuer}/deviceauth/callback`,
   - parse strict JSON `id_token`, `access_token`, `refresh_token`, and
     optional positive `expires_in`,
   - never persist raw `id_token`, device auth ID, authorization code,
     code verifier, endpoint bodies, or raw token responses.
2. Add a credential-service login boundary:
   - service exposes `StartOAuthDeviceLogin(providerInstanceID)` and
     `CompleteOAuthDeviceLogin(handle)`,
   - only configured Codex OAuth providers can use this flow,
   - API-key providers and stale provider IDs are rejected before HTTP,
   - the credential service retains device auth ID and other secret/poll state
     behind an opaque handle,
   - the opaque handle is random, non-secret-bearing, single-use, expiring,
     bounded in memory, and removed on success, failure, timeout, or TUI
     cancellation,
   - the TUI receives only provider ID, verification URL, user code, and the
     opaque handle,
   - provider adapter methods may return raw access and refresh tokens only to
     the credential service,
   - raw access and refresh tokens are never returned to TUI, app smoke output,
     provider metadata, logs, or errors,
   - returned access/refresh tokens pass existing OAuth secret validation,
   - `id_token` is decoded by the credential service only to derive safe
     account metadata and is then dropped,
   - require a non-empty ChatGPT account ID claim before storing,
   - account ID claim key is
     `https://api.openai.com/auth.chatgpt_account_id`,
   - email claim keys are top-level `email` or
     `https://api.openai.com/profile.email`,
   - plan claim key is
     `https://api.openai.com/auth.chatgpt_plan_type`,
   - account hash input remains the existing
     provider type + provider instance ID + canonical account ID,
   - account display label is sanitized email when present, otherwise a coarse
     `Codex account` label,
   - plan label is sanitized plan claim when present,
   - raw account IDs are only used to compute the existing account hash and are
     not stored or displayed,
   - access and refresh tokens are stored through existing OAuth credential
     storage with kind-specific secret rows.
3. Add TUI login wiring:
   - add a dedicated `l` key for OAuth login,
   - keep `o` for OAuth row selection and `r` for refresh,
   - pressing `l` requests a Codex device login challenge for the first
     configured OAuth login-capable provider in deterministic registry order,
   - the view shows only provider ID, verification URL, and user code,
   - completion runs through a Bubble Tea command and reloads OAuth metadata,
   - success clears temporary login state and shows the new OAuth row,
   - failures show only `OAuth login failed`,
   - TUI model state must not store device auth IDs, access tokens, refresh
     tokens, id tokens, auth codes, code verifiers, raw endpoint bodies, account
     IDs, cookies, or provider payloads.
4. Extend `manage --check`:
   - perform the mutating device login smoke in an isolated check DB/home,
   - use the real device-login adapter against a fake HTTPS auth server,
   - assert exact user-code, poll, and token-exchange paths,
   - assert exact content types and request body fields,
   - assert pending 403/404 polling behavior,
   - assert interval clamp, total timeout, max polls, response body cap,
     per-request timeout, hung endpoint, oversized body, trailing JSON,
     non-JSON, wrong content type, and redirect behavior are honored with short
     check values,
   - assert empty/malformed `device_auth_id`, `user_code`, authorization code,
     code challenge, and code verifier are rejected before persistence,
   - assert login handles are single-use, expiring, bounded, and contain no
     device auth ID, auth code, code verifier, access token, refresh token,
     account ID, id token, or endpoint body marker,
   - assert returned `id_token` is parsed for safe metadata and dropped,
   - assert access and refresh tokens are stored only as `oauth_access` and
     `oauth_refresh` secret material,
   - assert device auth ID, authorization code, code verifier, id token, raw
     account ID, access token, refresh token, token endpoint body, provider
     payload, request ID, balance, credit, cookie, and bearer markers do not
     appear in any SQLite table outside the expected secret rows, TUI output,
     CLI output, model cache, or errors,
   - assert API-key providers, stale provider IDs, malformed responses,
     unsafe token values, missing account ID, HTTP errors, timeout, and
     non-JSON/trailing JSON failures are rejected with normalized marker-free
     errors,
   - assert selected-home config bytes/mtime and DB snapshot remain unchanged
     by `manage --check`; only the isolated login-smoke DB may mutate.
5. Keep existing behavior stable:
   - `serve` must not receive the device login controller,
   - `serve` must not read refresh tokens or login secrets,
   - existing OAuth refresh, Codex model discovery, Codex non-streaming chat,
     DeepSeek/OpenRouter chat, and fallback checks remain passing.

## Out of Scope

- Browser authorization-code callback login.
- Opening a browser automatically.
- Importing Codex `auth.json` or keyring state.
- Revocation/logout.
- Automatic refresh during `/v1/models` or `/v1/chat/completions`.
- 401 recovery and retry after refresh.
- Codex streaming chat.
- OAuth account fallback or 429 account cycling.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` remains a compile/package check only.
- Do not push.
- Provider adapters do not import SQLite, TUI, or config loaders.
- Storage does not perform HTTP.
- TUI does not mutate `config.toml`.
- Device login temporary state stays in memory only.
- The HTTP adapter owns endpoint shape and response parsing.
- The credential service owns provider eligibility, token validation, account
  metadata normalization, and persistence.
- Failure messages stay coarse and marker-free.

## Proposed Package Changes

```text
internal/provider/
  oauth_device.go   # Codex device code, polling, and token exchange adapter
internal/credentials/
  upstream.go       # device login controller interfaces and service methods
internal/tui/
  tui.go            # l key login flow and view state
internal/app/
  app.go            # manage-check fake device OAuth server and leak checks
```

Interface shape:

```go
type OAuthDeviceLoginController interface {
    StartOAuthDeviceLogin(ctx context.Context, providerInstanceID string) (OAuthDeviceLoginChallenge, error)
    CompleteOAuthDeviceLogin(ctx context.Context, handle string) (OAuthCredentialMetadata, error)
}
```

`OAuthDeviceLoginChallenge` contains only provider ID, verification URL, user
code, and an opaque handle. The provider adapter returns raw token response
material only to the credential service; the service validates and persists
tokens through existing OAuth credential storage and drops the raw `id_token`.

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
- `manage` can create a Codex OAuth credential from the device flow,
- the fake auth server sees exact endpoint paths, content types, and bodies,
- the TUI shows only the device URL and user code while login is pending,
- raw token/login/account markers do not leak to views, metadata, model cache,
  or errors,
- ineligible providers do not cause auth HTTP calls,
- malformed/error/timeout flows fail with coarse marker-free errors,
- existing refresh, model discovery, chat, fallback, observability, and pruning
  checks still pass.

## Review Questions

1. Is device-code login the right first OAuth login path for the local TUI?
2. Is decoding id-token claims without storing the raw token sufficient for
   account metadata in this slice?
3. Should browser callback login remain separate to keep this slice bounded?

# Plan 012: Codex OAuth Refresh

## Goal

Make stored Codex OAuth refresh tokens usable from the local management path.

The previous slice let `/v1/models` use an already-valid Codex OAuth access
token. This slice adds the missing refresh boundary so an expired Codex access
token can be refreshed deliberately from `ilonasin manage`, while keeping
browser login, device login, Codex chat, automatic 401 recovery, and account
fallback out of scope.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- prior plans `001` through `011`
- `AGENTS.md`

## Scope

1. Carry OAuth auth issuer metadata through the provider registry:
   - Codex default issuer is `https://auth.openai.com`,
   - config `auth_issuer` overrides are accepted only for HTTPS URLs,
   - non-OAuth providers do not expose refresh behavior,
   - no config file mutation.
2. Add a narrow OAuth refresh service boundary:
   - refresh is explicit by selected credential ID,
   - service verifies the credential belongs to a configured Codex provider
     instance with explicit refresh capability,
   - disabled credentials and non-OAuth credentials are rejected,
   - future OAuth-capable non-Codex providers must not be sent through the
     Codex refresh adapter,
   - storage first selects the eligible credential ID and refresh secret ID
     without reading any secret material,
   - storage then fetches only
     `credential_secrets.id = oauth_tokens.refresh_token_secret_id AND
     credential_secrets.credential_id = selected_id AND
     credential_secrets.secret_kind = 'oauth_refresh'`,
   - cross-linked secret IDs are skipped/rejected and never read,
   - storage reads only the selected credential's matching `oauth_refresh`
     secret,
   - access-token secrets are not read before the refresh call,
   - returned access and optional refresh tokens pass the existing OAuth secret
     validator before persistence,
   - API-key credential paths remain separate.
3. Add a Codex OAuth refresh HTTP adapter:
   - POST JSON to `{auth_issuer}/oauth/token`,
   - body fields are only `client_id`, `grant_type: refresh_token`, and
     `refresh_token`,
   - use Codex CLI client ID `app_EMoamEEZ73f0CkXaXp7hrann`,
   - strict JSON response parsing with no trailing data,
   - require a non-empty safe `access_token`,
   - accept optional `refresh_token`; when omitted, keep the old refresh token,
   - accept and drop `id_token`; it is never stored or displayed,
   - accept optional positive `expires_in` and store `now + expires_in`,
   - apply existing timeout/body-size limits for auth HTTP,
   - persist only this normalized failure-class allowlist:
     `refresh_token_expired`, `refresh_token_reused`,
     `refresh_token_invalidated`, `refresh_unauthorized`,
     `refresh_network_error`, `refresh_timeout`, `refresh_http_error`,
     `refresh_body_too_large`, `refresh_invalid_response`, and
     `refresh_unavailable`,
   - map 401 response `error` values `refresh_token_expired`,
     `refresh_token_reused`, and `refresh_token_invalidated` exactly; unknown
     401 values map to `refresh_unauthorized`,
   - map non-401 HTTP responses to `refresh_http_error`,
   - map malformed/trailing/empty JSON or unsafe returned token values to
     `refresh_invalid_response`,
   - map timeout, network, and body-size failures to their matching coarse
     classes,
   - never return raw response bodies, token endpoint payloads, token values,
     account IDs, request IDs, balances, or credits in errors.
4. Update SQLite token state atomically:
   - update existing `oauth_access` secret material for the credential,
   - update existing `oauth_refresh` secret material only when a new refresh
     token is returned,
   - update `oauth_tokens.expires_at`, `last_refresh_at`, and clear
     `refresh_failure_class` on success,
   - set `last_refresh_at` and safe `refresh_failure_class` on failure,
   - do not mutate provider account metadata, API-key credentials, fallback
     policies, model cache, telemetry, migrations, or config.
5. Add TUI refresh control:
   - TUI receives only a narrow refresh-by-credential-ID controller plus the
     existing OAuth metadata reader,
   - raw refresh/access tokens are never stored in TUI model fields, Bubble Tea
     messages, errors, or smoke output,
   - key `r` refreshes the currently selected OAuth credential ID, not the
     first enabled credential,
   - OAuth rows get their own deterministic selection state and visible cursor,
   - disabled rows and rows whose provider is not configured for Codex refresh
     are not refreshable,
   - the TUI reloads OAuth metadata after success,
   - failures show only a coarse `OAuth refresh failed` error,
   - the view continues to display only safe account labels, plan labels,
     expiry timestamps, and normalized refresh failure class.
6. Extend `manage --check`:
   - seed a Codex OAuth credential with expired access token and refresh token
     markers,
   - use the real Codex OAuth HTTP adapter against a fake HTTPS auth server,
   - assert exact `POST /oauth/token`,
   - assert `Content-Type: application/json`,
   - assert request JSON fields are exactly `client_id`, `grant_type`, and
     `refresh_token`,
   - assert no access token, account ID, cookie, provider payload, extra
     secret header, or raw endpoint body is sent,
   - assert strict response JSON with no trailing data,
   - assert timeout, too-large body, HTTP failures, malformed/trailing JSON,
     unsafe returned token values, and 401 error mappings produce marker-free
     normalized failures,
   - press `r` through the TUI path,
   - seed two eligible OAuth credentials and set OAuth selection to the second
     one,
   - assert only the selected credential's refresh token is read/sent/updated,
   - assert the unselected credential remains unchanged,
   - seed a cross-linked refresh secret ID and assert it is not read, sent, or
     updated,
   - assert access token material changed and old access token material no
     longer exists,
   - assert refresh token material changes only when the fake response includes
     a replacement refresh token,
   - assert new access token appears only as `oauth_access` secret material,
   - assert replacement refresh token appears only as `oauth_refresh` secret
     material,
   - assert old refresh token is kept exactly when no replacement is returned,
   - assert old/replaced token markers are absent from non-secret tables,
     TUI output, failure messages, model cache, telemetry, and smoke output,
   - assert returned `id_token` marker is accepted-and-dropped and never
     persists or displays,
   - assert expiry, `last_refresh_at`, and `refresh_failure_class` are updated,
   - assert disabled, non-OAuth, stale-provider, and API-key credentials cannot
     be refreshed,
   - assert refresh failure records a normalized class and the TUI error is
     coarse,
   - assert no token material, token endpoint body, raw response, account ID,
     provider payload, request ID, balance, or credit marker appears in TUI
     output, metadata tables, model cache, telemetry, or errors,
   - assert isolated DB snapshots mutate only OAuth token/secret timestamp
     state expected for refresh,
   - assert selected-home DB and config snapshots remain unchanged.
7. Extend `serve --check` only enough to keep existing model discovery behavior
   passing with the new interfaces:
   - server keeps only the OAuth access-token resolver,
   - server does not receive the refresh manager or refresh HTTP adapter,
   - no `serve` path can read `oauth_refresh` secret material.

## Out of Scope

- Browser OAuth login.
- Device-code OAuth login.
- Importing Codex `auth.json` or keyring state.
- Automatic refresh during `/v1/models` or `/v1/chat/completions`.
- 401 recovery and request retry after refresh.
- Codex chat completions and `/responses`.
- Token revocation/logout.
- Account fallback or 429 account cycling.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` remains a compile/package check only.
- Do not push.
- Provider adapters do not import SQLite, TUI, or config loaders.
- Storage does not perform HTTP.
- TUI does not edit `config.toml`.
- Refresh-token material is read only for the selected credential and only for
  the manage/TUI refresh operation.
- The server package must not depend on the refresh controller, refresh HTTP
  adapter, or repository methods that read `oauth_refresh`.
- Access-token material is written on success but not displayed.
- Errors, TUI output, metadata, model cache, and smoke output must remain
  marker-free.
- The refresh HTTP adapter owns endpoint shape and response parsing; the
  credential service owns eligibility and storage updates.

## Proposed Package Changes

```text
internal/provider/
  provider.go      # auth issuer default/override metadata
  oauth.go         # Codex OAuth refresh HTTP adapter
internal/credentials/
  upstream.go      # OAuth refresh manager interfaces and service method
internal/storage/sqlite/
  db.go            # resolve refresh secret and atomically update tokens
internal/tui/
  tui.go           # r key refresh action and smoke exercise helper
internal/app/
  app.go           # manage-check fake refresher and no-leak assertions
```

Interface shape:

```go
type OAuthTokenRefresher interface {
    RefreshOAuthToken(ctx context.Context, req OAuthRefreshRequest) (OAuthRefreshResult, error)
}

type OAuthRefreshRequest struct {
    ProviderType string
    AuthIssuer   string
    RefreshToken string
    Now          time.Time
}

type OAuthRefreshResult struct {
    AccessToken  string
    RefreshToken string
    ExpiresAt    *time.Time
}
```

The TUI-facing controller is narrower:

```go
type OAuthRefreshController interface {
    RefreshOAuthCredential(ctx context.Context, credentialID int64) error
}
```

Only the credential service and HTTP adapter see `OAuthRefreshRequest` with raw
refresh-token material.

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
- Codex auth issuer default and HTTPS override validation work, including
  rejection of `http`, missing host, userinfo, query, and fragment overrides,
- refresh reads only the selected credential's matching `oauth_refresh` secret,
- refresh never reads or sends access-token material as input,
- cross-linked refresh/access secret IDs are rejected and not read, sent, or
  updated,
- real HTTP adapter sends exactly `POST /oauth/token` with only allowed JSON
  fields and no extra headers carrying secrets,
- success updates access token, optional refresh token, expiry, and
  `last_refresh_at`,
- success clears previous refresh failure state,
- failure records only an allowed normalized refresh failure class,
- 401 `refresh_token_expired`, `refresh_token_reused`, and
  `refresh_token_invalidated` are preserved; unknown 401 maps to
  `refresh_unauthorized`,
- unsafe returned access/refresh/id token material is not persisted or
  displayed,
- disabled, stale-provider, non-OAuth, and API-key credentials are not
  refreshable,
- TUI `r` refreshes the selected OAuth credential ID and reloads safe OAuth
  metadata,
- refresh manager/refresher is manage-only and unavailable to `serve`,
- TUI success/failure output contains no token, account, endpoint body, raw
  response, provider payload, request ID, balance, or credit markers,
- serve model discovery still uses valid OAuth access tokens and still skips
  no-eligible Codex OAuth cache rows,
- selected-home DB and config snapshots remain unchanged across checks.

## Review Questions

1. Is manual TUI refresh the right first refresh path before automatic 401
   recovery?
2. Should missing `refresh_token` in a successful response keep the existing
   refresh token?
3. Is carrying `auth_issuer` through provider registry enough for this slice?

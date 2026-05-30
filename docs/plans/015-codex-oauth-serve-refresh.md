# Plan 015: Codex OAuth Serve Refresh

## Goal

Make `ilonasin serve` recover Codex OAuth credentials by refreshing them when
the access token is expired before a request, or when Codex returns an upstream
401 during model discovery or non-streaming chat.

Previous slices added Codex OAuth storage, manual TUI refresh, model discovery,
non-streaming chat, and device login. This slice closes the runtime recovery
gap while keeping Codex streaming, revocation/logout, browser login, and account
fallback out of scope.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- prior plans `001` through `014`
- `AGENTS.md`

## Scope

1. Add a narrow server refresh boundary:
   - define a serve-facing OAuth refresh interface that can refresh one
     provider instance without returning refresh-token material,
   - define a serve-facing exact-ID OAuth bearer resolver for retrying the same
     credential after a 401,
   - `server` receives only OAuth bearer resolution and refresh orchestration,
   - `server` never reads `oauth_refresh` secrets directly,
   - refresh remains implemented in `credentials.UpstreamService`,
   - device-login controller is still not passed to `serve`.
2. Add refresh serialization:
   - serialize automatic refresh per provider/credential inside the credential
     service,
   - concurrent requests for the same expired provider or same 401 credential
     must share one in-flight refresh rather than reading/reusing the same
     refresh token multiple times,
   - after acquiring the refresh lock, re-resolve the bearer before refreshing;
     if another request already refreshed a usable access token, return without
     calling the refresh endpoint again,
   - after a 401-triggered refresh, re-resolve the exact credential ID before
     retrying,
   - context cancellation must not leave locks held,
   - refresh failure state must be recorded once and surfaced to waiters as a
     coarse marker-free error.
3. Add provider-level refresh selection:
   - credential service can select the deterministic first active Codex OAuth
     credential for a provider that has both access and refresh secret links,
   - expired credentials remain refreshable,
   - storage query selects only active Codex OAuth rows for the exact provider,
   - owned `oauth_access` and `oauth_refresh` secret links must be joined and
     validated before any `secret_material` is read,
   - disabled, stale-provider, non-OAuth, missing-refresh, missing-access, and
     cross-linked secret rows are rejected before reading refresh material,
   - successful refresh updates existing access/refresh secret rows and expiry
     through the existing storage path,
   - refresh failures set the existing refresh failure state and return coarse
     marker-free errors.
4. Refresh before request when needed:
   - if Codex OAuth bearer resolution returns no eligible credential because
     only expired access tokens are present, ask the credential service to
     refresh one Codex credential for that provider, then resolve the bearer
     again,
   - do this for `/v1/models` and non-streaming `codex/...` chat,
   - API-key providers and non-Codex providers keep current behavior,
   - if refresh cannot produce an eligible access token, return the existing
     coarse `credential_unavailable` path without leaking refresh details.
5. Refresh and retry after upstream 401:
   - Codex model discovery must surface upstream 401 distinctly enough for the
     server to recognize auth recovery,
   - Codex non-streaming chat must surface upstream 401 distinctly enough for
     the server to recognize auth recovery,
   - on the first upstream 401 for a Codex OAuth credential, refresh that same
     credential ID and retry the upstream request once with the new access
     token,
   - after refresh, re-resolve the access bearer by the same credential ID; do
     not provider-resolve a different Codex account for the retry,
   - do not retry non-401 failures, malformed responses, timeouts, 429s, or
     client cancellation,
   - do not rotate accounts or fallback groups on 401 in this slice,
   - if the retry also fails with 401, return a coarse local upstream auth
     failure and record only safe metadata.
6. Define local failure semantics:
   - `/v1/chat/completions` no-refreshable or refresh failure before any
     upstream attempt returns HTTP 401 with error class `credential_unavailable`
     and records metadata `credential_unavailable`,
   - `/v1/chat/completions` first upstream 401 followed by refresh failure
     returns HTTP 502 with error class `upstream_auth_failed` and records
     metadata `upstream_auth_failed`,
   - `/v1/chat/completions` retry upstream 401 after a successful refresh
     returns HTTP 502 with error class `upstream_auth_failed` and records
     metadata `upstream_auth_failed`,
   - `/v1/models` no-refreshable or refresh failure means Codex contributes no
     models and does not expose stale Codex cache for that auth failure,
   - `/v1/models` first upstream 401 followed by refresh failure, or retry 401
     after successful refresh, suppresses stale Codex cache for that provider;
     if every attempted provider failed without safe cache, return HTTP 502
     with error class `model_discovery_failed`,
   - raw refresh errors, upstream 401 bodies, and provider payloads are never
     copied into OpenAI-compatible local error JSON or metadata.
7. Preserve safety invariants:
   - no prompt, completion, request body, response body, raw provider payload,
     raw SSE chunk, access token, refresh token, bearer header, provider
     request ID, account ID, balance, credit, cookie, tool argument, or tool
     result is stored or displayed,
   - refresh-token material is only read inside the credentials/storage boundary,
   - provider adapters do not import SQLite, TUI, config loaders, or credential
     storage,
   - storage performs no HTTP,
   - failure messages remain coarse and marker-free.
8. Extend `serve --check`:
   - seed an expired Codex OAuth credential and assert `/v1/models` refreshes
     it before discovery and sends only the new access bearer upstream,
   - seed an expired Codex OAuth credential and assert non-streaming
     `codex/...` chat refreshes it before chat and sends only the new access
     bearer upstream,
   - seed a valid-but-stale Codex access token where fake upstream returns 401,
     then assert model discovery refreshes and retries once with the new access
     bearer,
   - do the same for non-streaming Codex chat,
   - assert refresh failure, missing refresh token, disabled credential,
     stale-provider credential, and cross-linked secrets do not send refresh
     tokens upstream and do not leak markers,
   - missing access-token link coverage includes `NULL` access secret ID,
     missing access secret row, and cross-linked access secret row; all three
     are rejected before refresh-token material is read,
   - direct marker scans prove rejected refresh tokens are not read into output,
     local errors, request metadata, model cache, health events, or fallback
     events,
   - concurrent expired model/chat requests and concurrent 401 model/chat
     requests cause exactly one refresh endpoint call per provider/credential
     and all waiters use the refreshed access token or the same coarse failure,
   - seed a second eligible Codex OAuth credential and assert a 401 retry uses
     the refreshed bearer from the original credential ID, not the other account,
   - assert 429 and 5xx do not trigger OAuth refresh,
   - assert retry count/metadata is safe and does not store raw auth or provider
     payload markers,
   - assert DeepSeek/OpenRouter API-key chat, streaming chat, fallback, Codex
     model discovery, Codex non-streaming chat, device login, and TUI refresh
     checks still pass.

## Out of Scope

- Codex streaming passthrough.
- OAuth account fallback or 429 account cycling.
- Browser authorization-code callback login.
- Revocation/logout.
- Importing Codex `auth.json` or keyring state.
- Provider command-backed auth.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` remains a compile/package check only.
- Do not push.
- `serve` may trigger refresh through the credential service, but must not
  receive refresh-token material.
- Provider adapters surface normalized status/error classes only; they do not
  perform credential refresh.
- Credential refresh updates existing rows rather than creating a new credential.
- 401 recovery is separate from 429 handling.

## Proposed Package Changes

```text
internal/credentials/
  upstream.go        # provider-scoped refresh controller and serialization
internal/storage/sqlite/
  db.go              # resolve first refreshable OAuth credential for provider
internal/provider/
  chat.go            # model result can carry safe HTTP status
  http_chat.go       # surface Codex 401 for chat/model discovery
internal/server/
  server.go          # refresh-before-request and refresh-once-after-401
internal/app/
  app.go             # serve-check refresh/retry smokes
```

Interface shape:

```go
type OAuthProviderRefreshController interface {
    ResolveOAuthBearerByID(ctx context.Context, credentialID int64, now time.Time) (ResolvedOAuthBearerCredential, error)
    RefreshOAuthProviderCredential(ctx context.Context, providerInstanceID string) error
    RefreshOAuthCredentialIfBearer(ctx context.Context, credentialID int64, staleBearerToken string) error
    RefreshOAuthCredential(ctx context.Context, credentialID int64) error
}
```

`RefreshOAuthProviderCredential` chooses the deterministic first refreshable
Codex OAuth credential for the provider. `RefreshOAuthCredentialIfBearer` is
used after an upstream 401 when the server already knows the credential ID and
bearer it sent. `ResolveOAuthBearerByID` is used only for retrying that same
credential after a successful 401-triggered refresh. Provider-scoped
pre-request refresh re-resolves after lock acquisition before reading any
refresh token. Exact-ID 401 refresh is serialized by credential ID, so
concurrent waiters share the first refresh result and then re-resolve the same
credential ID for retry.

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
- expired Codex OAuth access tokens are refreshed before model discovery/chat,
- upstream 401 for Codex model discovery/chat refreshes and retries once,
- 401 retry reuses the same credential ID/account rather than selecting another
  eligible Codex account,
- refresh tokens are never sent to provider model/chat endpoints,
- 429 and 5xx do not trigger OAuth refresh,
- refresh failures are marker-free and update refresh failure state,
- concurrent refresh callers share one refresh endpoint call and do not reuse a
  refresh token,
- DeepSeek/OpenRouter and existing Codex non-refresh behavior still pass.

## Review Questions

1. Is provider-scoped refresh-before-resolve the right smallest runtime recovery
   step for expired access tokens?
2. Should upstream 401 retry refresh the exact credential ID that was used,
   rather than selecting any account for the provider?
3. Is it acceptable to keep 429 account/provider cycling explicitly out of scope?

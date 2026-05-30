# Plan 011: Codex OAuth Model Discovery

## Goal

Make stored Codex OAuth credentials useful for safe model discovery.

This is the first serving path that uses OAuth credential material. It should
make `GET /v1/models` able to refresh/list Codex models through a stored Codex
OAuth access token while keeping Codex chat completions, browser login, device
login, and token refresh out of scope.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- prior plans `001` through `010`
- `AGENTS.md`

## Scope

1. Add a narrow OAuth bearer resolution boundary:
   - storage can resolve one enabled OAuth access token for a provider
     instance,
   - eligibility is exactly: configured OAuth-capable provider instance,
     `provider_credentials.kind = 'oauth'`, `provider_credentials.disabled_at
     IS NULL`, `oauth_tokens.access_token_secret_id` points at
     `credential_secrets.secret_kind = 'oauth_access'`, and either
     `oauth_tokens.expires_at IS NULL` or it is later than the current time,
   - selection is deterministic oldest enabled credential first by
     `provider_credentials.id ASC`,
   - expired credentials, disabled credentials, missing access-token secret
     rows, and non-OAuth-capable provider instances are skipped or rejected,
   - only `credential_secrets.secret_material` for the selected access-token
     secret row is read,
   - refresh-token secrets are never joined, selected, or read in this slice,
   - API-key resolution remains separate.
   Interface shape:
   ```go
   type OAuthBearerResolver interface {
       ResolveOAuthBearer(ctx context.Context, providerInstanceID string, now time.Time) (ResolvedOAuthBearerCredential, error)
   }

   type ResolvedOAuthBearerCredential struct {
       ID                 int64
       ProviderInstanceID string
       BearerToken        string
       ExpiresAt          *time.Time
   }
   ```
   The server receives this read-only resolver separately from OAuth mutation
   and metadata readers. Production passes `time.Now().UTC()`; check paths use
   an injected deterministic clock so expired/not-expired cases are stable.
2. Generalize provider model discovery credentials from API-key-only naming to
   bearer credentials:
   - use a typed adapter credential shape with `Kind`/source metadata rather
     than a generic untyped secret string,
   - DeepSeek/OpenRouter API-key model discovery continues unchanged,
   - Codex model discovery uses OAuth access-token bearer auth,
   - DeepSeek/OpenRouter must not receive OAuth credentials,
   - Codex must not resolve through API-key paths,
   - no provider adapter receives refresh tokens or account IDs.
3. Enable Codex model discovery:
   - keep Codex chat completions unimplemented,
   - add an explicit model-discovery capability path separate from `APIKey` and
     `Placeholder`,
   - Codex can be model-discovery-capable through OAuth without becoming
     API-key-capable or chat-capable,
   - register a Codex model discoverer only for `/models`,
   - request `GET {base}/models?client_version=ilonasin` for Codex,
   - accept only strict JSON with no trailing garbage,
   - accept only a non-empty normalized model list from a `data` array,
   - copy only safe model fields already supported by model cache metadata,
   - validate provider model IDs with the same model ID rules as other
     providers,
   - use provider-default Codex capability flags for now,
   - keep existing model timeout and body-size bounds,
   - failed, malformed, empty, duplicate, timeout, or too-large responses must
     not replace existing cache rows.
   - if Codex has no eligible OAuth access credential, `/v1/models` skips Codex
     and must not return stale Codex cache rows.
4. Extend `serve --check`:
   - seed a Codex OAuth credential in the isolated serve-check DB,
   - seed disabled, expired, and no-access-secret Codex OAuth credentials that
     must not be selected,
   - fake a Codex `/models` upstream,
   - assert `/v1/models` returns `codex/<model>` from OAuth bearer auth,
   - assert the fake upstream saw only the OAuth access token as bearer auth,
   - assert disabled, expired, no-access-secret, and refresh-token markers are
     not seen by the fake upstream,
   - assert DeepSeek/OpenRouter still use API-key model discovery and cannot
     receive OAuth credentials,
   - assert Codex still cannot be resolved by API-key resolver methods,
   - assert failed/malformed/empty/duplicate/timeout/too-large Codex model
     refreshes do not replace cached rows,
   - assert a separate no-eligible-Codex-OAuth scenario with disabled,
     expired, and no-access-secret credentials plus preseeded Codex cache does
     not expose stale Codex cache rows,
   - assert no OAuth token material, refresh token, account ID, raw response,
     provider payload, or unsafe model metadata leaks into output, metadata,
     model cache, or errors,
   - assert Codex chat completion still returns a coarse unimplemented error.
5. Extend `manage --check` only if signatures or visible metadata change.

## Out of Scope

- Browser OAuth login.
- Device-code OAuth login.
- OAuth token refresh and 401 recovery.
- Codex chat completions and `/responses`.
- Codex streaming.
- Importing Codex local `auth.json` or keyring state.
- Agent identity, command bearer auth, ChatGPT account headers, cookies, and
  app-server auth.
- Cross-account Codex fallback or 429 account cycling.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` remains a compile/package check only.
- Do not push.
- Provider adapters must not import SQLite, credentials services, or TUI.
- Server gets only resolver interfaces, not OAuth mutation methods.
- Model discovery may read an OAuth access token, but never a refresh token.
- The OAuth bearer resolver must not join `credential_secrets` for refresh
  token rows.
- The model discoverer credential value includes a typed `Kind` such as
  `api_key` or `oauth_access`, so credential domains remain explicit.
- Do not store or display prompts, completions, request bodies, response
  bodies, raw provider payloads, raw SSE chunks, tool arguments/results, full
  bearer tokens, full provider request IDs, full account IDs, balances, or
  credits.
- Codex OAuth model discovery must not make Codex an API-key provider.
- Codex chat remains explicitly unavailable until a future `/responses` slice.
- Failure messages must stay coarse and marker-free.

## Proposed Package Changes

```text
internal/credentials/
  upstream.go      # OAuth bearer resolver interface/type
internal/storage/sqlite/
  db.go            # resolve OAuth access-token bearer only
internal/provider/
  provider.go      # explicit model discovery capability metadata
  chat.go          # bearer credential naming for model discovery
  http_chat.go     # Codex /models client_version query
internal/server/
  server.go        # /v1/models can use API-key or OAuth resolver
internal/app/
  app.go           # serve-check Codex OAuth model discovery smoke
```

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
- DeepSeek/OpenRouter API-key model discovery still works,
- Codex OAuth model discovery returns a normalized `codex/<model>` row,
- Codex model discovery uses the stored access token as bearer auth,
- disabled, expired, and missing-access-secret Codex OAuth credentials are not
  selected,
- no eligible Codex OAuth access credential means stale Codex cache rows are
  not exposed,
- refresh token material is never joined/read by resolver or seen by fake
  upstream,
- DeepSeek/OpenRouter cannot receive OAuth credentials,
- Codex cannot be resolved by API-key resolver methods,
- failed/malformed/empty/duplicate/timeout/too-large Codex refreshes preserve
  existing model cache rows,
- Codex chat remains unavailable with a coarse local error,
- distinct access token, refresh token, and account markers are seeded and
  scanned directly: access token appears only in
  `credential_secrets.secret_material`; refresh token is never seen upstream;
  account markers appear nowhere outside account hash metadata,
- OAuth token material, raw provider payloads, unsafe model metadata, and
  account IDs do not appear in CLI output, metadata, errors, model cache, or
  logs,
- selected-home DB and config snapshots remain unchanged across checks.

## Review Questions

1. Is `/v1/models` the right first serving path for Codex OAuth?
2. Is the bearer credential abstraction narrow enough without collapsing API-key
   and OAuth storage domains?
3. Does keeping Codex chat unimplemented avoid false support claims?

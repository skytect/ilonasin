# 259 SQLite OAuth Resolution Storage Split

## Goal

Make SQLite OAuth storage more modular by splitting bearer and refresh
resolution/update behavior out of `internal/storage/sqlite/oauth_credentials.go`
without changing credential behavior.

`oauth_credentials.go` should keep OAuth credential/account insertion, listing,
account metadata, and refresh-failure persistence. A new same-package file should
own the resolver/update path used by provider routing, OAuth refresh, and
subscription usage.

## Scope

1. Add `internal/storage/sqlite/oauth_resolution.go`.
2. Move these methods from `oauth_credentials.go` to the new file:
   - `ResolveOAuthBearerCredential`
   - `ResolveOAuthBearerCredentials`
   - `ResolveOAuthBearerCredentialByID`
   - `ResolveOAuthRefreshCredential`
   - `ResolveOAuthRefreshCredentialForProvider`
   - `ResolveOAuthRefreshToken`
   - `UpdateOAuthTokens`
3. Move these helper declarations to the new file:
   - `oauthBearerRow`
   - `materializeOAuthBearer`
   - `updateCredentialSecret`
4. Treat `updateCredentialSecret` as a same-package shared secret update
   helper. It will live beside refresh-update code but remain available to the
   insert/upsert path in `oauth_credentials.go`.
5. Preserve the existing SQL text, ordering, transaction boundaries, error
   mapping, expiry checks, missing-secret behavior, token update behavior, and
   transient ChatGPT routing-claim parsing.
6. Keep `MarkOAuthRefreshFailure`, `UpdateOAuthAccountMetadata`,
   credential/account insert/upsert helpers, `ListOAuthCredentials`, and
   `ListProviderAccounts` in `oauth_credentials.go`.
7. Do not change exported interfaces, management APIs, DTOs, provider behavior,
   server routing, TUI rendering, logging policy, schema, migrations, config,
   or tests.

## Boundaries

- No schema changes.
- No direct TUI SQLite or `config.toml` changes.
- No new secret exposure paths.
- No raw OAuth access tokens, refresh tokens, bearer tokens, full account IDs,
  prompts, completions, request bodies, response bodies, SSE chunks, tool
  arguments, or tool results in logs, management snapshots, or TUI output.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused storage smoke, then remove it before commit:

- create a temporary SQLite store;
- insert a Codex OAuth credential with safe metadata;
- assert bearer resolution returns the access token and safe credential
  metadata;
- assert `ResolveOAuthBearerCredential` keeps the current first-row behavior;
- assert `ResolveOAuthBearerCredentials` returns ordered eligible rows while
  skipping expired or missing-secret rows;
- assert `ResolveOAuthBearerCredentialByID` preserves the current
  `now.IsZero()` expiry handling and rejects expired credentials with a real
  `now`;
- assert refresh resolution returns only secret IDs and resolves the refresh
  token through `ResolveOAuthRefreshToken`;
- assert `ResolveOAuthRefreshCredentialForProvider` preserves the current first
  provider credential behavior;
- assert refresh resolution validates access and refresh secret IDs against the
  same credential;
- assert `UpdateOAuthTokens` updates access token material, optional refresh
  token material, expiry, and last-refresh metadata;
- assert `UpdateOAuthTokens` preserves the previous refresh token when the
  update omits a replacement refresh token;
- assert disabled credentials, missing access/refresh secret rows, and
  zero-row secret updates still return the existing ineligible/not-found
  errors;
- assert `ListOAuthCredentials` still does not expose token material;
- assert expired or missing-secret credentials remain ineligible.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify API/providers/usage/logs
   chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- OAuth resolver/update code lives in `oauth_resolution.go`.
- OAuth insert/list/account metadata and refresh-failure storage remain in
  `oauth_credentials.go`.
- Public storage behavior and interfaces are unchanged.
- Token material remains confined to `credential_secrets` reads/writes.
- Full ChatGPT account IDs remain transient bearer routing claims only.
- Compile, vet, focused storage smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

# Codex Authentication Map

This document maps Codex CLI authentication as of the installed `codex-cli 0.133.0` source snapshot at `/tmp/codex-src-0.133.0/codex-rs`, with supplemental routing-header and model-discovery checks against `rust-v0.135.0` source at `/tmp/codex-src-0.135.0/codex-rs`. It intentionally does not inspect local credential files, environment values, cookies, or account-specific state. Token examples are redacted.

## Executive Summary

Codex has several auth lanes, not one global API-key path:

- First-party model auth supports OpenAI API keys, ChatGPT OAuth tokens, externally supplied ChatGPT access tokens, and agent identity assertions.
- Provider auth can override first-party auth with a provider `env_key`, a literal experimental bearer token, or a command that prints a bearer token.
- Amazon Bedrock uses either `AWS_BEARER_TOKEN_BEDROCK` or AWS SDK credentials plus SigV4 signing.
- MCP servers have separate bearer-token and OAuth flows, with separate storage in the OS keyring or `CODEX_HOME/.credentials.json`.
- Plugin and connector paths reuse ChatGPT/agent auth when they talk to ChatGPT backend services, while plugin-defined MCP servers normalize into the same MCP auth system.
- App-server WebSocket auth is a separate control-plane auth boundary, not model-provider auth.

Codex does not implement generic account failover on rate limit. It has 401 recovery paths for expired or invalid auth, and it surfaces/parses rate-limit metadata, but 429s are not used to switch credentials or providers automatically.

## Auth Modes

The core first-party enum has four modes: `ApiKey`, `Chatgpt`, `ChatgptAuthTokens`, and `AgentIdentity` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:48-55`).

`ApiKey` stores an OpenAI API key and sends it as a bearer token. `login_with_api_key` writes `auth_mode: ApiKey`, `OPENAI_API_KEY`, and no OAuth token bundle (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:528-542`).

`Chatgpt` is the normal browser or device OAuth login. The default ChatGPT backend is `https://chatgpt.com/backend-api`; refresh and revoke default to `https://auth.openai.com/oauth/token` and `https://auth.openai.com/oauth/revoke` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:93-97`).

`ChatgptAuthTokens` is an external-token lane. Codex converts externally supplied ChatGPT token data into an `AuthDotJson` shape with an access token, empty refresh token, account metadata, and forced ephemeral storage (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:935-998`).

`AgentIdentity` is not a regular OAuth access-token login. `login_with_access_token` verifies an agent-identity JWT and stores the raw JWT string in `agent_identity` with `auth_mode: AgentIdentity` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:544-564`, `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/storage.rs:31-48`). At runtime, that JWT is decoded into an `AgentIdentityAuthRecord` containing `agent_runtime_id`, `agent_private_key`, account/user/email/plan metadata, and FedRAMP status (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/storage.rs:50-77`).

## Credential Sources and Precedence

Codex defines readers for `OPENAI_API_KEY`, `CODEX_API_KEY`, and `CODEX_ACCESS_TOKEN` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:465-482`), but normal `AuthManager` loading does not treat `OPENAI_API_KEY` as a first-party auth source.

The load order is important:

1. `CODEX_API_KEY` wins when environment API-key auth is enabled.
2. External ChatGPT token state is checked next.
3. If storage mode is `Ephemeral`, loading stops there.
4. `CODEX_ACCESS_TOKEN` is then interpreted as agent identity.
5. Persistent storage is read last (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:737-784`).

`OPENAI_API_KEY` is still used in helper/UI paths: onboarding can prefill API-key entry from it, and realtime has a temporary OpenAI-provider fallback to it when realtime requires API-key auth (`/tmp/codex-src-0.133.0/codex-rs/tui/src/onboarding/auth.rs:793-810`, `/tmp/codex-src-0.133.0/codex-rs/core/src/realtime_conversation.rs:949-973`).

Provider auth has its own precedence. A provider `env_key` or `experimental_bearer_token` becomes `Authorization: Bearer <redacted>` before Codex falls back to first-party auth (`/tmp/codex-src-0.133.0/codex-rs/model-provider/src/auth.rs:78-104`). Command-backed provider auth creates a provider-scoped `AuthManager`, so it does not use the normal user auth manager (`/tmp/codex-src-0.133.0/codex-rs/model-provider/src/auth.rs:65-75`).

The built-in OpenAI provider also maps `OPENAI_ORGANIZATION` and `OPENAI_PROJECT` into `OpenAI-Organization` and `OpenAI-Project` request headers (`/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:318-352`). Those are header environment mappings, separate from the OAuth JWT `organization_id` and `project_id` values that are only placed into the local login success redirect (`/tmp/codex-src-0.133.0/codex-rs/login/src/server.rs:851-883`).

## Credential Storage

Auth storage modes are `File`, `Keyring`, `Auto`, and `Ephemeral` (`/tmp/codex-src-0.133.0/codex-rs/config/src/types.rs:85-98`). File storage is `CODEX_HOME/auth.json`; keyring storage uses service `Codex Auth` and key `cli|<hashed codex_home>`; `Auto` tries keyring first and falls back to file; `Ephemeral` is process memory keyed by hashed `codex_home` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/storage.rs:84-86`, `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/storage.rs:125-153`, `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/storage.rs:160-174`, `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/storage.rs:265-333`).

The serialized auth shape includes optional `auth_mode`, `OPENAI_API_KEY`, `tokens`, `last_refresh`, and `agent_identity` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/storage.rs:31-48`). On Unix, file-backed auth is written with mode `0600` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/storage.rs:125-153`).

Forced login settings can log out incompatible auth. A forced ChatGPT method accepts `Chatgpt`, `ChatgptAuthTokens`, and `AgentIdentity`; a forced API method accepts API-key mode. Forced workspace enforcement compares account IDs and logs out on mismatch (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:617-702`).

## ChatGPT OAuth

Browser login runs a local callback server. Defaults are issuer `https://auth.openai.com`, callback port `1455`, and fallback port `1457` (`/tmp/codex-src-0.133.0/codex-rs/login/src/server.rs:54-57`). The CLI constructs `ServerOptions` with the public OAuth client ID and current credential store mode, then starts the local server and prints the auth URL (`/tmp/codex-src-0.133.0/codex-rs/cli/src/login.rs:116-131`).

The OAuth client ID constant is `app_EMoamEEZ73f0CkXaXp7hrann` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:927-933`). The browser flow uses authorization-code PKCE, with `response_type=code`, `scope=openid profile email offline_access api.connectors.read api.connectors.invoke`, `code_challenge_method=S256`, `id_token_add_organizations=true`, `codex_cli_simplified_flow=true`, `state`, `originator`, and optional `allowed_workspace_id` (`/tmp/codex-src-0.133.0/codex-rs/login/src/server.rs:491-518`). State is 32 random bytes encoded as base64url (`/tmp/codex-src-0.133.0/codex-rs/login/src/server.rs:521-525`).

The callback handler validates `state`, requires an authorization `code`, exchanges it at `{issuer}/oauth/token`, optionally enforces workspace, obtains a Codex backend API key by token exchange when available, persists tokens, and redirects to a local success page (`/tmp/codex-src-0.133.0/codex-rs/login/src/server.rs:263-405`, `/tmp/codex-src-0.133.0/codex-rs/login/src/server.rs:714-839`). The token exchange POST is form-encoded with `grant_type=authorization_code`, `code`, `redirect_uri`, `client_id`, and `code_verifier` (`/tmp/codex-src-0.133.0/codex-rs/login/src/server.rs:714-785`).

The device-code flow is separate. It POSTs to `{issuer}/api/accounts/deviceauth/usercode`, shows the user a code at `{issuer}/codex/device`, polls `{issuer}/api/accounts/deviceauth/token`, and then exchanges the returned authorization code with redirect URI `{issuer}/deviceauth/callback` (`/tmp/codex-src-0.133.0/codex-rs/login/src/device_code_auth.rs:61-96`, `/tmp/codex-src-0.133.0/codex-rs/login/src/device_code_auth.rs:98-146`, `/tmp/codex-src-0.133.0/codex-rs/login/src/device_code_auth.rs:159-222`). In the CLI dispatch, device code is used when `--device-auth` is selected; otherwise normal login starts browser login or stdin API/access-token flows (`/tmp/codex-src-0.133.0/codex-rs/cli/src/main.rs:1148-1175`, `/tmp/codex-src-0.133.0/codex-rs/cli/src/login.rs:266-299`). A device-code-to-browser fallback helper exists in source, but no local call site was found in this snapshot (`/tmp/codex-src-0.133.0/codex-rs/cli/src/login.rs:301-360`).

Security note: the login code redacts sensitive query keys in logs (`/tmp/codex-src-0.133.0/codex-rs/login/src/server.rs:624-638`), but the local success redirect includes an `id_token` query parameter for the localhost success page (`/tmp/codex-src-0.133.0/codex-rs/login/src/server.rs:841-895`). A router should avoid copying this browser-facing pattern into logs, remote callbacks, telemetry, or shared URLs.

Codex HTTP clients can also install a process-global ChatGPT Cloudflare cookie store. The source explicitly limits this shared jar to infrastructure cookies and warns not to store ChatGPT account, session, auth, or other user-specific cookies in it (`/tmp/codex-src-0.133.0/codex-rs/codex-client/src/chatgpt_cloudflare_cookies.rs:10-15`, `/tmp/codex-src-0.133.0/codex-rs/codex-client/src/chatgpt_cloudflare_cookies.rs:46-56`). The default reqwest client and backend client add this cookie store (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/default_client.rs:203-229`, `/tmp/codex-src-0.133.0/codex-rs/backend-client/src/client.rs:143-159`). A router should not treat cookies as portable auth state unless they are scoped per account/session and explicitly allowlisted.

## Token Types and Claims

ChatGPT OAuth token data contains `id_token`, `access_token`, `refresh_token`, and `account_id` (`/tmp/codex-src-0.133.0/codex-rs/login/src/token_data.rs:10-25`). Parsed ID-token info includes email, plan type, ChatGPT user ID, account ID, FedRAMP flag, and raw JWT (`/tmp/codex-src-0.133.0/codex-rs/login/src/token_data.rs:27-42`).

Claims are parsed from the JWT payload without signature verification in this parsing helper (`/tmp/codex-src-0.133.0/codex-rs/login/src/token_data.rs:117-128`). The helper reads top-level `email`, namespaced profile email, and namespaced auth values such as `chatgpt_plan_type`, `chatgpt_user_id`, `user_id`, `chatgpt_account_id`, and `chatgpt_account_is_fedramp` (`/tmp/codex-src-0.133.0/codex-rs/login/src/token_data.rs:71-99`, `/tmp/codex-src-0.133.0/codex-rs/login/src/token_data.rs:137-161`).

Codex persists `TokenData.account_id` separately from the access token and uses
that value for `ChatGPT-Account-ID`. Ilonasin does not store a separate full
account ID. It may derive `ChatGPT-Account-ID` transiently from an access-token
claim when present, but this is best-effort compatibility rather than the
durable Codex source. Normal metadata stores only the account hash, safe display
labels, plan labels, and local credential IDs. Logs, management snapshots,
CLI/TUI output, request metadata, and fallback metadata must not contain the
full account ID.

Agent identity JWT claims include issuer, audience, issued/expiry times, runtime ID, private key, account/user/email/plan metadata, and FedRAMP status (`/tmp/codex-src-0.133.0/codex-rs/agent-identity/src/lib.rs:64-78`). When JWKS is provided, validation requires RS256, expected issuer, and expected audience (`/tmp/codex-src-0.133.0/codex-rs/agent-identity/src/lib.rs:147-171`). The default audience is `codex-app-server`, and the expected issuer is `https://chatgpt.com/codex-backend/agent-identity` (`/tmp/codex-src-0.133.0/codex-rs/agent-identity/src/lib.rs:33-37`).

## Refresh, Revocation, and 401 Recovery

Refresh uses a JSON POST to the refresh endpoint with `client_id`, `grant_type: refresh_token`, and `refresh_token`. Success can update ID/access/refresh tokens; HTTP 401 is classified as permanent or other refresh-token failures (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:813-856`). Refresh 401 error codes include `refresh_token_expired`, `refresh_token_reused`, and `refresh_token_invalidated` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:858-911`).

Logout can revoke managed ChatGPT tokens. Revocation chooses a refresh token when possible, falls back to an access token, and POSTs JSON containing `token`, `token_type_hint`, and `client_id` for refresh-token revocation (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/revoke.rs:55-65`, `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/revoke.rs:84-148`). Refresh and revoke endpoints can be overridden by `CODEX_REFRESH_TOKEN_URL_OVERRIDE` and `CODEX_REVOKE_TOKEN_URL_OVERRIDE`; revoke can also derive from a refresh override (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:93-97`, `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/revoke.rs:150-168`).

Unauthorized recovery is only for auth failures. For managed ChatGPT auth, Codex can reload auth from disk and then refresh OAuth tokens. For external ChatGPT tokens, it asks the external auth provider. For external bearer auth, it reruns the provider command (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:1057-1075`, `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:1184-1241`). HTTP and WebSocket response loops retry on 401 after recovery; other errors are returned (`/tmp/codex-src-0.133.0/codex-rs/core/src/client.rs:1216-1311`, `/tmp/codex-src-0.133.0/codex-rs/core/src/client.rs:1342-1412`).

## Request Headers

Bearer auth attaches:

- `Authorization: Bearer <redacted>`
- `ChatGPT-Account-ID: <redacted>` when an account ID exists
- `X-OpenAI-Fedramp: true` for FedRAMP accounts

Source: `/tmp/codex-src-0.133.0/codex-rs/model-provider/src/bearer_auth_provider.rs:31-46`. The same bearer routing headers are present in the `rust-v0.135.0` snapshot at `/tmp/codex-src-0.135.0/codex-rs/model-provider/src/bearer_auth_provider.rs:31-42`.

Codex's default originator is `codex_cli_rs`; `rust-v0.135.0` defines it in `/tmp/codex-src-0.135.0/codex-rs/login/src/auth/default_client.rs:36`. The user-agent helper builds an originator/version/platform string from `CARGO_PKG_VERSION` at `/tmp/codex-src-0.135.0/codex-rs/login/src/auth/default_client.rs:133-145`. The models manager derives the whole client version from `CARGO_PKG_VERSION_MAJOR`, `CARGO_PKG_VERSION_MINOR`, and `CARGO_PKG_VERSION_PATCH` at `/tmp/codex-src-0.135.0/codex-rs/models-manager/src/lib.rs:19-26`, and the Codex models endpoint appends `client_version` at `/tmp/codex-src-0.135.0/codex-rs/codex-api/src/endpoint/models.rs:35-53`.

Agent identity attaches:

- `Authorization: AgentAssertion <redacted-envelope>`
- `ChatGPT-Account-ID: <redacted>`
- `X-OpenAI-Fedramp: true` when applicable

Source: `/tmp/codex-src-0.133.0/codex-rs/model-provider/src/auth.rs:21-49`. The assertion is signed over `agent_runtime_id`, `task_id`, timestamp, and a private-key signature (`/tmp/codex-src-0.133.0/codex-rs/agent-identity/src/lib.rs:106-126`). Agent task registration uses the agent identity auth API base, defaulting to `https://auth.openai.com/api/accounts` or `CODEX_AGENT_IDENTITY_AUTHAPI_BASE_URL`, and calls `/v1/agent/{runtime}/task/register` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/agent_identity.rs:20-28`, `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/agent_identity.rs:64-70`, `/tmp/codex-src-0.133.0/codex-rs/agent-identity/src/lib.rs:304-307`). JWKS is derived from the ChatGPT base URL and is under `/wham/agent-identities/jwks` for backend-api URLs or `/agent-identities/jwks` otherwise (`/tmp/codex-src-0.133.0/codex-rs/agent-identity/src/lib.rs:314-321`).

Requests are built from provider default headers, extra headers, body, and then `auth.apply_auth(req)` before transport execution (`/tmp/codex-src-0.133.0/codex-rs/codex-api/src/endpoint/session.rs:47-60`, `/tmp/codex-src-0.133.0/codex-rs/codex-api/src/endpoint/session.rs:96-109`, `/tmp/codex-src-0.133.0/codex-rs/codex-api/src/endpoint/session.rs:137-150`).

## Provider Routing

Built-in providers are OpenAI, Amazon Bedrock, Ollama, and LM Studio. Source comments say Codex intentionally does not bundle a list of third-party hosted providers; users add their own `model_providers` entries in `config.toml` (`/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:408-430`). Ollama and LM Studio are unauthenticated local OSS providers by default, using `CODEX_OSS_PORT` or `CODEX_OSS_BASE_URL` for the base URL (`/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:475-514`).

`ModelProviderInfo` supports `base_url`, `env_key`, `experimental_bearer_token`, command-backed `auth`, `aws`, query params, static headers, environment-derived headers, retry/timeout fields, `requires_openai_auth`, and WebSocket support (`/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:81-136`). Provider validation rejects incompatible combinations such as AWS plus bearer/env/command/OpenAI auth, and command auth plus env/literal/OpenAI auth (`/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:148-207`).

OpenAI base routing depends on auth mode: ChatGPT, external ChatGPT tokens, and agent identity default to the ChatGPT Codex backend, while API-key mode defaults to `https://api.openai.com/v1`; provider `base_url` overrides this (`/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:236-267`).

Command-backed provider auth runs a configured command with args and cwd, reads trimmed UTF-8 stdout as the bearer token, caches it by refresh interval, and reruns it on refresh (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/external_bearer.rs:30-76`, `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/external_bearer.rs:105-160`). A generic router can emulate this cleanly: each upstream credential source should be its own provider-scoped credential resolver with explicit cache and refresh semantics.

## Amazon Bedrock and AWS

The Bedrock provider is special-cased. It either uses `AWS_BEARER_TOKEN_BEDROCK` with a configured region or uses AWS SDK config and SigV4 (`/tmp/codex-src-0.133.0/codex-rs/model-provider/src/amazon_bedrock/auth.rs:22-57`). The bearer-token path requires `model_providers.amazon-bedrock.aws.region` (`/tmp/codex-src-0.133.0/codex-rs/model-provider/src/amazon_bedrock/auth.rs:66-74`).

SigV4 signing strips headers containing underscores before signing, prepares the final request body, signs method/url/headers/body, replaces headers/url/body, and disables compression (`/tmp/codex-src-0.133.0/codex-rs/model-provider/src/amazon_bedrock/auth.rs:88-139`). The default Mantle service is `bedrock-mantle`; default region-aware URLs look like `https://bedrock-mantle.{region}.api.aws/openai/v1` (`/tmp/codex-src-0.133.0/codex-rs/model-provider/src/amazon_bedrock/mantle.rs:9-60`). AWS SDK config uses the default chain plus optional profile and region (`/tmp/codex-src-0.133.0/codex-rs/aws-auth/src/config.rs:9-38`, `/tmp/codex-src-0.133.0/codex-rs/aws-auth/src/lib.rs:79-110`).

## MCP Auth and OAuth

MCP auth is separate from model-provider auth. MCP OAuth credential store modes are `Auto`, `File`, and `Keyring`; file mode uses `CODEX_HOME/.credentials.json` (`/tmp/codex-src-0.133.0/codex-rs/config/src/types.rs:100-113`). In local development builds, keyring/auto can be forced to file (`/tmp/codex-src-0.133.0/codex-rs/core/src/config/mod.rs:229-240`).

An MCP server config can include OAuth client ID/resource/scopes, HTTP headers, environment-derived headers, and a `bearer_token_env_var` for streamable HTTP servers (`/tmp/codex-src-0.133.0/codex-rs/config/src/mcp_types.rs:117-190`, `/tmp/codex-src-0.133.0/codex-rs/config/src/mcp_types.rs:211-230`). Streamable HTTP resolves bearer tokens from the named environment variable and rejects missing/empty/non-Unicode values (`/tmp/codex-src-0.133.0/codex-rs/codex-mcp/src/rmcp_client.rs:421-446`).

MCP auth status checks bearer env vars, default `Authorization` headers, stored OAuth tokens, and OAuth discovery (`/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/auth_status.rs:29-60`). Discovery uses RFC8414-style well-known paths and requires authorization and token endpoints (`/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/auth_status.rs:81-192`).

MCP OAuth login starts a local callback, supports configured or dynamic client ID, adds `resource` when configured, opens a browser, handles the callback, and stores `StoredOAuthTokens` (`/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/perform_oauth_login.rs:173-255`, `/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/perform_oauth_login.rs:357-581`, `/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/perform_oauth_login.rs:604-637`). If the callback URL is non-localhost, the server binds `0.0.0.0`, which is a meaningful exposure change (`/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/perform_oauth_login.rs:427-440`).

MCP stored tokens contain server name, URL, client ID, token response, and expiry. Keyring service is `Codex MCP Credentials`; file fallback stores plaintext access/refresh tokens in `.credentials.json`, with Unix mode `0600` (`/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/oauth.rs:53-64`, `/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/oauth.rs:371-460`, `/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/oauth.rs:517-581`). MCP OAuth refresh uses a 30-second expiry skew and persists changed credentials (`/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/oauth.rs:252-369`, `/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/oauth.rs:477-515`).

## Plugins, Apps, and Connectors

Plugin configs are mostly policy overlays; plugin MCP server definitions normalize into ordinary `McpServerConfig` (`/tmp/codex-src-0.133.0/codex-rs/config/src/types.rs:791-839`, `/tmp/codex-src-0.133.0/codex-rs/core-plugins/src/loader.rs:1033-1083`). That means plugin-supplied MCP auth follows the MCP bearer/OAuth behavior above.

Remote plugin catalog operations require ChatGPT-backend-capable auth. `ensure_chatgpt_auth` rejects missing or incompatible auth, and authenticated requests reuse `auth_provider_from_auth`, so ChatGPT bearer headers or agent assertions are reused (`/tmp/codex-src-0.133.0/codex-rs/core-plugins/src/remote.rs:1402-1419`).

Connector/app discovery also uses ChatGPT authenticated requests. Connector listing builds ChatGPT GET requests after resolving connector auth (`/tmp/codex-src-0.133.0/codex-rs/chatgpt/src/connectors.rs:94-116`), and install URLs are ChatGPT app URLs such as `https://chatgpt.com/apps/{slug}/{connector_id}` (`/tmp/codex-src-0.133.0/codex-rs/connectors/src/lib.rs:425-428`).

## App-Server and Remote TUI Auth

Codex has a separate app-server WebSocket control plane. It supports `--ws-auth capability-token` with a token file or SHA-256 digest, and `--ws-auth signed-bearer-token` with an HS256 shared-secret file plus optional issuer/audience/max-clock-skew validation (`/tmp/codex-src-0.133.0/codex-rs/app-server-transport/src/transport/auth.rs:27-86`, `/tmp/codex-src-0.133.0/codex-rs/app-server-transport/src/transport/auth.rs:137-264`). Upgrade authorization reads `Authorization: Bearer <redacted>`, then either compares the token SHA-256 in constant time or validates the signed JWT claims (`/tmp/codex-src-0.133.0/codex-rs/app-server-transport/src/transport/auth.rs:273-386`).

A non-loopback WebSocket listener is refused unless WebSocket auth is configured (`/tmp/codex-src-0.133.0/codex-rs/app-server-transport/src/transport/websocket.rs:129-141`). The interactive client can connect to a remote app-server endpoint and read the bearer token from an environment variable named by `--remote-auth-token-env` (`/tmp/codex-src-0.133.0/codex-rs/cli/src/main.rs:752-764`). Remote auth tokens are only allowed with `wss://` or loopback `ws://`, and the client attaches `Authorization: Bearer <redacted>` (`/tmp/codex-src-0.133.0/codex-rs/app-server-client/src/remote.rs:675-709`). This control-plane auth should not be mixed with upstream LLM credentials in a router.

## Rate Limits and Error Handling

Provider retry config disables automatic 429 retry by default while enabling 5xx and transport retries (`/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:251-257`). The retry policy only retries 429 when `retry_429` is true, and it uses exponential backoff with jitter rather than provider account failover (`/tmp/codex-src-0.133.0/codex-rs/codex-client/src/retry.rs:22-73`).

Codex parses 429 bodies and headers into structured errors. `usage_limit_reached` becomes `UsageLimitReached` with active-limit and reset metadata; `usage_not_included` becomes `UsageNotIncluded`; other 429s become retry-limit errors (`/tmp/codex-src-0.133.0/codex-rs/codex-api/src/api_bridge.rs:80-140`). Rate-limit headers include primary/secondary used percent, window, reset time, credits, promo messages, and active limit IDs (`/tmp/codex-src-0.133.0/codex-rs/codex-api/src/rate_limits.rs:21-179`, `/tmp/codex-src-0.133.0/codex-rs/codex-api/src/rate_limits.rs:205-217`).

Streaming responses emit `codex.rate_limits` events before normal SSE events (`/tmp/codex-src-0.133.0/codex-rs/codex-api/src/sse/responses.rs:29-70`). SSE `response.failed` with code `rate_limit_exceeded` can become a retryable API error when a delay can be parsed from the message (`/tmp/codex-src-0.133.0/codex-rs/codex-api/src/sse/responses.rs:487-510`). Internally, rate-limit events are recorded for session state and later token-count output (`/tmp/codex-src-0.133.0/codex-rs/core/src/session/turn.rs:2002-2007`).

No reviewed path switches accounts, rotates identities, or changes providers on 429. Codex only has automatic auth recovery on 401.

## Dynamic Auth-Related Endpoints and Overrides

Important defaults and overrides:

- ChatGPT backend base: `https://chatgpt.com/backend-api` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:93-97`).
- OAuth issuer default: `https://auth.openai.com` (`/tmp/codex-src-0.133.0/codex-rs/login/src/server.rs:54-57`).
- OAuth refresh/revoke overrides: `CODEX_REFRESH_TOKEN_URL_OVERRIDE`, `CODEX_REVOKE_TOKEN_URL_OVERRIDE` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:93-97`).
- Agent identity auth API override: `CODEX_AGENT_IDENTITY_AUTHAPI_BASE_URL`; default `https://auth.openai.com/api/accounts` (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/agent_identity.rs:10-12`, `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/agent_identity.rs:64-70`).
- Agent identity task registration uses the agent identity auth API base, while JWKS is derived from the ChatGPT base URL (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth/agent_identity.rs:20-28`, `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/agent_identity.rs:64-70`, `/tmp/codex-src-0.133.0/codex-rs/agent-identity/src/lib.rs:304-321`).
- OpenAI provider base: API-key mode defaults to `https://api.openai.com/v1`; ChatGPT/agent modes default to `https://chatgpt.com/backend-api/codex`; provider `base_url` overrides (`/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:35-45`, `/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:236-267`).
- Bedrock Mantle URL is region-derived (`/tmp/codex-src-0.133.0/codex-rs/model-provider/src/amazon_bedrock/mantle.rs:41-60`).
- MCP callback URL and port are configurable, with binding behavior changing for non-localhost callbacks (`/tmp/codex-src-0.133.0/codex-rs/core/src/config/mod.rs:752-774`, `/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/perform_oauth_login.rs:357-440`).

## Security Boundaries

Treat these as hard boundaries for a router design:

- Do not read or reuse Codex local credential files directly. If interop is needed, use explicit user import/export flows with clear scope and redaction.
- File-backed `auth.json` and MCP `.credentials.json` are plaintext at rest, protected by file permissions only. Prefer OS keyring or a purpose-built encrypted store.
- `AgentIdentity` stores a private key and signs `AgentAssertion` headers. That private key is more than a passive bearer token; compromise enables task assertions for that runtime/account.
- `experimental_bearer_token` stores a literal token in provider config and is explicitly discouraged in favor of env vars (`/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:96-99`).
- MCP `http_headers` can embed literal secrets; `env_http_headers` and `bearer_token_env_var` are safer because config files do not hold the secret value (`/tmp/codex-src-0.133.0/codex-rs/config/src/mcp_types.rs:211-230`, `/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/utils.rs:60-116`).
- OAuth callbacks and success URLs must not leak token-bearing query strings to shared logs, remote redirects, terminal transcripts, or analytics.
- Shared cookie jars are only safe for narrowly allowlisted infrastructure cookies. User/session/auth cookies must be scoped per account and per auth context.
- Control-plane auth for a router UI/API is separate from upstream model-provider auth. Do not let an OpenAI-compatible client bearer token double as an administrative app-server token.
- Codex emits auth environment presence metadata, not secret values: whether OpenAI/Codex API-key env vars are present, whether the Codex API-key env path is enabled, whether a provider env key is configured/present, and whether the refresh endpoint override is present (`/tmp/codex-src-0.133.0/codex-rs/login/src/auth_env_telemetry.rs:8-42`). Feedback and OTEL paths include those booleans/buckets (`/tmp/codex-src-0.133.0/codex-rs/feedback/src/lib.rs:145-155`, `/tmp/codex-src-0.133.0/codex-rs/otel/src/events/session_telemetry.rs:455-465`). A router should treat even presence metadata as potentially sensitive configuration inventory.
- Switching between accounts to evade rate limits is materially different from availability failover. The local source only proves Codex does not do account/provider rotation on 429; for a router, hidden account cycling should be treated as a security-boundary and provider-policy risk that must be checked against each upstream's terms.

## Implications for a Generic LLM Router

Safe design ideas to copy:

- Model credentials as provider-scoped resolvers: static env key, OAuth token bundle, command-backed token, AWS SigV4 signer, or local unauthenticated provider.
- Keep model-provider auth, MCP/tool auth, and app/plugin auth as separate credential domains.
- Keep control-plane auth for your router's API/TUI separate from upstream provider credentials.
- Store token metadata with account/provider identity, expiry, scopes, refresh state, and last refresh time.
- Normalize outbound OpenAI-compatible requests to `Authorization: Bearer <redacted>`, plus provider-specific headers such as organization/project or account headers only when that provider requires them.
- Surface rate-limit telemetry as first-class state: active limit, reset time, window, used percent, credits, and request IDs.
- Implement 401 recovery separately from 429 handling.
- Support health-based provider failover only when the upstream terms and user configuration allow it.

Risky or likely invalid design ideas:

- Using a consumer ChatGPT subscription OAuth token as a shared OpenAI-compatible backend for unrelated clients. The source shows Codex's OAuth path is built for the Codex client and ChatGPT backend, not as a generic shared inference credential; provider terms must be checked before any router design assumes otherwise.
- Rotating multiple ChatGPT accounts to bypass per-account limits. Codex does not do this; a router should treat it as a quota-evasion risk rather than normal failover.
- Treating agent-identity JWTs or private keys as generic bearer tokens. The source shows signed task assertions and account-bound registration, so replaying them outside that context crosses an auth boundary.
- Scraping Codex credential storage without explicit user consent. Even if technically possible, it is a poor security model and creates hidden coupling to Codex internals.
- Logging full OAuth callback URLs, success URLs, token endpoint bodies, provider command stdout, or MCP token stores.

Recommended router shape:

1. Define a `CredentialProvider` interface with `resolve()`, `refresh(reason)`, `account_id()`, `expires_at()`, and `redacted_debug()`.
2. Define explicit credential types: API key, OAuth authorization-code account, OAuth device account, external bearer command, AWS SigV4, unauthenticated local, and manual bearer token.
3. For a generic router, consider encrypted credential storage and least-privilege scopes per upstream provider/account. Ilonasin's current architecture deliberately uses plaintext SQLite with local file permissions, redaction, and user-visible warnings; database-level encryption is future hardening, not a current requirement.
4. Expose an OpenAI-compatible frontend, but keep internal upstream routing policy transparent: selected provider, selected account, reason for failover, and whether failover is allowed for the error class.
5. For 429, prefer queueing, backoff, user-visible quota state, or switching to a separately paid provider/account only when explicitly configured as legitimate capacity. Do not implement hidden account cycling for quota evasion.
6. Build a TUI around the structured state Codex already hints at: auth status, token expiry, refresh failures, active rate-limit window, reset time, provider health, last request ID, and storage backend.

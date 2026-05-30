# Codex CLI Endpoint Inventory

Date: 2026-05-30

## Scope

This document inventories the network endpoints reachable by the installed `codex` CLI binary on this machine, with source-level confirmation where possible.

Analyzed artifact:

| Item | Value |
| --- | --- |
| CLI version | `codex-cli 0.133.0` |
| Wrapper binary | `/nix/store/dckq80bl9rs9lc17c0p3yamqjxx4j09f-codex-0.133.0/bin/codex` |
| Wrapped ELF payload | `/nix/store/dckq80bl9rs9lc17c0p3yamqjxx4j09f-codex-0.133.0/bin/.codex-wrapped` |
| Nix source tarball | `/nix/store/0847cw4lqhib0y674x8g0qgx9pnwa5h8-codex-x86_64-unknown-linux-musl.tar.gz` |
| Upstream release URL from derivation | `https://github.com/openai/codex/releases/download/rust-v0.133.0/codex-x86_64-unknown-linux-musl.tar.gz` |
| Source tag checked | `rust-v0.133.0` |
| Source commit checked | `9474e5cfc4494b0ba319352aa86ce436c59e65c8` |
| Local source checkout used for evidence | `/tmp/codex-src-0.133.0` |

Method:

- Resolved the installed `codex` wrapper and the wrapped Rust ELF payload.
- Read the Nix derivation to identify the upstream artifact.
- Cross-checked static strings in `.codex-wrapped` against the matching source tag.
- Grepped source for URL constants, request builders, WebSocket builders, OAuth discovery, plugin download paths, updater paths, telemetry, and local listeners.

Limitations:

- Codex supports user-configured model providers, MCP servers, proxies, OTLP exporters, plugin marketplaces, remote plugin bundle URLs, and remote thread-config endpoints. Those are intentionally dynamic; this document describes the URL shapes Codex constructs and the hard-coded defaults.
- User shell commands run by Codex can access arbitrary endpoints. That is outside the Codex binary's own endpoint inventory.
- Dependency/test/documentation URLs that are only examples or docs are excluded unless the binary can actually request them or surface them for browser/system invocation.

## Default Internet Hosts

These are the fixed remote hosts the binary can contact without user-supplied endpoint configuration, depending on feature path:

| Host | Purpose |
| --- | --- |
| `api.openai.com` | OpenAI API-key provider base. |
| `chatgpt.com` | ChatGPT/Codex backend, login browser targets, install/update scripts, usage/settings links, Cyber Safety links. |
| `auth.openai.com` | ChatGPT OAuth login, token exchange, token refresh, token revocation, device login. |
| `bedrock-mantle.<region>.api.aws` | Amazon Bedrock Mantle OpenAI-compatible provider. Default region base is `us-east-1`; supported regions are listed below. |
| `ab.chatgpt.com` | Built-in Statsig OTLP metrics exporter. |
| `o33249.ingest.us.sentry.io` | Feedback upload via Sentry DSN. |
| `api.github.com` | Release checks and curated plugin sync. |
| `github.com` | Curated plugin git clone/fetch and release/source links. |
| `codeload.github.com` | GitHub zipball redirects from `api.github.com` may land here. |
| `raw.githubusercontent.com` | Announcement tooltip fetch. |
| `registry.npmjs.org` | npm package readiness check when installed via npm/bun. |
| `formulae.brew.sh` | Homebrew cask version check when installed via Homebrew. |
| `persistent.oaistatic.com` | Pet spritesheets in this Linux binary; macOS source paths also use it for desktop app DMG downloads. |

## Model Provider API

Provider requests are built from a provider `base_url` plus endpoint paths. Evidence:

- Provider defaults: `/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:35`, `:41`, `:236`, `:243`, `:355`, `:402`, `:475`
- URL construction: `/tmp/codex-src-0.133.0/codex-rs/codex-api/src/provider.rs:52`
- WebSocket conversion: `/tmp/codex-src-0.133.0/codex-rs/codex-api/src/provider.rs:92`

Default provider bases:

| Provider/auth path | Base URL |
| --- | --- |
| OpenAI API key mode | `https://api.openai.com/v1` |
| ChatGPT auth mode | `https://chatgpt.com/backend-api/codex` |
| Amazon Bedrock Mantle default | `https://bedrock-mantle.us-east-1.api.aws/openai/v1` |
| Amazon Bedrock Mantle with region override | `https://bedrock-mantle.<region>.api.aws/openai/v1` |
| Ollama OSS default | `http://localhost:11434/v1` |
| LM Studio OSS default | `http://localhost:1234/v1` |
| Custom provider | User-configured `base_url` |

Amazon Bedrock Mantle supported regions, from `/tmp/codex-src-0.133.0/codex-rs/model-provider/src/amazon_bedrock/mantle.rs:10`: `us-east-2`, `us-east-1`, `us-west-2`, `ap-southeast-3`, `ap-south-1`, `ap-northeast-1`, `eu-central-1`, `eu-west-1`, `eu-west-2`, `eu-south-1`, `eu-north-1`, `sa-east-1`.

Amazon Bedrock AWS SDK credential discovery:

When Bedrock uses AWS SDK SigV4 auth instead of `AWS_BEARER_TOKEN_BEDROCK`, Codex loads `aws_config::defaults(...)` and calls the AWS credentials provider chain. Evidence: `/tmp/codex-src-0.133.0/codex-rs/aws-auth/src/config.rs:14`, `/tmp/codex-src-0.133.0/codex-rs/aws-auth/src/lib.rs:109`, `/tmp/codex-src-0.133.0/codex-rs/model-provider/src/amazon_bedrock/auth.rs:37`. The exact endpoints are environment/profile dependent and owned by the AWS SDK, but binary/source evidence shows these reachable families:

| Purpose | URL shape |
| --- | --- |
| EC2 IMDS credential/region discovery | `http://169.254.169.254/latest/meta-data/iam/security-credentials/`, `http://169.254.169.254/latest/meta-data/placement/region`, IMDSv2 `PUT http://169.254.169.254/latest/api/token` |
| IPv6 EC2 IMDS | `http://[fd00:ec2::254]/...` |
| ECS/container credentials | `http://169.254.170.2{AWS_CONTAINER_CREDENTIALS_RELATIVE_URI}` or `AWS_CONTAINER_CREDENTIALS_FULL_URI` |
| STS assume-role/web-identity flows | `https://sts.amazonaws.com` plus partition, region, FIPS, and dual-stack STS variants selected by AWS config |
| AWS SSO/OIDC/login profile flows | AWS SDK SSO/OIDC/login endpoints selected from profile `sso_region`, `sso_session`, partition, FIPS, and dual-stack config |

Model/provider endpoint paths:

| Method/protocol | Path relative to provider base | Default OpenAI URL | Default ChatGPT URL | Evidence |
| --- | --- | --- | --- | --- |
| `POST` SSE | `/responses` | `https://api.openai.com/v1/responses` | `https://chatgpt.com/backend-api/codex/responses` | `codex-api/src/endpoint/responses.rs:60` |
| WebSocket | `/responses` with `http`/`https` converted to `ws`/`wss` | `wss://api.openai.com/v1/responses` | `wss://chatgpt.com/backend-api/codex/responses` | `codex-api/src/endpoint/responses_websocket.rs:376` |
| `GET` | `/models?client_version=<version>` | `https://api.openai.com/v1/models?client_version=...` | `https://chatgpt.com/backend-api/codex/models?client_version=...` | `codex-api/src/endpoint/models.rs:31` |
| `POST` | `/responses/compact` | `https://api.openai.com/v1/responses/compact` | `https://chatgpt.com/backend-api/codex/responses/compact` | `codex-api/src/endpoint/compact.rs:33` |
| `POST` | `/memories/trace_summarize` | `https://api.openai.com/v1/memories/trace_summarize` | `https://chatgpt.com/backend-api/codex/memories/trace_summarize` | `codex-api/src/endpoint/memories.rs:32` |
| `POST` | `/alpha/search` | `https://api.openai.com/v1/alpha/search` | `https://chatgpt.com/backend-api/codex/alpha/search` | `codex-api/src/endpoint/search.rs:31` |
| `POST` | `/realtime/calls` | `https://api.openai.com/v1/realtime/calls` | `https://chatgpt.com/backend-api/codex/realtime/calls` | `codex-api/src/endpoint/realtime_call.rs:60` |
| WebSocket | normalized realtime path | `wss://api.openai.com/v1/realtime?...` | `wss://chatgpt.com/backend-api/codex/realtime?...` | `codex-api/src/endpoint/realtime_websocket/methods.rs:731` |
| `POST` | `/files` | `https://api.openai.com/v1/files` | `https://chatgpt.com/backend-api/codex/files` | `codex-api/src/files.rs:131` |
| `PUT` | Presigned `upload_url` returned by `/files` | Dynamic upload URL | Dynamic upload URL | `codex-api/src/files.rs:165` |
| `POST` | `/files/{file_id}/uploaded` | `https://api.openai.com/v1/files/{file_id}/uploaded` | `https://chatgpt.com/backend-api/codex/files/{file_id}/uploaded` | `codex-api/src/files.rs:187` |

Realtime WebSocket normalization:

- Empty base path becomes `/v1/realtime`.
- A base ending in `/v1` appends `/realtime`.
- A base already ending in `/realtime` is preserved.
- V1 realtime adds `intent=quicksilver` and `model=<model>` query params where applicable.
- Sideband connections can add `call_id=<id>`.

Azure/OpenAI-compatible custom providers:

- Azure detection is based on user-configured base URL markers such as `openai.azure.`, `cognitiveservices.azure.`, `aoai.azure.`, `azure-api.`, `azurefd.`, and `windows.net/openai`.
- There is no hard-coded Azure endpoint; requests use the configured provider `base_url` plus the same relative paths above.

## Local OSS Providers

The built-in OSS provider default is `http://localhost:<port>/v1`, overridable by `CODEX_OSS_PORT` or `CODEX_OSS_BASE_URL`. Evidence: `/tmp/codex-src-0.133.0/codex-rs/model-provider-info/src/lib.rs:475`.

LM Studio:

| Method | URL shape | Evidence |
| --- | --- | --- |
| `GET` | `{base_url}/models`; default `http://localhost:1234/v1/models` | `lmstudio/src/client.rs:95` |
| `POST` | `{base_url}/responses`; default `http://localhost:1234/v1/responses` | `lmstudio/src/client.rs:64` |

Ollama:

| Method | URL shape | Evidence |
| --- | --- | --- |
| `GET` health probe | If OpenAI-compatible: `{host_root}/v1/models`; else `{host_root}/api/tags` | `ollama/src/client.rs:80` |
| `GET` models | `{host_root}/api/tags`; default host root `http://localhost:11434` | `ollama/src/client.rs:103` |
| `GET` version | `{host_root}/api/version`; default `http://localhost:11434/api/version` | `ollama/src/client.rs:129` |
| `POST` pull | `{host_root}/api/pull`; default `http://localhost:11434/api/pull` | `ollama/src/client.rs:157` |
| Model inference through provider API | `{base_url}/responses`; default `http://localhost:11434/v1/responses` | provider path above |

## ChatGPT Login and OAuth

Default issuer is `https://auth.openai.com`. Evidence:

- Token constants and override environment variables: `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/manager.rs:93`
- Browser OAuth URL construction: `login/src/server.rs:518`
- Token exchange: `login/src/server.rs:729`
- Device code: `login/src/device_code_auth.rs:67`, `:106`, `:159`

Endpoints:

| Method/action | URL |
| --- | --- |
| Open browser | `https://auth.openai.com/oauth/authorize?...` |
| `POST` authorization-code token exchange | `https://auth.openai.com/oauth/token` |
| `POST` API-key token exchange | `https://auth.openai.com/oauth/token` |
| `POST` refresh token | `https://auth.openai.com/oauth/token` |
| `POST` revoke token | `https://auth.openai.com/oauth/revoke` |
| `POST` device user code | `https://auth.openai.com/api/accounts/deviceauth/usercode` |
| `POST` device token poll | `https://auth.openai.com/api/accounts/deviceauth/token` |
| Open browser for device login | `https://auth.openai.com/codex/device` |
| Device redirect URI sent to token exchange | `https://auth.openai.com/deviceauth/callback` |
| Local browser callback listener | `http://localhost:<port>/auth/callback` |
| Local success/cancel pages | `http://localhost:<port>/success?...`, `http://127.0.0.1:<port>/cancel` |

Overrides:

- `CODEX_REFRESH_TOKEN_URL_OVERRIDE` can replace `https://auth.openai.com/oauth/token`.
- `CODEX_REVOKE_TOKEN_URL_OVERRIDE` can replace `https://auth.openai.com/oauth/revoke`.

## ChatGPT/Codex Backend

Default ChatGPT backend base is `https://chatgpt.com/backend-api`. `https://chat.openai.com` and `https://chatgpt.com` bases are normalized to include `/backend-api`. Evidence: `/tmp/codex-src-0.133.0/codex-rs/backend-client/src/client.rs:143`.

The backend client has two path styles:

- If base contains `/backend-api`, use `/wham/...`.
- Otherwise, use `/api/codex/...`.

Endpoints:

| Method | ChatGPT backend URL | Non-`backend-api` base URL shape | Evidence |
| --- | --- | --- | --- |
| `GET` | `https://chatgpt.com/backend-api/wham/usage` | `{base}/api/codex/usage` | `backend-client/src/client.rs:293` |
| `POST` | `https://chatgpt.com/backend-api/wham/accounts/send_add_credits_nudge_email` | `{base}/api/codex/accounts/send_add_credits_nudge_email` | `backend-client/src/client.rs:527` |
| `GET` | `https://chatgpt.com/backend-api/wham/tasks/list?limit=...&task_filter=...&cursor=...&environment_id=...` | `{base}/api/codex/tasks/list?...` | `backend-client/src/client.rs:319` |
| `GET` | `https://chatgpt.com/backend-api/wham/tasks/{task_id}` | `{base}/api/codex/tasks/{task_id}` | `backend-client/src/client.rs:355` |
| `GET` | `https://chatgpt.com/backend-api/wham/tasks/{task_id}/turns/{turn_id}/sibling_turns` | `{base}/api/codex/tasks/{task_id}/turns/{turn_id}/sibling_turns` | `backend-client/src/client.rs:374` |
| `GET` | `https://chatgpt.com/backend-api/wham/config/requirements` | `{base}/api/codex/config/requirements` | `backend-client/src/client.rs:394` |
| `POST` | `https://chatgpt.com/backend-api/wham/tasks` | `{base}/api/codex/tasks` | `backend-client/src/client.rs:411` |

Cloud requirements uses the same config requirements backend endpoint. Evidence: `/tmp/codex-src-0.133.0/codex-rs/cloud-requirements/src/lib.rs:225`.

Additional ChatGPT backend endpoints:

| Method | Default URL | Evidence |
| --- | --- | --- |
| `POST` | `https://chatgpt.com/backend-api/codex/analytics-events/events` | `analytics/src/client.rs:390`; base from `app-server/src/analytics_utils.rs:7` |
| `GET` | `https://chatgpt.com/backend-api/accounts/{encoded_account_id}/settings` | `chatgpt/src/workspace_settings.rs:117`; request join in `chatgpt/src/chatgpt_client.rs:43` |
| `GET` | `https://chatgpt.com/backend-api/connectors/directory/list?external_logos=true` | `connectors/src/lib.rs:201`; ChatGPT fetch in `chatgpt/src/connectors.rs:103` |
| `GET` | `https://chatgpt.com/backend-api/connectors/directory/list?token={encoded_token}&external_logos=true` | `connectors/src/lib.rs:209` |
| `GET` | `https://chatgpt.com/backend-api/connectors/directory/list_workspace?external_logos=true` | `connectors/src/lib.rs:234` |

Cloud Tasks uses `CODEX_CLOUD_TASKS_BASE_URL`, defaulting to `https://chatgpt.com/backend-api`, and normalizes ChatGPT hosts to `/backend-api`. Evidence: `/tmp/codex-src-0.133.0/codex-rs/cloud-tasks/src/lib.rs:49` and `/tmp/codex-src-0.133.0/codex-rs/cloud-tasks/src/util.rs:27`.

Cloud Tasks environment discovery endpoints:

| Method | Backend-api URL | Non-`backend-api` base URL shape | Evidence |
| --- | --- | --- | --- |
| `GET` | `{base}/wham/environments` | `{base}/api/codex/environments` | `cloud-tasks/src/env_detect.rs:70`; `:310` |
| `GET` | `{base}/wham/environments/by-repo/github/{owner}/{repo}` | `{base}/api/codex/environments/by-repo/github/{owner}/{repo}` | `cloud-tasks/src/env_detect.rs:36`; `:266` |

Cloud Tasks display/browser URLs:

| Action | URL shape | Evidence |
| --- | --- | --- |
| Task URL for default base | `https://chatgpt.com/codex/tasks/{task_id}` | `cloud-tasks/src/util.rs:80`; printed in `cloud-tasks/src/lib.rs:177` |
| Task URL for base ending `/api/codex` | `{root}/codex/tasks/{task_id}` | `cloud-tasks/src/util.rs:86` |
| Task URL for base ending `/codex` | `{base}/tasks/{task_id}` | `cloud-tasks/src/util.rs:89` |
| Fallback task URL | `{normalized_base}/codex/tasks/{task_id}` | `cloud-tasks/src/util.rs:92` |

## Agent Identity

Evidence: `/tmp/codex-src-0.133.0/codex-rs/agent-identity/src/lib.rs:128`, `:196`, plus default authapi base in `/tmp/codex-src-0.133.0/codex-rs/login/src/auth/agent_identity.rs:10`.

| Method/action | URL shape |
| --- | --- |
| `GET` JWKS with backend-api base | `{chatgpt_base_url}/wham/agent-identities/jwks`; default `https://chatgpt.com/backend-api/wham/agent-identities/jwks` |
| `GET` JWKS without backend-api base | `{chatgpt_base_url}/agent-identities/jwks` |
| `POST` task registration for agent-identity auth | `https://auth.openai.com/api/accounts/v1/agent/{agent_runtime_id}/task/register` |

Agent identity JWT issuer string is `https://chatgpt.com/codex-backend/agent-identity`; that is a validation issuer value, not an HTTP request URL by itself.

`CODEX_AGENT_IDENTITY_AUTHAPI_BASE_URL` can override the task-registration base. `agent_registration_url()` and `agent_identity_biscuit_url()` helper functions exist in `agent-identity/src/lib.rs`, but no call sites were found in this checkout; they are not counted as observed request endpoints.

## Codex Apps MCP and Connectors

Codex can create a host-owned Streamable HTTP MCP server for ChatGPT apps/connectors. Evidence: `/tmp/codex-src-0.133.0/codex-rs/codex-mcp/src/mcp/mod.rs:415`.

| Use | URL shape |
| --- | --- |
| ChatGPT apps MCP default | `https://chatgpt.com/backend-api/wham/apps` |
| Non-backend-api Codex base | `{base}/api/codex/apps` |
| Base already containing `/api/codex` | `{base}/apps` |
| Override | `{base}/{apps_mcp_path_override}` |

Connector install/auth URLs are constructed for browser/elicitation display as `https://chatgpt.com/apps/{slug}/{connector_id}`. Evidence: `/tmp/codex-src-0.133.0/codex-rs/connectors/src/lib.rs:425`. These are not fetched by the connector code path shown; they are surfaced to the user/client.

`https://api.githubcopilot.com/mcp/` is special-cased for an explanatory auth error if the user configures that MCP server without a token. Evidence: `/tmp/codex-src-0.133.0/codex-rs/codex-mcp/src/connection_manager.rs:721`.

## MCP User-Configured Servers and OAuth Discovery

For user-configured Streamable HTTP MCP servers, Codex contacts the configured `url` directly. Evidence: `/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/rmcp_client.rs:340`.

OAuth discovery for those configured MCP URLs tries RFC 8414 paths on the same origin. Evidence: `/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/auth_status.rs:81`, `:168`.

For configured server URL `https://mcp.example.com/mcp`, candidate discovery paths are:

| Method | URL |
| --- | --- |
| `GET` | `https://mcp.example.com/.well-known/oauth-authorization-server/mcp` |
| `GET` | `https://mcp.example.com/mcp/.well-known/oauth-authorization-server` |
| `GET` | `https://mcp.example.com/.well-known/oauth-authorization-server` |

For an empty root path, only `/.well-known/oauth-authorization-server` is used.

MCP OAuth login also creates a callback listener. By default its redirect URI is `http://127.0.0.1:<port>/callback/<callback_id>` or `http://[::1]:<port>/callback/<callback_id>` depending on bind address; `mcp_oauth_callback_url` can override the public redirect URI. Evidence: `/tmp/codex-src-0.133.0/codex-rs/rmcp-client/src/perform_oauth_login.rs:357`, `:387`, `:408`.

The actual authorization and token endpoints for MCP OAuth are discovered from the remote server metadata, so they are dynamic, not hard-coded.

## Remote Control and App Server Transport

Remote control accepts only HTTPS ChatGPT/chatgpt-staging hosts or loopback HTTP/HTTPS. Evidence: `/tmp/codex-src-0.133.0/codex-rs/app-server-transport/src/transport/remote_control/protocol.rs:134`, `:153`.

| Purpose | URL |
| --- | --- |
| Remote control WebSocket default | `wss://chatgpt.com/backend-api/wham/remote/control/server` |
| Remote control enrollment default | `https://chatgpt.com/backend-api/wham/remote/control/server/enroll` |
| Staging example | `wss://api.chatgpt-staging.com/backend-api/wham/remote/control/server` and `https://api.chatgpt-staging.com/backend-api/wham/remote/control/server/enroll` |
| Local HTTP example | `ws://localhost:<port>/backend-api/wham/remote/control/server` and `http://localhost:<port>/backend-api/wham/remote/control/server/enroll` |
| Local HTTPS example | `wss://localhost:<port>/backend-api/wham/remote/control/server` and `https://localhost:<port>/backend-api/wham/remote/control/server/enroll` |

Codex can also expose local app-server endpoints:

| Direction | URL shape | Evidence |
| --- | --- | --- |
| Local WebSocket listener | `ws://<addr>` | `app-server-transport/src/transport/websocket.rs:60` |
| Readiness endpoint | `http://<addr>/readyz` | `app-server-transport/src/transport/websocket.rs:62` |
| Health endpoint | `http://<addr>/healthz` | `app-server-transport/src/transport/websocket.rs:64` |
| UDS WebSocket handshake marker | `ws://localhost/rpc` | `app-server-client/src/remote.rs:70` |

## Remote Plugins and Marketplaces

Remote plugin service base is the ChatGPT backend base, defaulting to `https://chatgpt.com/backend-api`. Evidence: `/tmp/codex-src-0.133.0/codex-rs/core-plugins/src/remote.rs:91`.

Marketplace/catalog endpoints:

| Method | URL shape | Evidence |
| --- | --- | --- |
| `GET` | `{base}/ps/plugins/list?scope=...&limit=...&collection=...&pageToken=...` | `core-plugins/src/remote.rs:1306` |
| `GET` | `{base}/ps/plugins/workspace/shared?limit=...&pageToken=...` | `core-plugins/src/remote.rs:1328` |
| `GET` | `{base}/ps/plugins/installed?scope=...&includeDownloadUrls=...&pageToken=...` | `core-plugins/src/remote.rs:1344` |
| `GET` | `{base}/ps/plugins/{plugin_id}?includeDownloadUrls=...` | `core-plugins/src/remote.rs:1365` |
| `GET` | `{base}/ps/plugins/{plugin_id}/skills/{skill_name}` | `core-plugins/src/remote.rs:1381` |
| `POST` | `{base}/ps/plugins/{plugin_id}/install` | `core-plugins/src/remote.rs:905` |
| `POST` | `{base}/plugins/{plugin_id}/uninstall` | `core-plugins/src/remote.rs:937` |

Legacy remote plugin endpoints still present in the binary/source:

| Method | URL shape | Evidence |
| --- | --- | --- |
| `GET` | `{base}/plugins/list` | `core-plugins/src/remote_legacy.rs:130` |
| `GET` | `{base}/plugins/featured?platform=...` | `core-plugins/src/remote_legacy.rs:157` |
| `POST` | `{base}/plugins/{plugin_id}/enable` | `core-plugins/src/remote_legacy.rs:197` |
| `POST` | `{base}/plugins/{plugin_id}/uninstall` | `core-plugins/src/remote_legacy.rs:206` |

Workspace share/upload endpoints:

| Method | URL shape | Evidence |
| --- | --- | --- |
| `GET` | `{base}/ps/plugins/workspace/created?limit=...&pageToken=...` | `core-plugins/src/remote/share.rs:377` |
| `POST` | `{base}/public/plugins/workspace/upload-url` | `core-plugins/src/remote/share.rs:393` |
| `PUT` | Presigned `upload_url` returned by backend | `core-plugins/src/remote/share.rs:414` |
| `POST` | `{base}/public/plugins/workspace` | `core-plugins/src/remote/share.rs:444` |
| `POST` | `{base}/public/plugins/workspace/{remote_plugin_id}` | `core-plugins/src/remote/share.rs:444` |
| `DELETE` | `{base}/public/plugins/workspace/{remote_plugin_id}` | `core-plugins/src/remote/share.rs:276` |
| `PUT` | `{base}/ps/plugins/{remote_plugin_id}/shares` | `core-plugins/src/remote/share.rs:297` |

Remote plugin bundle downloads are dynamic HTTPS URLs returned by the backend; Codex validates `https` scheme and downloads the bundle URL directly. Debug builds can allow loopback HTTP for tests. Evidence: `/tmp/codex-src-0.133.0/codex-rs/core-plugins/src/remote_bundle.rs:137`, `:210`, `:267`.

User-added plugin marketplaces can point at arbitrary user-configured git/HTTP(S) sources. GitHub shorthand is normalized to `https://github.com/{owner}/{repo}.git`. Evidence: `/tmp/codex-src-0.133.0/codex-rs/core-plugins/src/marketplace_add/source.rs:120`.

## Curated Plugin Startup Sync

Evidence: `/tmp/codex-src-0.133.0/codex-rs/core-plugins/src/startup_sync.rs:18`, `:590`, `:620`, `:631`.

| Method/tool | URL |
| --- | --- |
| `git clone`/`git fetch` | `https://github.com/openai/plugins.git` |
| `GET` | `https://api.github.com/repos/openai/plugins` |
| `GET` | `https://api.github.com/repos/openai/plugins/git/ref/heads/{default_branch}` |
| `GET` | `https://api.github.com/repos/openai/plugins/zipball/{remote_sha}` |
| Possible redirect target | `https://codeload.github.com/openai/plugins/...` |
| Backup metadata | `https://chatgpt.com/backend-api/plugins/export/curated` |
| Backup archive download | `download_url` returned by the backup metadata endpoint |

## Updates, Installers, and Version Checks

Version checks and update actions are conditional on install method and config (`check_for_update_on_startup`). Evidence:

- Update checks: `/tmp/codex-src-0.133.0/codex-rs/tui/src/updates.rs:65`
- npm package URL: `/tmp/codex-src-0.133.0/codex-rs/tui/src/npm_registry.rs:5`
- TUI update actions: `/tmp/codex-src-0.133.0/codex-rs/tui/src/update_action.rs:39`
- Managed updater fetch: `/tmp/codex-src-0.133.0/codex-rs/app-server-daemon/src/update_loop.rs:157`
- macOS desktop app install: `/tmp/codex-src-0.133.0/codex-rs/cli/src/desktop_app/mac.rs:8`

| Method/action | URL |
| --- | --- |
| `GET` latest GitHub release | `https://api.github.com/repos/openai/codex/releases/latest` |
| `GET` npm package metadata | `https://registry.npmjs.org/@openai%2fcodex` |
| `GET` Homebrew cask metadata | `https://formulae.brew.sh/api/cask/codex.json` |
| Standalone Unix update/install | `https://chatgpt.com/codex/install.sh` |
| Standalone Windows update/install | `https://chatgpt.com/codex/install.ps1` |
| macOS-only desktop DMG, Apple Silicon | `https://persistent.oaistatic.com/codex-app-prod/Codex.dmg` |
| macOS-only desktop DMG, x64 | `https://persistent.oaistatic.com/codex-app-prod/Codex-latest-x64.dmg` |
| Windows-only desktop installer browser/download URL | `https://get.microsoft.com/installer/download/9PLM9XGG6VKS?cid=website_cta_psi` |
| Windows-only desktop Microsoft Store fallback | `https://apps.microsoft.com/detail/9plm9xgg6vks` |

The Nix derivation's upstream release tarball, `https://github.com/openai/codex/releases/download/rust-v0.133.0/codex-x86_64-unknown-linux-musl.tar.gz`, is provenance for the installed package. I did not find evidence that this installed Linux CLI requests that tarball at runtime.

## Assets, Tooltips, Telemetry, and Feedback

| Method/action | URL | Evidence |
| --- | --- | --- |
| `GET` built-in pet spritesheet | `https://persistent.oaistatic.com/codex/pets/v1/{spritesheet_file}`; example `dewey-spritesheet-v4.webp` | `tui/src/pets/asset_pack.rs:29`, `:85` |
| `GET` announcement tooltip TOML | `https://raw.githubusercontent.com/openai/codex/main/announcement_tip.toml` | `tui/src/tooltips.rs:6`, `:209` |
| OTLP metrics export | `https://ab.chatgpt.com/otlp/v1/metrics` with `statsig-api-key` header | `otel/src/config.rs:9` |
| Feedback upload | Sentry DSN `https://ae32ed50620d7a7792c1ce5df38b3e3e@o33249.ingest.us.sentry.io/4510195390611458` | `feedback/src/lib.rs:33`, `:430` |

Codex's OTEL exporters can also be user-configured to arbitrary OTLP HTTP or gRPC endpoints through config. The built-in default metrics exporter is Statsig in release builds; logs/traces default to `None`. Evidence: `/tmp/codex-src-0.133.0/codex-rs/config/src/types.rs:543`.

## Experimental/Dynamic Local and Configured Endpoints

| Feature | URL shape | Evidence |
| --- | --- | --- |
| Remote thread config | User-configured gRPC endpoint from `experimental_thread_config_endpoint` | `config/src/thread_config/remote.rs:39` |
| Custom OTLP HTTP/gRPC exporters | User-configured endpoint and headers | `core/src/otel_init.rs:23` |
| Network proxy | Local proxy URLs such as `http://<addr>` and local SOCKS/HTTP proxy bind addresses | `network-proxy/src/proxy.rs:479` |
| Sandbox proxy environment defaults | Loopback denial/proxy placeholders such as `http://127.0.0.1:9` | `windows-sandbox-rs/src/env.rs:130` |
| User-configured HTTP proxies | Standard `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, etc., from environment/config | `network-proxy/src/config.rs:490` |

These are not fixed remote Codex service endpoints; they depend on local configuration or the user's environment.

## Bundled System Skills

Codex embeds sample/system skills from `codex-rs/skills/src/assets/samples` and installs them into `CODEX_HOME/skills/.system`. Evidence: `/tmp/codex-src-0.133.0/codex-rs/skills/src/lib.rs:10`. These URLs are shipped inside the binary's bundled skill assets; some helper scripts can request them when the corresponding skill is used, while others are instructions/examples surfaced by the skill text.

| Skill/source | URL or URL family | Classification |
| --- | --- | --- |
| `openai-docs` skill | `https://developers.openai.com/api/docs/guides/latest-model.md` | Fetch target for latest-model resolution; evidence `openai-docs/scripts/resolve-latest-model-info.js:7`. |
| `openai-docs` skill | `https://developers.openai.com` | Base URL used by latest-model resolver script; evidence `openai-docs/scripts/resolve-latest-model-info.js:8`. |
| `openai-docs` skill | `https://developers.openai.com/mcp` | MCP server/config/help URL embedded in skill instructions and agent config. |
| `skill-installer` skill | `https://api.github.com/repos/{repo}/contents/{path}?ref={ref}` | GitHub API request built by `skill-installer/scripts/github_utils.py:21`. |
| `skill-installer` skill | `https://codeload.github.com/{owner}/{repo}/zip/{ref}` | GitHub zip download URL built by `skill-installer/scripts/install-skill-from-github.py:81`. |
| `skill-installer` skill | `https://github.com/{owner}/{repo}.git` | Git source URL normalized by `install-skill-from-github.py:180`. |
| `skill-installer` skill | `https://github.com/{repo}/tree/{ref}/{path}` | Display/source URL built by `skill-installer/scripts/list-skills.py:58`. |
| `skill-installer` skill | `https://github.com/openai/skills/tree/main/skills/.curated` | Default curated skill source/listing. |
| `skill-installer` skill | `https://github.com/openai/skills/tree/main/skills/.experimental` | Experimental skill source/listing. |
| `skill-installer` skill | `https://github.com/openai/skills/tree/main/skills/.system` | System skill source/listing. |
| `skill-creator` reference | `https://api.githubcopilot.com/mcp/` | Example MCP URL in bundled reference material. |
| `imagegen` skill | `https://platform.openai.com/api-keys` | API-key setup instruction. |
| `plugin-creator` references | `https://docs.example.com/plugin`, `https://github.com/author`, `https://github.com/author/plugin` | Example plugin metadata URLs in bundled reference material. |
| `plugin-creator` references | `https://openai.com/`, `https://openai.com/policies/row-privacy-policy/`, `https://openai.com/policies/row-terms-of-use/` | Example plugin legal/company URLs in bundled reference material. |
| `plugin-creator` skill | `codex://plugins/{normalized_plugin_name}?marketplacePath={absolute_marketplace_json_path}` and `&mode=share` variant | Codex app plugin view/share deeplinks surfaced in bundled skill instructions. |

## Displayed or Browser-Opened URLs

These URLs appear in the binary and source, but the source paths reviewed either display them, open them in the user's browser, or include them in documentation/error text rather than using Codex's HTTP client directly:

| URL | Use |
| --- | --- |
| `https://chatgpt.com/apps/{slug}/{connector_id}` | Connector install/auth elicitation link. |
| `https://chatgpt.com/codex/tasks/{task_id}` | Cloud Tasks task link for default ChatGPT backend base. |
| `https://chatgpt.com/codex?app-landing-page=true` | Tooltip/app promotion link. |
| `https://chatgpt.com/codex/settings/usage` | Usage/settings link. |
| `https://chatgpt.com/explore/plus` and `https://chatgpt.com/explore/pro` | Plan upgrade/error message links. |
| `https://chatgpt.com/#settings` | Training data preferences link in onboarding text. |
| `https://chatgpt.com/cyber` | Cyber Safety verification link. |
| `https://developers.openai.com/codex/concepts/cyber-safety` | Cyber Safety documentation link. |
| `https://developers.openai.com/codex/security` | Security documentation link. |
| `https://developers.openai.com/codex/cli/` | CLI documentation link surfaced by the CLI. |
| `https://developers.openai.com/codex/mcp` | MCP documentation link surfaced in MCP history/help UI. |
| `https://developers.openai.com/codex/memories` | Memories documentation link surfaced in TUI help. |
| `https://developers.openai.com/codex/config-basic#feature-flags` | Feature-flag documentation link. |
| `https://developers.openai.com/codex/config-advanced/#metrics` | Metrics configuration docs. |
| `https://developers.openai.com/codex/concepts/sandboxing#prerequisites` | Sandboxing prerequisite help. |
| `https://developers.openai.com/mcp` | General MCP documentation link surfaced in tooltips. |
| `https://platform.openai.com` and `https://platform.openai.com/api-keys` | Platform/API-key onboarding help. |
| `https://platform.openai.com/org-setup?...` | Login success page can redirect org owners to platform setup. |
| `https://platform.openai.com/docs/guides/reasoning?api-mode=responses#get-started-with-reasoning` | Reasoning-effort documentation URL emitted in generated schemas and surfaced from protocol docs. |
| `https://platform.openai.com/docs/guides/reasoning?api-mode=responses#reasoning-summaries` | Reasoning-summary documentation URL emitted in generated schemas and surfaced from protocol docs. |
| `https://platform.api.openai.org/org-setup?...` | Same setup redirect when the login issuer is not the default issuer. |
| `codex://threads/new` | Login success page attempts to open the Codex app via custom URL scheme. |
| `codex://plugins/{normalized_plugin_name}?marketplacePath={absolute_marketplace_json_path}` | Plugin-creator skill app handoff deeplink, with optional `&mode=share`. |
| `https://help.openai.com/en/articles/11487775-apps-in-chatgpt` | Apps help article link in plugin UI. |
| `https://github.com/openai/codex/issues/new?template=3-cli.yml` | Issue filing link. |
| `https://github.com/openai/codex` | Installation-options/update fallback link. |
| `https://github.com/openai/codex/releases/latest` | Update prompt fallback/release link. |
| `https://github.com/openai/codex/discussions/7782` | Help link for removed `wire_api = "chat"` and `ollama-chat` provider modes. |
| `https://github.com/settings/personal-access-tokens` | Help text for GitHub MCP auth. |
| `https://go/codex-feedback/{thread_id}` and `http://go/codex-feedback-internal` | OpenAI employee feedback follow-up links. |
| `https://community.openai.com/c/codex/37` | Community link. |
| `http://discord.gg/openai` | Community/Discord tooltip link. |
| `https://github.com/ollama/ollama?tab=readme-ov-file#ollama` | Ollama install help. |
| `https://lmstudio.ai/` and `https://lmstudio.ai/download` | LM Studio install help. |
| `https://developers.openai.com/codex/windows` | Windows sandbox setup/help UI. |
| `https://openai.com/index/introducing-gpt-5-5/` and `https://openai.com/index/introducing-gpt-5-4` | Model metadata/reference links shipped in `models-manager/models.json`. |

`https://api.openai.com/profile` and `https://api.openai.com/auth` appear as JWT claim keys parsed from tokens, not as HTTP request targets in the reviewed source.

## Binary String Cross-Check Notes

Static string extraction from `.codex-wrapped` produced the endpoint families above plus many examples, docs, test fixtures, and dependency URLs. Notable strings that were confirmed as real request paths include:

- `https://api.openai.com/v1`
- `https://chatgpt.com/backend-api/codex`
- `https://auth.openai.com/oauth/token`
- `https://auth.openai.com/oauth/revoke`
- `https://bedrock-mantle.us-east-1.api.aws/openai/v1`
- `https://ab.chatgpt.com/otlp/v1/metrics`
- `https://persistent.oaistatic.com/codex/pets/v1`
- `https://api.github.com/repos/openai/codex/releases/latest`
- `https://registry.npmjs.org/@openai%2fcodex`
- `https://formulae.brew.sh/api/cask/codex.json`
- `https://raw.githubusercontent.com/openai/codex/main/announcement_tip.toml`
- `https://chatgpt.com/codex/install.sh`
- `https://chatgpt.com/codex/install.ps1`
- `https://chatgpt.com/backend-api/plugins/export/curated`
- `https://github.com/openai/plugins.git`
- `https://api.githubcopilot.com/mcp/`
- `https://developers.openai.com/api/docs/guides/latest-model.md`
- `https://github.com/openai/skills/tree/main/skills/.curated`
- `https://chatgpt.com/backend-api/codex/analytics-events/events`
- `https://chatgpt.com/backend-api/accounts/{encoded_account_id}/settings`
- `https://chatgpt.com/backend-api/connectors/directory/list`
- `/files`, `/files/{file_id}/uploaded`, and presigned upload URLs from file upload flow

Strings excluded as non-endpoint evidence include language/runtime documentation URLs, JSON schema identifiers, test `example.com` fixtures, protocol documentation links, and dependency source links.

# Ilonasin Architecture

Status: draft architecture plan.

Access date: 2026-05-30. This document captures the high-level design decisions
for `ilonasin`, a local LLM router with compatibility APIs, provider credentials,
OAuth-capable accounts, metadata-only observability, and a polished TUI.

## Product Shape

`ilonasin` is a local daemon and management tool for routing LLM requests across
configured provider instances.

The first product target is not a hosted SaaS. It is a local service that:

- exposes local compatibility APIs for local clients,
- stores mutable credentials and usage metadata in local SQLite owned by the
  daemon,
- uses static TOML config for provider instances and daemon bootstrap,
- supports API-key and OAuth-style provider credentials,
- offers a polished Bubble Tea/Lipgloss TUI for management,
- avoids storing prompts, completions, request bodies, response bodies, tool
  arguments, or raw stream chunks in normal operation.

## Locked Decisions

### Language and Binary

Implementation language: Go.

There is one binary with two subcommands:

```text
ilonasin serve
ilonasin manage
```

`serve` runs the local compatibility API daemon.

`manage` opens the local management TUI.

The TUI is built with Bubble Tea and Lipgloss and should be treated as a
first-class user interface, not as a debug panel.

### Local Home

Default home directory:

```text
~/.ilonasin/
```

Default layout:

```text
~/.ilonasin/
  config.toml
  ilonasin.sqlite
  ilonasin.sqlite-shm
  ilonasin.sqlite-wal
  logs/
  cache/
```

Default path rules:

- If `--config` is provided, load that config file.
- If `ILONASIN_HOME` is set, use it as the home directory.
- Otherwise use `~/.ilonasin`.
- On Unix, create the home directory with mode `0700` where practical.
- On Unix, create config and SQLite files with mode `0600` where practical.

XDG path support is deferred.

### Config

`config.toml` is static runtime configuration.

The TUI must not mutate `config.toml`.

The config defines daemon bootstrap settings and provider instances. Provider
instances are configured by instance ID. The instance `type` selects a built-in
provider class with defaults. Provider class defaults include base URLs, auth
style, endpoint layout, and adapter behavior.

Example shape:

```toml
[server]
bind = "127.0.0.1:11435"

[paths]
data_dir = "~/.ilonasin"
database = "~/.ilonasin/ilonasin.sqlite"
log_dir = "~/.ilonasin/logs"
cache_dir = "~/.ilonasin/cache"

[providers.deepseek]
type = "deepseek"

[providers.openrouter]
type = "openrouter"

[providers.codex]
type = "codex"

# Optional override example.
[providers.codex-dev]
type = "codex"
base_url = "https://chatgpt.com/backend-api/codex"
auth_issuer = "https://auth.openai.com"
```

Provider `type` names are concise. For example, use `type = "codex"`, not
`type = "codex_subscription"`.

`base_url`, auth issuer URLs, and similar fields are optional overrides. They
are not required for built-in provider types when defaults are known.

### SQLite State

SQLite is the mutable source of truth.

SQLite is plaintext. It does not need database-level encryption in the initial
architecture. The security posture relies on local file permissions, redaction,
and clear user warnings.

The daemon owns SQLite reads and writes. `ilonasin manage` is a client of a
local daemon-owned management API and must not read or write SQLite directly.
New management operations should be added on the daemon-owned management API,
not as direct TUI storage calls.

SQLite stores:

- ilonasin local API tokens,
- upstream API keys,
- OAuth access and refresh tokens,
- OAuth refresh failure descriptions from token endpoint error responses,
- OAuth account metadata,
- credential health,
- provider credential records,
- model cache data,
- usage and latency metadata,
- fallback/retry metadata,
- TUI-managed state.

Provider instances themselves are defined by config. Tokens and credentials for
those instances are stored in SQLite.

Adding a new provider instance requires editing `config.toml` and reloading or
restarting. Adding, refreshing, disabling, or deleting credentials for an
existing provider instance is a daemon management operation surfaced through
`ilonasin manage`.

### Local API Auth

Every local API request to `ilonasin serve` must use an ilonasin client token:

```http
Authorization: Bearer <ilonasin_token>
```

Ilonasin client tokens live in SQLite.

Ilonasin client tokens are separate from upstream provider credentials. A client
token must never double as a provider API key, OAuth token, TUI admin token, or
provider bearer token.

### Local API Surface

The local daemon exposes bounded compatibility surfaces for local clients.

OpenAI-compatible routes:

- `GET /models`
- `GET /v1/models`
- `POST /v1/chat/completions`
- streaming chat completions

Responses-compatible routes:

- `POST /responses`
- `POST /v1/responses`

Anthropic-compatible routes:

- `POST /v1/messages`
- `POST /v1/messages/count_tokens`

Unsupported fields should return clear local errors. The daemon should not
silently forward unknown or unsupported fields to providers.

Provider-specific escape hatches can exist, but they should be explicit and
namespaced.

Responses and Anthropic-compatible routes are local compatibility surfaces, not
claims of universal upstream feature parity. They should convert into the local
strict request model or reject unsupported features before provider dispatch.

### Model Addressing

Clients address models as:

```text
<provider_instance_id>/<provider_model_id>
```

Examples:

```text
deepseek/deepseek-v4-pro
openrouter/deepseek/deepseek-v4-pro
codex/gpt-5.5
```

The first path segment identifies a configured provider instance, not a provider
class.

Generic OpenAI-compatible routes may also accept a bare provider model ID as a
compatibility alias when the model cache has exactly one exact match for that
ID. If zero providers match, or more than one provider matches, the request must
fail with `invalid_model`; the router must not guess across providers.

For `openrouter/deepseek/deepseek-v4-pro`:

- provider instance ID: `openrouter`
- provider model ID: `deepseek/deepseek-v4-pro`

For `deepseek/deepseek-v4-pro`:

- provider instance ID: `deepseek`
- provider model ID: `deepseek-v4-pro`

For `codex/gpt-5.5`:

- provider instance ID: `codex`
- provider model ID: `gpt-5.5`

Provider classes are inferred from the configured provider instance.

### Modes and Reasoning Effort

Fast mode, reasoning effort, and similar behavior should be expressed through
request fields, not model suffixes.

The exact field mapping for Codex-style fast mode and reasoning effort is
deferred until deeper Codex/provider research. The architecture should follow
Codex-compatible request semantics where practical.

### Routing

Routing is explicit by default.

The requested provider instance and provider model should be derived from the
client model string. The router should not choose a different provider or model
unless an explicit future route policy allows it.

The router should record metadata for:

- requested provider instance,
- requested model,
- resolved provider instance,
- resolved model,
- credential label or credential ID,
- fallback/retry behavior.

### Credential Pooling

Credential pooling is constrained:

- same provider instance,
- same provider model,
- eligible credentials attached to that provider instance.

No cross-provider fallback by default.

No cross-model fallback by default.

No hidden downgrade or upgrade by default.

Pooling across API keys and OAuth accounts is default for a requested provider
instance and model. It is especially relevant for provider instances where
multiple credentials represent the same provider/model route.

Pooling must remain auditable through metadata-only request, fallback, health,
and quota rows. Fallback-policy rows are operator/display metadata; serving
eligibility is the default same-provider credential pool.

Allowed examples:

- retry the same credential on transient network failure,
- retry the same credential on retryable upstream `5xx`,
- switch to another eligible credential for the same provider instance and same
  model on availability failure before a response is committed,
- switch to another eligible credential for the same provider instance and same
  model on quota pressure before a response is committed,
- use subscription account pooling for Codex OAuth accounts without storing full
  account IDs separately.

Not allowed by default:

- hidden cross-provider fallback,
- hidden cross-model fallback,
- querying provider billing, balances, credits, plan limits, or account
  settings to infer quota,
- retry loops that cycle indefinitely through blocked credentials,
- weakening privacy/routing constraints during fallback.

Quota pooling uses only local quota observations already produced by routed
requests. It may use provider `retry_after` or reset timing when already
available in normalized metadata. If no retry/reset timing exists, the daemon
may apply a short local cooldown to avoid immediately retrying a blocked
credential. Fabricated local cooldowns are not provider reset times and must not
be rendered as such.

Credential affinity should use fields that real clients already send, in this
priority order:

| Client or API | Out-of-box or common signal | Pooling interpretation |
| --- | --- | --- |
| Codex CLI Responses | The audited Codex CLI 0.135 path sets body `prompt_cache_key` from the Codex thread ID. `client_metadata` includes installation metadata, and transport headers may include `session-id`, `thread-id`, `x-client-request-id`, and `x-codex-window-id`. | Prefer safe body `prompt_cache_key`. Use safe `client_metadata` session or thread fields only when the cache key is absent. Use safe `session-id` or `thread-id` headers only as fallback. `x-codex-window-id` is observed window metadata, not a supported credential-affinity fallback. Never use request-id-shaped values as generic affinity. |
| Codex app-server Responses | App-server turn APIs can forward turn-scoped `responsesapi_client_metadata` into Responses `client_metadata`. | Use only selected safe `client_metadata` session, thread, conversation, or prompt-cache keys. Ignore installation, account, device, token, and request-id-shaped values. |
| Claude Code Anthropic | Captured traffic uses Anthropic `metadata.user_id` as a JSON string containing `session_id`, plus `X-Claude-Code-Session-Id`. | Prefer the safe nested `metadata.user_id.session_id`. Use the safe session header only if body affinity is absent. |
| Generic OpenAI Chat | Many clients send only `model` and `messages`. `session_id`, `prompt_cache_key`, `user`, and `metadata` are optional. | Prefer safe `session_id`, then safe top-level `prompt_cache_key`, then safe `user`, then selected safe metadata keys: `session_id`, `thread_id`, `conversation_id`, and `prompt_cache_key`. |
| Generic Responses | `prompt_cache_key` and top-level `metadata` are optional. Many clients may send neither. | Prefer safe body `prompt_cache_key`, then selected safe `metadata` keys: `prompt_cache_key`, `session_id`, `thread_id`, and `conversation_id`. |
| Minimal clients | Often send only model plus a local ilonasin API token. | Route by local token identity, provider instance, provider model, least-in-flight credential pressure, and round-robin tie breaking. |

Affinity is best-effort locality, not a quota or correctness boundary. It must
never require clients to send a session field. When no safe client signal is
available, pooling still spreads traffic across eligible credentials using the
local token, requested route, in-flight pressure, and cursor state.

### Observability and Logging

Ilonasin must not persist request or response bodies in normal operation.

Do not store:

- prompts,
- completions,
- request bodies,
- response bodies,
- raw provider payloads,
- tool arguments,
- raw tool results,
- raw SSE chunks,
- full bearer tokens,
- full provider request IDs,
- full account IDs.

OAuth refresh failure descriptions from token endpoint error objects may be
stored and rendered for account visibility. Do not persist the full token
endpoint response body.

Metadata-only telemetry is allowed and expected.

`[logging].capture_io = true` is the explicit local debugging exception. When
enabled, `ilonasin-io.log` may persist local request bodies, local response
bodies, and streamed event payloads needed to debug wire-shape issues. When the
normal logging level is `debug`, the same IO log may also persist bounded
upstream provider request bodies, response bodies, and streamed provider events
captured at adapter boundaries. It must not persist bearer tokens, local client
tokens, upstream API keys, OAuth tokens, cookies, authorization codes, device
codes, code verifiers, provider command stdout, or configured credential secret
values.

Full upstream account IDs may be derived transiently from credential secrets
when building outbound provider routing headers. They must not be stored
separately, logged, rendered, exposed in management snapshots, written to
request metadata, or stored in normal metadata tables. All observable account
references should use local credential IDs, safe display labels, or one-way
account hashes.

Telemetry fields may include:

- timestamp,
- client token label or ID,
- provider instance,
- provider type,
- credential label or ID,
- requested model,
- resolved model,
- HTTP status,
- normalized error class,
- retry count,
- fallback count,
- fallback reason,
- prompt token count,
- completion token count,
- total token count,
- reasoning token count,
- cache hit and cache write counts,
- estimated or actual cost,
- total latency,
- time to first token,
- output tokens per second,
- stream completion status.

Telemetry retention default: keep forever until pruned.

Manual pruning is available through the daemon-owned management API and TUI.
Scheduled pruning remains a policy question.

Debugging features that capture bodies must remain behind the explicit
`capture_io` switch and must write only to the local IO log.

### Provider Health

Provider and credential health should be tracked separately from request usage.

Health metadata may include:

- last success timestamp,
- last failure timestamp,
- last normalized error class,
- consecutive failure count,
- retry-after timestamp,
- token expiry timestamp,
- refresh failure state,
- refresh failure description,
- credential disabled state,
- provider/model capability observations.

The router can use health metadata to avoid known-bad credentials without
looking at request or response bodies.

## High-Level Runtime Architecture

```text
Local API clients
  -> HTTP API auth
    -> local API request parser/converter
      -> strict request validator
        -> model address resolver
          -> routing policy
            -> credential resolver
              -> provider adapter
                -> upstream provider
```

Side planes:

```text
config.toml
  -> provider instance registry
  -> server/path/bootstrap settings

SQLite
  -> ilonasin client tokens
  -> upstream credentials and OAuth tokens
  -> model cache
  -> health state
  -> metadata-only usage ledger

manage TUI
  -> daemon-owned local management API
  -> SQLite-backed mutation and inspection through the daemon
  -> OAuth/API-key auth flows
  -> usage/health views
```

## Provider Adapter Boundary

Provider adapters own provider-specific behavior.

Adapter responsibilities:

- know default base URLs and endpoint paths for a provider type,
- apply upstream auth,
- translate strict common requests into provider-specific requests,
- reject unsupported features clearly,
- stream provider responses into normalized OpenAI-style chunks,
- normalize provider errors,
- extract token/cost/cache metadata,
- expose provider model discovery,
- expose provider health/account status where safe.

The router core should not embed provider-specific quirks beyond selecting an
adapter and passing typed route options.

Initial provider types:

- `deepseek`
- `openrouter`
- `codex`

Future provider types can be added as adapters.

## Management TUI

`ilonasin manage` is the local control plane.

It should be visually polished and useful for repeated daily operation.

Expected first-class views:

- provider instances from config,
- credential/account list,
- add API key flow,
- OAuth login/refresh flow for OAuth-capable provider types,
- credential health,
- model cache and capability summary,
- recent metadata-only requests,
- usage totals,
- latency/TTFT/TPS summaries,
- retry/fallback events,
- local API token management.

The TUI talks to the daemon-owned local management API for mutable operations.
The daemon performs SQLite reads and writes behind that management boundary.

The TUI does not edit `config.toml`.

The management API should use a local-only internal transport, such as a Unix
domain socket on Unix, and must not be exposed as part of the public
local compatibility API surface.

## Conceptual SQLite Tables

Exact schema is deferred, but the architecture should cover these concepts:

- `client_tokens`: local ilonasin API tokens for local API clients.
- `provider_credentials`: API-key, OAuth, command, or other credentials bound to
  configured provider instance IDs.
- `oauth_tokens`: access/refresh token material, expiry data, refresh failure
  classes, and refresh failure descriptions from token endpoint error
  responses.
  Terminal refresh-token failures, such as reused, invalidated, expired, invalid
  grant, or access denied, make the bearer ineligible until the account is
  reauthed and the refresh failure is cleared.
- `provider_accounts`: account identity metadata from upstream providers.
- `model_cache`: model lists and capability metadata per provider instance.
- `request_metadata`: metadata-only request ledger.
- `stream_metrics`: TTFT, TPS, completion status, and stream timing.
- `health_events`: provider/credential/model health history.
- `fallback_events`: retry and fallback decisions.
- `migrations`: schema migration state.

Do not create tables for raw prompts, completions, request bodies, response
bodies, or raw provider payloads. OAuth refresh failure descriptions are allowed
as extracted error metadata, not as raw token endpoint responses.

## Deferred Research

Do not choose speculative implementation libraries in this architecture
document.

Areas that still need research or stronger live evidence:

- XDG path support.
- Non-Unix management transport.
- Provider adapter test strategy for hosted/deferred/namespaced tool families.
- OpenRouter provider/model behavior for Codex CLI tool-response paths.
- Exact provider-term boundaries for subscription account keepalive and quota
  pooling.

## Open Questions

- What exact request fields should represent Codex fast mode and reasoning
  effort?
- Should local API tokens be one-way hashed in SQLite?
- Should credential records use labels visible in telemetry?
- What is the exact policy for subscription account fallback under provider
  terms?
- How should daemon management transport work on non-Unix platforms?
- Should metadata pruning be manual, scheduled, or both?
- How much local Responses and Anthropic-compatible tool-family parity is
  necessary beyond the currently supported conversion paths?

## MVP Target

The architecture supports an MVP with:

- `ilonasin serve`,
- `ilonasin manage`,
- config-defined provider instances,
- SQLite-backed local API tokens,
- SQLite-backed upstream API keys and OAuth tokens,
- DeepSeek, OpenRouter, and Codex provider adapters,
- `/models`,
- `/v1/models`,
- `/v1/chat/completions`,
- streaming chat completions,
- `/responses`,
- `/v1/responses`,
- `/v1/messages`,
- `/v1/messages/count_tokens`,
- strict request validation,
- same-provider-instance and same-model credential fallback,
- metadata-only usage ledger,
- polished provider/credential/usage TUI.

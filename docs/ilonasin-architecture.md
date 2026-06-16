# Ilonasin Architecture

Status: active target architecture for current implementation work.

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
- only stores prompts, completions, request bodies, response bodies, tool
  arguments, or raw stream chunks when IO logging is enabled.

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
are not required for built-in provider types when defaults are known. Provider
URL overrides must be `https` URLs with a host and must not include userinfo,
query, or fragment components. Path components are allowed for provider bases
such as Codex.

### SQLite State

SQLite is the mutable source of truth.

SQLite is plaintext. It does not need database-level encryption in the initial
architecture. The security posture relies on local file permissions,
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

`Authorization: Bearer <ilonasin_token>` is the primary local auth mechanism.
For Anthropic-compatible local routes only, `X-Api-Key: <ilonasin_token>` is
accepted as a compatibility alias when no `Authorization` header is present.
When both headers are present, `Authorization` takes precedence. This alias
still verifies an ilonasin local client token. It is not an upstream provider
API key, and the full token must not be logged, stored in metadata, or exposed
through management snapshots or the TUI.

Ilonasin client tokens live in SQLite as one-way token hashes plus safe display
fragments. The full token is generated by ilonasin and returned only in the
management create-token response. Later management list/snapshot surfaces and
the TUI expose only token metadata, prefix, and last-four fragments.

Bearer verification hashes the presented token and compares that hash with the
stored token hash. The full token must not be persisted in SQLite, request
metadata, logs, management snapshots, or the TUI.

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

Compatibility routes are strict by default. A provider may also expose a
route-native capability for a specific surface. When a selected provider exposes
native Responses, the `/responses` route may preserve the validated native
request shape and relay native provider SSE without converting through Chat
Completions. When no native capability exists, Responses must convert into the
local strict request model or fail locally before provider dispatch.

Codex native Responses is a route-native capability. Ilonasin may inspect only
auth, model routing, statelessness, coarse safety limits, credential affinity,
and safe metadata fields. It must not reject Codex-native input or output
families solely because they are not representable as Chat Completions.

Chat-adapter provider paths must not silently flatten or forward hosted,
deferred, namespaced, MCP, shell, tool-search, or other unrepresentable tool
families. Unsupported transcript and output families outside implemented relay
paths must fail locally rather than be lossy-converted.

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

For Codex Chat Completions, Codex-specific controls live under explicit provider
options:

- `provider_options.codex.reasoning.effort` for reasoning effort,
- `provider_options.codex.reasoning.summary` for reasoning summary behavior,
- `provider_options.codex.verbosity` for text verbosity,
- `provider_options.codex.service_tier` for Codex-specific service tier
  selection.

Codex Chat also accepts top-level `service_tier` for standard local tiers such
as `default`, `priority`, and `flex`. Provider-specific service tier values
must stay under `provider_options.codex.service_tier`; the local `fast` alias
maps to upstream `priority` when the selected Codex model supports that tier.

For Codex native Responses, provider-specific controls remain in the native
Responses body after local validation of routing, statelessness, and safety
boundaries. For non-native Responses conversion paths, supported controls are
translated into the existing local provider-options contract before dispatch.

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
and quota rows. Credential pool-group listing and metadata are operator display
metadata; serving eligibility is the default same-provider, same-model eligible
credential pool.

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

Credential affinity should start from fields clients actually send, then fall
back to local inputs the daemon always has. The daemon must keep three classes
separate:

- observed named-client behavior, grounded in local source inspection or
  sanitized local captures,
- optional API fields, which SDKs may expose but harnesses often omit,
- local fallback inputs, such as the verified local token identity and resolved
  provider/model route.

`prompt_cache_key` is a preferred signal when present because Codex CLI sends it
in the audited Responses path. It is not a required generic-client field, and
the daemon must not synthesize an affinity key from prompts, messages, input
content, or request bodies when clients omit it. Many harnesses send only the
required model plus message/input content, so pooling must remain useful when
every optional session, user, metadata, or prompt-cache field is absent.

| Client or API | What sends what | Local affinity priority |
| --- | --- | --- |
| Codex CLI Responses | The audited `codex-cli 0.135.0` Responses path sends body `prompt_cache_key` from the Codex thread ID. It also sends `client_metadata` with installation metadata. Transport headers may include `session-id`, `thread-id`, `x-client-request-id`, and `x-codex-window-id`. | Prefer safe body `prompt_cache_key`. Use safe `client_metadata` session, thread, conversation, or prompt-cache fields only when the cache key is absent. Use safe `session-id` or `thread-id` headers only as last-resort stable-session fallbacks. |
| Codex app-server Responses | App-server turn APIs can forward turn-scoped `responsesapi_client_metadata` into Responses `client_metadata`. | Use only selected safe `client_metadata` session, thread, conversation, or prompt-cache keys when top-level `prompt_cache_key` is absent. |
| Claude Code Anthropic | Captured Claude Code `2.1.159` traffic uses Anthropic `metadata.user_id` as a JSON string containing `session_id`, plus `X-Claude-Code-Session-Id`. | Prefer safe nested `metadata.user_id.session_id`, then safe plain `metadata.session_id`. Use the safe session header only after conversion if body affinity is absent. |
| Generic OpenAI Chat | Many clients send only `model` and `messages`. `session_id`, `prompt_cache_key`, `user`, and `metadata` are optional API fields, even when SDKs expose them. | Prefer safe `session_id`, then safe top-level `prompt_cache_key`, then safe `user`, then selected safe metadata keys: `session_id`, `thread_id`, `conversation_id`, and `prompt_cache_key`. |
| Generic Responses | Many clients send only `model` and `input`. `prompt_cache_key`, `client_metadata`, and top-level `metadata` are optional API fields. | Prefer safe body `prompt_cache_key`, then selected safe `client_metadata` keys, then selected safe top-level `metadata` keys: `prompt_cache_key`, `session_id`, `thread_id`, and `conversation_id`. |
| Minimal clients | Often send only model, messages or input, and a local ilonasin API token. | Route by verified local token identity, provider instance, provider model, least-in-flight credential pressure, and token-scoped cursor tie breaking. |

Do not use request-id-shaped values, `x-client-request-id`,
`x-codex-window-id`, installation IDs, account IDs, device IDs, token values,
authorization values, prompts, completions, or tool payloads as generic
credential affinity. `prompt_cache_key` is the preferred cache-locality signal
when a client sends it, but out-of-box pooling must still work for clients that
send only model and message/input content.

Affinity is best-effort locality, not a quota or correctness boundary. It must
never require clients to send a session field. When no safe client signal is
available, pooling still spreads traffic across eligible credentials using the
verified local token identity, requested route, in-flight pressure, and cursor
state.

### Observability and Logging

Ilonasin must not persist request or response bodies unless IO logging is enabled.

Do not store without IO logging:

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
bodies, upstream provider request bodies, upstream provider response bodies,
and streamed event payloads needed to debug wire-shape issues. It must not
persist bearer tokens, local client tokens, upstream API keys, OAuth tokens,
cookies, authorization codes, device codes, code verifiers, provider command
stdout, or configured credential secret values.

IO logging must be bounded by local rotation settings. Defaults should keep a
small retained set of local JSONL debug files, with `ilonasin-io.log` as the
active file and numeric suffixes for older files. Implementations should preserve
complete JSONL records rather than truncating oversized records, so the practical
disk bound is the retained file count times the larger of the rotation threshold
or the largest single retained encoded record. Management and TUI surfaces may
show only safe retention policy metadata, not IO log paths or payload content.

Full upstream account IDs may be derived transiently from credential secrets
when building outbound provider routing headers. They must not be stored
separately, logged, rendered, exposed in management snapshots, written to
request metadata, or stored in normal metadata tables. All observable account
references should use local credential IDs, safe display labels, or one-way
account hashes.

Durable request, health, fallback, and quota metadata rows should reference
upstream credentials by local credential ID. Management summaries and TUI views
may display current safe credential labels by joining credential metadata at
read time. Those labels are operator display metadata only: they must be
sanitized before snapshots or TUI rendering, and they must not be treated as
credential selectors, account IDs, bearer tokens, or durable secret copies.

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

Manual pruning is available through the daemon-owned management API and TUI. The
current TUI operation prunes metadata-only request, stream, fallback, health, and
quota rows older than 30 days. Scheduled pruning and configurable retention
durations are not part of the current architecture; they would need a separate
retention-policy design before implementation.

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

Native route relays follow the same metadata-only rule. Raw native request
bodies, provider payloads, streamed events, tool arguments, and tool results
must not be stored unless IO logging is enabled. Normal native relay telemetry
may record provider, model, credential, status, latency, usage, retry, quota,
and safe event-class metadata.

## High-Level Runtime Architecture

```text
Local API clients
  -> HTTP API auth
    -> route preflight and model address resolver
      -> provider route capability selection
        -> native route capability
          -> credential resolver
            -> provider-native request/stream relay
        -> compatibility parser/converter
          -> strict request validator
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
- declare supported route capabilities: chat, responses, models, and
  compatibility surfaces where applicable,
- apply upstream auth,
- translate strict common requests or relay validated provider-native requests
  through explicit route capabilities,
- reject unsupported features clearly,
- stream provider responses into normalized OpenAI-style chunks,
- normalize provider errors,
- extract token/cost/cache metadata,
- keep model discovery independent from chat completion support,
- parse only safe scalar metadata from native streams unless IO logging is
  explicitly enabled,
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

The current management API transport is local-only HTTP over a Unix-domain socket
under the selected home runtime directory. The socket path is derived from the
selected home, config path, and database path identity; the runtime directory and
socket file are permission-tightened where Unix permissions apply. Management
routes must not be exposed as part of the public local compatibility API surface.
Non-Unix management transport remains deferred research.

## SQLite Tables

Concrete schema details live in SQLite migrations. This architecture names the
durable state boundaries those migrations must preserve:

- `client_tokens`: local ilonasin API tokens for local API clients.
- `provider_credentials`: API-key, OAuth, command, or other credentials bound to
  configured provider instance IDs.
- `credential_secrets`: local secret material for upstream provider
  credentials, referenced by credential metadata and never exposed through
  management snapshots or the TUI.
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
- `quota_events`: metadata-only quota observations, retry-after values, and
  reset timing linked to request metadata where available.
- `subscription_usage_snapshots`: metadata-only subscription quota snapshots for
  OAuth-capable accounts, including usage percentages, window sizes, reset
  times, stale/error state, and safe account display labels.
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
- Provider adapter test strategy for native route capabilities and tool families
  not yet covered by live Codex, OpenRouter, or DeepSeek evidence.
- OpenRouter provider/model behavior for Codex CLI tool-response paths.
- Exact provider-term policy for subscription account keepalive, quota pooling,
  and any subscription account fallback.

## Current Implemented Surface

The current implemented product surface includes:

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

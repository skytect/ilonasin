# Codex Cyber-Use Verification Research

Access date: 2026-06-07.

This document records source-backed research into whether ilonasin can route
Codex OAuth traffic to an account with cyber-use verification or Trusted Access
for Cyber.

It does not inspect local credential files, live account state, browser state,
cookies, tokens, or private account data.

## Sources Reviewed

- Installed CLI: `codex-cli 0.137.0`.
- Source snapshot:
  `/tmp/codex-src-0.137.0/codex`, commit
  `f221438b691b8f749d98f22077c93ebe01923fbe`.
- Official Codex manual:
  `https://developers.openai.com/codex/codex-manual.md`, section source
  `/codex/concepts/cyber-safety.md`, accessed 2026-06-07.
- Existing ilonasin architecture and Codex-auth docs:
  `docs/ilonasin-architecture.md`, `docs/codex-auth.md`, and
  `docs/codex-endpoints.md`.

## Direct Answer

Ilonasin cannot safely route by "account has cyber-use verification" with the
current evidence.

The reviewed Codex source exposes:

- a per-turn recommendation that more account verification is recommended;
- a per-request/model reroute signal for high-risk cyber activity;
- a hard `cyber_policy` block signal.

The reviewed source does not expose a stable positive account-level field such
as `trusted_access_for_cyber = true`, `cyber_verified = true`, or an account
capability endpoint that proves Trusted Access is active. Absence of warnings,
reroutes, or `cyber_policy` errors is therefore unknown state, not verified
state.

The safe near-term routing behavior is "do not treat this credential as proven
trusted"; optionally prefer credentials with no recent mitigation observations
or avoid credentials recently rerouted/blocked, as long as that is presented as
observed mitigation history rather than proof of verification.

## Proven Codex Signals

### Verification Recommendation

Codex parses only one cyber-verification recommendation value from Responses SSE
metadata: `trusted_access_for_cyber`
(`/tmp/codex-src-0.137.0/codex/codex-rs/codex-api/src/sse/responses.rs:27`).
The parser reads `metadata.openai_verification_recommendation` only on
`response.metadata` events, and only when it is an array
(`/tmp/codex-src-0.137.0/codex/codex-rs/codex-api/src/sse/responses.rs:186-195`,
`:210-239`).

When parsed, Codex emits `ResponseEvent::ModelVerifications`
(`/tmp/codex-src-0.137.0/codex/codex-rs/codex-api/src/sse/responses.rs:446-463`).
The protocol describes the resulting event as "Backend recommends additional
account verification for this turn"
(`/tmp/codex-src-0.137.0/codex/codex-rs/protocol/src/protocol.rs:1183-1187`).
The only enum value is `ModelVerification::TrustedAccessForCyber`
(`/tmp/codex-src-0.137.0/codex/codex-rs/protocol/src/protocol.rs:1841-1850`).

Codex emits this recommendation once per turn
(`/tmp/codex-src-0.137.0/codex/codex-rs/core/src/session/turn.rs:2038-2044`).
Tests confirm the recommendation emits a structured event without model reroute
or warning, and only once per turn
(`/tmp/codex-src-0.137.0/codex/codex-rs/core/tests/suite/safety_check_downgrade.rs:298-356`,
`:358-410`).

Interpretation: this is a backend recommendation that the current turn/account
context needs additional verification. It is not proof that the account is
verified.

### High-Risk Cyber Reroute

Codex compares the requested model with the server-reported model. If they
differ, Codex treats that mismatch as a reroute signal and emits `ModelReroute`
with reason
`ModelRerouteReason::HighRiskCyberActivity` and a warning pointing to
`https://chatgpt.com/cyber`
(`/tmp/codex-src-0.137.0/codex/codex-rs/core/src/session/mod.rs:2588-2616`).
The protocol enum contains only that reroute reason
(`/tmp/codex-src-0.137.0/codex/codex-rs/protocol/src/protocol.rs:1827-1838`).

Tests show a server model header mismatch from `gpt-5.3-codex` to `gpt-5.2`
emits that high-risk cyber reroute event
(`/tmp/codex-src-0.137.0/codex/codex-rs/core/tests/suite/safety_check_downgrade.rs:67-107`).

Interpretation: this is an observed request-level reroute signal as represented
by Codex. It is not independent proof of all backend mitigation details, and it
does not prove the account is unverified in a durable way.

### Cyber Policy Block

Responses SSE `response.failed` events with error code `cyber_policy` map to
`ApiError::CyberPolicy`
(`/tmp/codex-src-0.137.0/codex/codex-rs/codex-api/src/sse/responses.rs:312-327`,
`:529-545`). Non-stream HTTP 400 bodies with the same error code also map to
`CodexErr::CyberPolicy`
(`/tmp/codex-src-0.137.0/codex/codex-rs/codex-api/src/api_bridge.rs:59-72`,
`:145-147`).

The protocol marks `CyberPolicy` as non-retryable and maps it to
`CodexErrorInfo::CyberPolicy`
(`/tmp/codex-src-0.137.0/codex/codex-rs/protocol/src/error.rs:116-117`,
`:188-195`, `:220-228`).

Interpretation: this is a hard request-level policy block. It is safe to persist
metadata that a credential/request observed `cyber_policy`, but it must not be
treated as a general verified/unverified account classification.

## User-Facing Codex Notices

The TUI warning for model verification says conversations have multiple possible
cybersecurity flags and links Trusted Access for Cyber
(`/tmp/codex-src-0.137.0/codex/codex-rs/tui/src/chatwidget.rs:195-201`,
`/tmp/codex-src-0.137.0/codex/codex-rs/tui/src/chatwidget/turn_runtime.rs:449-455`).
Cyber-policy notices also point to `https://chatgpt.com/cyber`
(`/tmp/codex-src-0.137.0/codex/codex-rs/tui/src/history_cell/notices.rs:88-145`).

These notices support operator guidance, not account-state proof.

## Auth and Profile Claims

Codex token parsing stores email, plan type, user ID, account ID, FedRAMP flag,
and the raw JWT
(`/tmp/codex-src-0.137.0/codex/codex-rs/login/src/token_data.rs:27-69`,
`:71-160`). The serialized auth shape stores API key, token data, refresh time,
and agent identity
(`/tmp/codex-src-0.137.0/codex/codex-rs/login/src/auth/storage.rs:31-48`).
Agent identity records similarly include runtime ID, private key, account ID,
user ID, email, plan type, and FedRAMP flag
(`/tmp/codex-src-0.137.0/codex/codex-rs/login/src/auth/storage.rs:50-80`).

No reviewed token/profile field establishes cyber verification or Trusted Access
status. Plan tier, email, account ID, and FedRAMP are separate account metadata.

## Official Codex Manual Context

The official Codex manual says GPT-5.3-Codex is treated as High cybersecurity
capability, and classifier-based monitors may route high-risk traffic to GPT-5.2
(`/codex/concepts/cyber-safety.md`, lines 1178-1180 in the fetched manual).
It says accounts impacted by mitigations can regain access by joining Trusted
Access, with identity verification at `https://chatgpt.com/cyber`
(`/codex/concepts/cyber-safety.md`, lines 1192-1209). It also says rerouting is
visible in API request logs and in-product notices
(`/codex/concepts/cyber-safety.md`, line 1215).

The manual supports the existence of account-level Trusted Access as a product
program, but it does not document a machine-readable positive verification
field for routers.

## State Model for Ilonasin

Use a conservative observed-state model:

| State | Meaning | Source |
| --- | --- | --- |
| `unknown` | No current local evidence. This is the default and is not verified. | Absence of explicit source-backed signal |
| `verification_recommended` | Backend recommended Trusted Access verification for a turn. | `ModelVerification::TrustedAccessForCyber` |
| `mitigated_rerouted` | Codex observed a server model mismatch and represented it as high-risk cyber reroute. | `ModelRerouteReason::HighRiskCyberActivity` |
| `policy_blocked` | Backend returned `cyber_policy`. | `CodexErr::CyberPolicy` |
| `trusted_access_observed` | Reserved only for a future explicit positive source. | Not currently established |

Do not infer `trusted_access_observed` from:

- absence of recommendation, reroute, or block;
- successful requests;
- availability of `gpt-5.3-codex` in a model list;
- plan tier;
- email domain;
- account ID format;
- subscription usage windows;
- local operator labels.

## Persistence Recommendation

If ilonasin later implements cyber-safety-aware routing, persist only
metadata-only observations keyed by local credential ID:

- observed state from the table above;
- model requested and model served, if already allowed as metadata;
- last observed time;
- source route or event class;
- optional counters by state.

Do not persist prompts, completions, request bodies, raw response bodies, raw SSE
chunks, or raw provider payloads unless the request has explicit IO logging
enabled and the existing IO-retention policy permits it. Even with IO logging
enabled, bearer tokens, OAuth tokens, ID tokens, refresh tokens, cookies,
authorization codes, device codes, credential secrets, full account IDs, and
account emails remain credential or account secrets and must not be persisted in
normal metadata or IO logs.

Credential metadata refresh may update these observations from ordinary routed
traffic. It should not probe private account endpoints or perform live Trusted
Access checks unless a documented source-backed endpoint exists and the operator
has explicitly enabled that behavior.

## Future Implementation Boundary

A clean future implementation should keep the boundaries separate:

- Provider adapter: parse Codex response events into typed metadata observations.
- Routing: support policy such as "avoid credentials with recent cyber
  mitigation" or "require explicit trusted-access proof" only when a positive
  proof signal exists.
- SQLite: store observation metadata by local credential ID with bounded
  retention.
- Management API: expose sanitized status and timestamps, not raw payloads.
- TUI: show "unknown", "verification recommended", "rerouted", or "blocked" as
  observed status. Do not label unknown accounts as verified.
- Config: make any routing requirement explicit, with startup validation that
  rejects `require_trusted_access` until a positive proof source is implemented.
- Logging: normal logs may record metadata-only event classes. IO-bearing data
  remains behind `[logging].capture_io = true`.

## Unsafe Designs

Do not implement:

- routing that treats unknown as verified;
- routing that treats successful requests as proof of Trusted Access;
- routing that scrapes local auth files for account state;
- routing that stores account emails, full account IDs, tokens, cookies, or raw
  provider payloads in normal metadata;
- background live account probing without a documented source-backed endpoint
  and explicit operator opt-in;
- config options that imply a verified-only pool when no positive verification
  source exists.

## Open Unknowns

- Whether OpenAI exposes a stable machine-readable Trusted Access status for
  Codex accounts outside the reviewed source.
- Whether future Codex clients will expose request-level checks that replace
  account-level checks, as the manual says is planned.
- Whether enterprise Trusted Access has an organization-level status endpoint.
- Whether API request logs expose structured reroute fields that ilonasin could
  fetch safely. The manual says rerouting is visible there, but this slice did
  not identify a documented fetch API for those logs.

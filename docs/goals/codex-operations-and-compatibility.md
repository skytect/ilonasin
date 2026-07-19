# Codex Operations and Compatibility

## Goal

Make Ilonasin a reliable, observable, and operator-friendly router for
authorized Codex capacity, with faithful protocol compatibility, clear
quota/reset visibility, safe routing, and robust operational behavior.

This document defines the desired end state and boundaries. It is not an
implementation plan. Small implementation slices belong in `docs/plans/`.

## 1. Faithful Codex compatibility

Ilonasin should preserve validated upstream Codex behavior rather than
approximating or fabricating it.

This includes:

- model metadata and capability flags;
- supported reasoning efforts and defaults;
- normal and maximum context-window information;
- Code Mode and tool behavior;
- Responses Lite after its request semantics are understood and verified;
- one dynamic Codex client version shared by discovery and outbound requests;
- a matching outbound User-Agent;
- explicit output-token behavior rather than silently discarded limits; and
- accurate downstream propagation of terminal success and failure events.

### Desired outcome

Supported Codex clients should behave similarly through Ilonasin and directly
against the Codex backend, without misleading warnings, missing capabilities,
or silently ignored caller intent.

## 2. Complete capacity visibility

Ilonasin should expose operational capacity without conflating distinct
concepts:

- included 5-hour usage;
- included weekly usage;
- each window's natural reset timestamp;
- banked rate-limit reset count, status, grant time, and expiry;
- active model-scoped routing cooldowns;
- stale, failed, or unavailable observations; and
- summative capacity for the normal Codex pool.

Purchased-credit information is not part of the first implementation. If added
later, it must remain visibly separate from included quota and banked resets.

### Desired outcome

An operator can immediately answer:

- Which accounts can accept work now?
- Which capacity is expected to return next?
- Which observations are stale or uncertain?
- Which banked resets will expire soon?
- Is waiting, routing legitimate independent work elsewhere, or redeeming a
  reset the best option?

Supporting research:

- `docs/codex-banked-rate-limit-resets.md`
- `docs/plans/104-codex-subscription-usage.md`

## 3. Safe banked-reset handling

The first supported banked-reset surface should be read-only:

- collect available banked-reset inventory;
- display count, type, status, grant time, and expiry;
- distinguish zero, ineligible, unavailable, stale, and failed states; and
- warn before valuable banked resets expire.

If redemption is added later, it must:

- remain a local management operation;
- require explicit operator confirmation;
- select an exact banked reset, preferring the earliest-expiring eligible one;
- use a caller-generated idempotency key and reuse it for retries;
- serialize concurrent redemption for one account;
- refetch all banked-reset and quota-window state afterward; and
- report the backend outcome precisely.

Automatic redemption must remain disabled by default.

### Desired outcome

Banked resets are neither forgotten nor accidentally consumed, and Ilonasin
never reports a successful reset without verifying refreshed quota state.

## 4. Operator-focused TUI

The TUI should prioritize operational decisions rather than raw telemetry.

It should show:

- the normal Codex pool before secondary or experimental pools;
- aggregate capacity before detailed account rows;
- separate labels for 5-hour, weekly, cooldown, and banked-reset state;
- account-level freshness and error indicators;
- the nearest natural reset and nearest banked-reset expiry;
- actionable failure and confirmation states; and
- readable layouts at narrow and wide terminal widths.

Example summary:

```text
Normal Codex pool

Available now       5 / 6 accounts
5h account capacity 598 / 600 pp
Weekly capacity     525 / 600 pp
Next observed reset Sat 2:05 PM
Banked resets       3 · next expires in 6d
Stale observations  1
```

Use **observed reset** unless fresh evidence proves that an exhausted capacity
bucket will become usable at that timestamp.

### Desired outcome

The operator can understand current capacity and the next useful action without
reading raw payloads, logs, or dense per-account diagnostics.

## 5. Correct routing semantics

Authoritative routing availability is:

```text
administratively enabled
+ resolvable authentication
+ no active model-scoped quota block
= routable
```

Subscription-usage snapshots are advisory unless they can be safely correlated
with active routing constraints.

Ilonasin should:

- automatically reconsider a quota-blocked credential after its cooldown
  expires;
- avoid treating stale usage as live capacity;
- distinguish an observed reset timestamp from confirmed capacity coming
  online;
- account for overlapping 5-hour and weekly constraints;
- avoid hidden or indefinite account cycling; and
- preserve legitimate account, provider, workspace, and privacy boundaries.

### Desired outcome

Routing decisions, client retry information, management APIs, and TUI capacity
claims agree with one another.

## 6. Operational reliability

Ilonasin should support continuous service through:

- graceful SIGTERM/SIGINT shutdown with bounded request draining;
- bounded network timeouts and connection limits;
- reliable and concurrent-safe OAuth refresh;
- bounded telemetry and IO-log retention;
- safe model-cache stale-if-error behavior;
- visible persistence failures;
- migration, backup, restore, and newer-schema safety; and
- secure local management transport.

### Desired outcome

Ilonasin can run continuously without silent corruption, unbounded resource
use, misleading health, or avoidable interruption of active requests.

## 7. Security and privacy hardening

Ilonasin should maintain a strict local credential boundary:

- state directories use restrictive permissions;
- service umask prevents permissive credential sidecars;
- SQLite DB/WAL/SHM permissions are enforced and failures are visible;
- observability excludes raw OAuth tokens, full provider account IDs, raw
  provider payloads, and monetary balances;
- management mutations remain scoped to the local Unix management API;
- banked-reset identifiers are treated as protected operational data;
  and
- no public plaintext management interface is introduced.

### Desired outcome

Quota visibility, reset monitoring, and future redemption controls do not
weaken Ilonasin's credential, privacy, or transport boundaries.

## Compatibility checklist

The former switching checklist is evidence for compatibility decisions, not a
runtime approval gate.

Before describing a Codex client/backend version as compatible, inspect the
relevant supported surfaces, including:

- model discovery for each configured Codex, DeepSeek, and OpenRouter surface;
- text, workspace editing, shell, and custom tools;
- Code Mode and namespaced/multi-agent tools;
- images, tool search, and compaction where supported;
- reasoning efforts and service tiers;
- root and `/v1` base URLs;
- representative authentication, quota, retry, and server failures; and
- privacy of logs and management output.

The checklist is evidence for a compatibility decision. It must not become a
new permission system or a large permanent test framework.

## Implementation status appendix

Current status for the seven goal areas:

| Area | Classification | Current state |
| --- | --- | --- |
| Faithful Codex compatibility | PARTIAL | Implemented: dynamic Codex client version plus matching User-Agent, exact source-backed model metadata fields, omission of unverified metadata, explicit translated Chat output-cap behavior (`max_tokens` rejected; valid positive `max_completion_tokens` accepted as a compatibility hint but omitted upstream), bounded Chat `json_object`/string tool-choice translation, bounded function-tool `strict`, and terminal completed/failed/incomplete/error handling. Deferred: Code Mode, Responses Lite, namespaced/MCP/shell/broader tools, and any Sol/Terra/Luna naming until upstream live evidence identifies them. |
| Complete capacity visibility | PARTIAL | Implemented: normal/weekly subscription snapshots, stale/error display, active model-scoped cooldown visibility separated from usage snapshots, and read-only banked-reset inventory. Deferred: provider-wide "available now" count because blocks are model-scoped and authoritative auth readiness needs route context. |
| Safe banked-reset handling | PARTIAL | Implemented: read-only sanitized count/detail inventory and TUI visibility. Deferred: redemption, automatic use, and any protected upstream reset-ID mapping until separately reviewed. |
| Operator-focused TUI | PARTIAL | Implemented: management-owned subscription, cooldown, credential, telemetry, and banked-reset views. Deferred: larger UX rework beyond current panes. |
| Correct routing semantics | PARTIAL | Implemented: model-scoped active cooldown separation and automatic reconsideration after `ActiveUntil`; OAuth CAS refresh coordination and backoff. Deferred: inbound admission control until overload semantics are designed. |
| Operational reliability | PARTIAL | Implemented: graceful drain on shutdown, bounded outbound transport, newer-schema refusal, permission/umask hardening, visible telemetry persistence failures, and manual telemetry pruning. Deferred: automatic telemetry retention until policy/config exists; WAL-safe backup/restore until destination, retention, and free-space UX are specified. |
| Security and privacy hardening | PARTIAL | Implemented: restrictive home/config/SQLite permissions, service umask, metadata-only default, local Unix management transport, sanitized banked-reset inventory with no upstream reset IDs. Deferred: no public or non-Unix management transport until separately designed. |

Explicit deferrals:

- Responses Lite and Code Mode wait for upstream evidence.
- Automatic telemetry retention waits for policy and config.
- WAL-safe backup/restore waits for destination, retention, and free-space UX.
- Inbound admission control waits for overload semantics.
- Provider-wide "available now" count is deferred because cooldowns are
  model-scoped and auth readiness is route-contextual.
- Public or non-Unix management transport is not implemented.

## Non-goals

- No standalone permanent-testing or CI initiative.
- No hidden account rotation intended to evade provider restrictions.
- No automatic banked-reset redemption by default.
- No synthetic keepalive traffic without current evidence and an explicit
  operator decision.
- No monetary billing dashboard in the first version.
- No broad TUI rewrite.
- No fabricated capabilities or unsupported Codex options.
- No merging or transfer of personal-account quotas, banked resets, or
  purchased balances.
- No implementation details or task-by-task checklist in this goal document.

## Relationship to architecture, research, and plans

```text
docs/goals/
  codex-operations-and-compatibility.md
      Stable desired outcomes and boundaries

docs/
  ilonasin-architecture.md
      Target system architecture

  codex-banked-rate-limit-resets.md
      Supporting research and evidence

docs/plans/
  ...
      Small implementation slices that advance this goal
```

Implementation plans should link back to this goal and identify the smallest
outcome they advance. The goal should change only when the intended product or
architectural destination changes.

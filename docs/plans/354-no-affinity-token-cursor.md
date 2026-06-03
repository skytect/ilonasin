# 354 No-Affinity Token Cursor

## Context

Credential pooling now uses three layers:

- deterministic ordering from local client token, provider instance, and model;
- request/session affinity from real client fields such as Codex
  `prompt_cache_key`, Chat `session_id`, and Claude Code metadata;
- least-in-flight plus round-robin selection when no usable affinity exists.

The remaining policy wrinkle is the no-affinity round-robin cursor. In-flight
pressure should stay provider/model/credential scoped so concurrent traffic from
all local clients shares the same upstream-account pressure view. The
round-robin tie cursor, however, is currently scoped only by provider instance
and model. That makes generic no-affinity traffic rotate globally and weakens
the explicit downstream local API-key distinction that the base affinity order
already includes.

For clients that send no stable session/cache metadata, the out-of-box local
distinction is the verified ilonasin client token. This slice keeps concurrency
balancing global, but scopes only the zero-pressure tie cursor by local token as
well as provider/model.

## Scope

1. Add local client token ID to the no-affinity round-robin cursor scope.
   - Use the verified local token ID already passed to
     `planCredentialAttempts`.
   - Do not include bearer tokens, upstream account IDs, request bodies,
     prompts, response data, or provider payloads.
2. Keep in-flight pressure counts unchanged.
   - Pressure remains keyed by provider instance, provider model, and upstream
     credential ID.
   - Concurrent requests from different local tokens still see the same
     credential pressure and avoid currently busy accounts.
3. Apply token-scoped cursoring only on the no-affinity path.
   - Requests with non-empty affinity still use sticky first-slot behavior.
   - No-affinity selection still chooses the lowest in-flight candidates first.
   - The token-scoped cursor is used only to break ties among equally
     low-pressure candidates.
4. Preserve quota filtering and retry behavior.
   - Quota-filtered candidate lists remain the source of selectable attempts.
   - Retry attempts continue to reserve from the remaining untried slots.
   - Fallback events, health events, and quota observations remain unchanged.
5. Do not change request parsing, provider adapters, storage, management API,
   TUI rendering, config, metadata schema, logging, or prompt/cache forwarding.

## Out Of Scope

- Persisting cursor state across daemon restarts.
- Weighting by subscription remaining quota.
- Latency, success-rate, or cost-based routing.
- Cross-provider or cross-model fallback.
- Deriving affinity from prompt/message content.
- Adding a management/TUI surface for affinity or cursor state.

## Implementation Steps

1. Extend the pressure cursor scope with local client token ID.
2. Pass token ID from `reserveCredentialAttempt` callers into the no-affinity
   reservation path.
3. Keep `trackCredentialAttempt` and in-flight acquire/release helpers
   unchanged.
4. Review the diff for unchanged sticky-affinity behavior, privacy boundaries,
   and provider/model/credential pressure semantics before running checks.

## Verification

Use a temporary focused check, then remove it before commit:

- no-affinity reservations for one local token rotate through eligible
  credentials;
- another local token has its own cursor and starts from its deterministic
  token-derived order;
- when one credential is already in flight, both tokens prefer lower-pressure
  candidates before cursor tie-breaking;
- explicit non-empty affinity still bypasses no-affinity cursoring and selects
  the first slot;
- retry-filtered slot lists still reserve only remaining untried credentials;
- pressure release returns counts to zero.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage`, and cleaning up all temporary
files and processes.

## Acceptance

- Generic no-affinity traffic balances out of the box while preserving local
  API-key distinction in the tie cursor.
- Global in-flight pressure still prevents concurrent traffic from piling onto
  the same upstream credential.
- Request/session affinity, quota filtering, fallback metadata, provider
  behavior, storage, management, TUI, and logging are unchanged.

# 351 Chat Affinity Shared Safety

## Context

`docs/ilonasin-architecture.md` requires normal metadata and routing behavior to
avoid full account IDs, request IDs, bearer-like values, raw payload markers,
and other sensitive or unstable identifiers. Recent pooling slices added a
shared strict affinity filter in `internal/privacy` and documented actual
client affinity signals in `docs/codex-compatibility-audit.md`.

Responses body affinity and server header fallback already use
`privacy.SafeStrictAffinityValue`. Chat Completions body affinity still uses a
local `safeChatAffinityValue` helper in `internal/openai/types.go`. That local
copy is looser: it rejects account, device, token, authorization, and JWT-like
values, but it does not reject request-id-shaped values. This leaves the Chat
path out of sync with the shared safety boundary and the source-backed signal
map.

## Goal

Use the shared strict affinity safety helper for OpenAI Chat body-derived
affinity, and remove the duplicated local helper.

## Scope

1. Update `internal/openai/types.go` so:
   - `chatAffinityKey` validates `session_id` and `user` with
     `privacy.SafeStrictAffinityValue`;
   - `chatMetadataAffinityKey` validates selected metadata values with
     `privacy.SafeStrictAffinityValue`;
   - the local `safeChatAffinityValue` helper is removed;
   - imports are adjusted.
2. Preserve the existing Chat affinity priority:
   - `session_id`;
   - `user`;
   - metadata keys `session_id`, `thread_id`, `conversation_id`,
     `prompt_cache_key`.
3. Do not change public Chat request validation, provider request marshaling,
   storage, management DTOs, TUI rendering, config, logging, Anthropic,
   Responses, credential-pool ordering, quota behavior, or route shapes.
4. Keep this as a behavior-tightening safety cleanup only. It should affect
   local-only `AffinityKey`, not any field forwarded upstream.

## Verification

Before implementation review:

1. Review the diff manually for scope and import cleanup.
2. Run a temporary focused in-package smoke, removed before commit, covering:
   - safe Chat `session_id`, `user`, and metadata values still become
     `AffinityKey`;
   - request-id-shaped, account/device/token/authorization/JWT/JSON-looking,
     empty, and too-long Chat values do not become `AffinityKey`;
   - decoded Chat requests still marshal upstream without `AffinityKey`.
3. Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/openai ./internal/server
go test ./...
go vet ./...
```

4. Build a temporary `ilonasin` binary, start `ilonasin serve` with an isolated
   temporary `ILONASIN_HOME`, verify management health over the Unix socket,
   run a short `ilonasin manage` TUI smoke, terminate the daemon, and clean up
   temporary files.

## Expected Outcome

- Chat, Responses, and header fallback affinity share one strict safety policy.
- Request-id-shaped Chat values no longer become local credential affinity.
- No stored metadata, logs, provider payloads, or public route behavior changes.

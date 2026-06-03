# 352 Anthropic Affinity Shared Safety

## Context

`docs/ilonasin-architecture.md` requires normal routing metadata and
observability paths to avoid full account IDs, bearer-like values, request IDs,
raw payload markers, and other sensitive or unstable identifiers. Recent slices
made OpenAI Chat, Responses, and server header fallback affinity use the shared
`privacy.SafeStrictAffinityValue` helper.

Anthropic Messages affinity still has a local helper in
`internal/anthropic/affinity.go`:

```go
func safeAnthropicAffinityValue(value string) string
```

That helper trims and length-checks only. It allows request-id-shaped,
account/device/token/authorization-shaped, JSON-looking, and JWT-looking values
to become local `AffinityKey` if they appear as `metadata.session_id` or as the
nested `session_id` inside Claude Code style `metadata.user_id` JSON. That is
inconsistent with the shared pooling safety boundary and the affinity signal
map in `docs/codex-compatibility-audit.md`.

## Goal

Use the shared strict affinity safety helper for Anthropic body-derived
affinity and remove the duplicated local safety helper.

## Scope

1. Update `internal/anthropic/affinity.go` so:
   - nested `metadata.user_id.session_id` is still preferred;
   - plain `metadata.session_id` is still the fallback;
   - both paths trim and validate with `privacy.SafeStrictAffinityValue`;
   - the local `safeAnthropicAffinityValue` helper is removed;
   - imports are adjusted.
2. Preserve existing Anthropic behavior outside local-only affinity:
   - do not use plain `metadata.user_id` as an affinity fallback;
   - do not forward Anthropic metadata into OpenAI Chat requests;
   - do not log, store, render, or expose the affinity value.
3. Do not change request decoding, route shapes, provider request marshaling,
   storage, management DTOs, TUI rendering, config, OpenAI Chat, Responses,
   credential-pool ordering, quota behavior, or fallback semantics.

## Verification

Before implementation review:

1. Review the diff manually for scope and import cleanup.
2. Run a temporary focused in-package smoke, removed before commit, covering:
   - Claude Code style `metadata.user_id` JSON with safe nested `session_id`
     still becomes `AffinityKey`;
   - plain `metadata.session_id` still becomes `AffinityKey` when no nested
     session exists;
   - nested `session_id` wins over plain `metadata.session_id`;
   - plain `metadata.user_id` without nested `session_id` remains ignored;
   - request-id-shaped, account/device/token/authorization/JWT/JSON-looking,
     empty, and too-long session values do not become `AffinityKey`;
   - marshaled upstream Chat requests still omit metadata, user, session ID, and
     `AffinityKey`.
3. Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/anthropic ./internal/server
go test ./...
go vet ./...
```

4. Build a temporary `ilonasin` binary, start `ilonasin serve` with an isolated
   temporary `ILONASIN_HOME`, verify management health over the Unix socket,
   run a short `ilonasin manage` TUI smoke, terminate the daemon, and clean up
   temporary files.

## Expected Outcome

- Chat, Responses, Anthropic Messages, and header fallback affinity share one
  strict safety policy.
- Unsafe Anthropic session values no longer become local credential affinity.
- No stored metadata, logs, provider payloads, public API behavior, TUI output,
  or management response behavior changes.

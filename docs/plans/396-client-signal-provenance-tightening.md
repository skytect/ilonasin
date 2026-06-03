# 396 Client Signal Provenance Tightening

## Context

The pooling architecture already supports safe affinity from request bodies,
selected metadata, selected session headers, and no-affinity load balancing.
The remaining user concern is framing: the useful contract is not which fields
could exist, but which clients actually send them out of the box.

`prompt_cache_key` should be considered when Codex sends it, but generic
clients may still send only model plus messages or input. The architecture must
make that distinction explicit so later pooling work does not depend on
optional metadata.

## Goal

Clarify the source-backed client signal map without changing routing behavior.

## Scope

1. Tighten `docs/ilonasin-architecture.md` so the credential affinity section
   separates:
   - observed named-client fields;
   - optional API fields;
   - local fallback inputs that always exist inside ilonasin.
2. State directly that `prompt_cache_key` is used because Codex CLI sends it in
   the audited Responses path, not because every OpenAI-compatible harness
   sends it.
3. Keep the generic-client baseline clear: if no safe session, user, metadata,
   or prompt-cache signal exists, route by local token identity, provider
   instance, provider model, least-in-flight pressure, and token-scoped cursor.
4. Align `docs/codex-compatibility-audit.md` with the architecture wording,
   including Responses `client_metadata` as an optional fallback surface.

## Out Of Scope

- Credential selection algorithm changes.
- New affinity sources.
- Persistent affinity mappings.
- Management API, TUI, config, schema, provider adapter, or logging changes.
- New live captures against Codex or Claude Code.

## Verification

Run:

```sh
rg -n "prompt_cache_key|client_metadata|session-id|thread-id|x-client-request-id|x-codex-window-id|x-claude-code-session-id|least-in-flight|cursor" docs/ilonasin-architecture.md docs/codex-compatibility-audit.md internal/openai internal/anthropic internal/server
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke for slice discipline.

## Acceptance

- The docs answer what sends what before explaining local interpretation.
- Codex CLI `prompt_cache_key` is treated as an observed signal.
- Generic clients are not implied to send session IDs or prompt cache keys.
- Minimal clients still have out-of-box load balancing.
- Runtime behavior is unchanged.

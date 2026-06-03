# 388 Pooling Signal Grounding

## Context

The next pooling work depends on knowing what real clients send without extra
configuration. Current implementation already accepts multiple local-only
affinity signals:

- Chat Completions: `session_id`, top-level `prompt_cache_key`, `user`, and
  selected `metadata` keys.
- Responses: top-level `prompt_cache_key`, selected `client_metadata` keys, and
  selected top-level `metadata` keys.
- Anthropic Messages: nested `metadata.user_id.session_id`, then plain
  `metadata.session_id`; safe session headers apply only after conversion if
  body-derived affinity is empty.
- Header fallback: `session-id`, `thread-id`, and
  `x-claude-code-session-id` only when body affinity is absent.
- Minimal clients: local API token identity plus provider instance, provider
  model, least-in-flight pressure, and round-robin tie breaking.

The architecture doc describes this, but it mixes observed client behavior with
pooling interpretation. The user wants the distinction to be more explicit:
the point is what sends what.

## Goal

Clarify the source-backed affinity signal map without changing routing behavior.

## Scope

1. Update `docs/ilonasin-architecture.md` so the pooling signal section clearly
   separates:
   - observed or common request fields by client/API;
   - current local implementation priority;
   - explicit non-signals such as request IDs, window IDs, installation IDs,
     account/device/token values, prompts, and local bearer secrets.
2. Keep Codex-specific wording tied to the audited local version:
   `codex-cli 0.135.0`.
3. Keep Claude Code wording tied to the captured local version:
   `Claude Code 2.1.159`.
4. State directly that `prompt_cache_key` is used when present, but many
   generic clients do not send it, so no-affinity pooling must still work out of
   the box.
5. Update `docs/codex-compatibility-audit.md` only if needed to keep the
   architecture summary and audit wording aligned.

## Out Of Scope

- Credential selection algorithm changes.
- New persistent affinity mappings.
- New management API, TUI, config, schema, provider, or logging behavior.
- Running new live captures against Codex or Claude Code.

## Verification

Run:

```sh
rg -n "prompt_cache_key|session-id|thread-id|x-client-request-id|x-codex-window-id|x-claude-code-session-id|AffinityKey" docs/ilonasin-architecture.md docs/codex-compatibility-audit.md internal/openai internal/anthropic internal/server
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke even though this is
docs-only, to keep the slice discipline consistent.

## Acceptance

- The docs answer “what sends what” without implying every harness sends
  session metadata.
- The docs make clear that `prompt_cache_key` is already a preferred signal
  when present.
- The docs make clear that minimal clients still get load balancing through the
  local token, model route, least-in-flight pressure, and round-robin cursor.
- No runtime behavior changes.

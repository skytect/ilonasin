# 392 Affinity Signal Provenance

## Context

The pooling architecture now has a signal map, but the user called out the main
risk: the important question is what clients actually send, not which fields an
API could theoretically accept.

The current implementation already supports safe local affinity from:

- Chat Completions `session_id`, top-level `prompt_cache_key`, `user`, and
  selected `metadata` keys.
- Responses top-level `prompt_cache_key`, selected `client_metadata` keys, and
  selected top-level `metadata` keys.
- Anthropic Messages nested `metadata.user_id.session_id` and plain
  `metadata.session_id`.
- Stable session headers only as fallback.
- Minimal-client routing through local token identity, provider/model route,
  least-in-flight pressure, and cursor state.

The docs should make clear that Codex CLI sends `prompt_cache_key` out of the
box, while generic OpenAI-compatible harnesses often send only model plus
message/input content.

## Goal

Clarify affinity signal provenance in `docs/ilonasin-architecture.md` without
changing routing behavior.

## Scope

1. Add a short provenance rule before the affinity signal table:
   - observed named-client fields come from source inspection or local capture;
   - common API fields are optional and must not be assumed;
   - local fallback behavior is required for minimal clients.
2. Tighten the table wording:
   - Codex CLI `prompt_cache_key` is an observed out-of-box signal;
   - Codex installation/window/request IDs are observed non-signals;
   - generic OpenAI Chat and Responses `prompt_cache_key` are optional API
     fields, not guaranteed harness behavior;
   - minimal clients are the expected no-metadata baseline.
3. Keep local implementation priority aligned with current code:
   - body affinity first;
   - selected safe metadata next;
   - selected safe session headers last;
   - no-affinity least-in-flight plus cursor balancing when no safe signal
     exists.

## Out Of Scope

- Credential selection algorithm changes.
- New affinity sources.
- New tests, storage, management API, TUI, provider, logging, or config changes.
- New live captures against Codex or Claude Code.

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

- The architecture answers what sends what.
- It treats Codex CLI `prompt_cache_key` as a real observed signal.
- It does not imply generic clients or harnesses normally send session IDs or
  prompt cache keys.
- It preserves out-of-box pooling for minimal clients.
- No runtime behavior changes.

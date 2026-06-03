# 377 Codex Window Affinity Grounding

## Context

Credential pooling now uses fields real clients actually send. For Codex
Responses, the source-backed default is body `prompt_cache_key`, derived from
the thread ID. Codex also sends transport headers including `session-id`,
`thread-id`, `x-client-request-id`, and `x-codex-window-id`.

The current server header fallback accepts `x-codex-window-id`. That is broader
than the architecture table, which says to use safe session or thread headers as
fallback and never use request-id-shaped values. Window lineage is observed
transport metadata, but it is not the same as the cache/session key we want for
credential stickiness.

## Scope

1. Keep the current preferred affinity order:
   - Chat body `session_id`;
   - Chat body `prompt_cache_key`;
   - Chat body `user`;
   - selected Chat metadata keys;
   - Responses body `prompt_cache_key`;
   - selected Responses `client_metadata` keys;
   - Anthropic nested `metadata.user_id.session_id`;
   - Anthropic `metadata.session_id`.
2. Keep safe header fallback only for:
   - `session-id`;
   - `thread-id`;
   - `x-claude-code-session-id`.
3. Stop accepting `x-codex-window-id` as an ingress credential-affinity
   fallback.
4. Update architecture and compatibility docs to distinguish observed Codex
   window metadata from supported session/thread affinity.
5. Do not change upstream Codex provider headers, provider adapters, credential
   storage, quota state, request metadata schema, management DTOs, TUI, config,
   IO logging, or provider payload shapes.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server ./internal/openai ./internal/anthropic
go test ./...
go vet ./...
```

Run direct temporary `serve` and bounded `manage` smokes, then clean up all
temporary files and processes.

## Acceptance

- Codex body `prompt_cache_key` remains the primary out-of-box cache affinity
  signal.
- Session and thread headers remain fallback affinity signals when no safe body
  signal exists.
- `x-codex-window-id` and `x-client-request-id` are observed but not used for
  ingress credential affinity.
- No affinity value is logged, persisted, rendered, exposed, or forwarded
  upstream by this slice.

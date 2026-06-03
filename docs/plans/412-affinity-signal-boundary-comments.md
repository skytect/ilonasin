# 412 Affinity Signal Boundary Comments

## Context

The pooling signal map is now grounded in `docs/ilonasin-architecture.md` and
`docs/codex-compatibility-audit.md`:

- Codex CLI Responses sends `prompt_cache_key` in the audited path.
- Claude Code Anthropic sends session data in metadata in observed traffic.
- Generic OpenAI-compatible clients often send only model plus messages or input.
- Minimal clients may provide no stable request-level identifier beyond the
  verified local ilonasin API token.

The implementation already follows this policy, but the provenance rule is
spread across several extraction points. Future pooling work should not infer
credential affinity from request IDs, window IDs, installation IDs, account IDs,
device IDs, token-like values, prompts, messages, input, raw bodies, or tool
payloads.

## Goal

Make the existing affinity boundaries self-auditing without changing routing
behavior.

## Scope

1. Add concise comments at the current affinity extraction boundaries:
   - Chat: `chatAffinityKey`, `chatMetadataAffinityKey`, and
     `chatPromptCacheKey` in `internal/openai/types.go`.
   - Responses: `responseAffinityPromptCacheKey`, `ResponsesRequest.AffinityKey`,
     and `responsesMetadataAffinityKey` in `internal/openai/responses.go`.
   - Anthropic: `anthropicAffinityKey` in `internal/anthropic/affinity.go`.
   - Header fallback: `requestHeaderAffinity` in
     `internal/server/request_affinity.go`.
   - No-affinity credential balancing: `reserveCredentialAttempt` in
     `internal/server/credential_pool.go`.
2. The comments must explicitly preserve this signal priority:
   - use client cache/session affinity when the client actually sends a safe
     field;
   - keep `prompt_cache_key` preferred because Codex sends it, not because every
     harness sends it;
   - let empty affinity remain valid and load-balanced by token/provider/model,
     pressure, and cursor state.
3. Do not change parsing, validation, provider adapters, credential ordering,
   quota filtering, pressure tracking, fallback metadata, storage, management
   routes, TUI, config, public API responses, or logging behavior.
4. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- Chat with only `model` and `messages` has empty affinity.
- Chat uses safe `prompt_cache_key` when present.
- Chat rejects request-id, account/device/install/token-like, and unlisted
  metadata values as affinity.
- Responses with only `model` and `input` has empty affinity.
- Responses uses safe `prompt_cache_key` when present.
- Responses rejects request-id, account/device/install/token-like, and unlisted
  metadata values as affinity.
- Anthropic without supported metadata converts to empty affinity.
- Anthropic uses the observed Claude Code nested `metadata.user_id.session_id`
  shape when safe.
- `requestHeaderAffinity` ignores `x-client-request-id` and `x-codex-window-id`,
  and accepts only safe allowed session headers.
- Empty affinity still reaches the least-pressure token/provider/model cursor
  path.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/openai ./internal/anthropic ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- The code comments answer “what sends what” at the boundaries that matter.
- `prompt_cache_key` remains a preferred real signal only when a client sends it.
- Generic/minimal clients remain valid with empty affinity.
- No runtime behavior changes.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

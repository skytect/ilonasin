# 376 Chat Prompt Cache Key Affinity

## Context

Current pooling affinity supports:

- Responses top-level `prompt_cache_key`;
- Responses selected `client_metadata` keys;
- Chat `session_id`, `user`, and selected `metadata` keys;
- Anthropic metadata-derived session affinity;
- safe session header fallback;
- no-affinity least-in-flight and round-robin balancing.

OpenAI-compatible Chat clients can also send top-level `prompt_cache_key`.
Local source inspection of `/tmp/openai-node/src/resources/chat/completions/completions.ts`
shows Chat Completions exposes `prompt_cache_key` and describes `user` as
deprecated in favor of `prompt_cache_key` for caching. The current Chat decoder
rejects that field as unknown, so those clients cannot provide their real cache
affinity signal through the Chat route.

This should be a local routing-affinity input only. It should not change
provider payload behavior or metadata storage.

## Goal

Accept safe top-level Chat `prompt_cache_key` as local-only credential affinity,
with priority above deprecated `user`, without forwarding, logging, storing, or
displaying the value.

## Scope

1. Update the docs signal map:
   - mention top-level Chat `prompt_cache_key` in
     `docs/ilonasin-architecture.md`;
   - mention the local OpenAI SDK source evidence in
     `docs/codex-compatibility-audit.md`.
2. Extend `openai.ChatCompletionRequest` with a local decoded
   `PromptCacheKey` field.
3. Allow top-level `prompt_cache_key` in Chat request decoding.
4. Parse it permissively for affinity:
   - absent and `null` produce no affinity;
   - non-string, blank, overlong, request-id-shaped, account/device/token/
     authorization/JWT/JSON-looking, or otherwise unsafe values are ignored for
     affinity rather than rejected.
5. Preserve Chat affinity priority:
   - safe `session_id`;
   - safe top-level `prompt_cache_key`;
   - safe `user`;
   - safe selected `metadata` keys: `session_id`, `thread_id`,
     `conversation_id`, `prompt_cache_key`.
6. Keep the value local-only:
   - do not add it to `MarshalUpstreamChatRequest`;
   - do not store it in request metadata;
   - do not log it, including in `capture_io` request-body logs;
   - do not expose it in management DTOs or the TUI.
7. Keep provider validation local-field aware:
   - do not reject `prompt_cache_key` merely because it was present, because it
     has already been consumed as a local-only router field;
   - do not add it to provider-supported field lists or upstream payloads;
   - do not change Responses, Anthropic, credential-pool math, quota behavior,
     storage schema, management routes, TUI rendering, or config.
8. Extend the IO scrubber boundary narrowly so JSON fields named
   `prompt_cache_key` are redacted when IO capture is explicitly enabled.

## Verification

Run a temporary focused in-package smoke and remove it before commit. It should
prove:

- safe top-level Chat `prompt_cache_key` is accepted and becomes `AffinityKey`;
- `session_id` still wins over top-level `prompt_cache_key`;
- top-level `prompt_cache_key` wins over `user` and metadata affinity;
- unsafe, blank, null, non-string, and overlong `prompt_cache_key` values remain
  accepted where appropriate but do not become affinity;
- decoded Chat marshaling does not include `prompt_cache_key` or `AffinityKey`;
- provider validation does not reject the local-only field and upstream
  marshaling still omits it;
- a route-level smoke with a seeded `prompt_cache_key` marker records normal
  request metadata and route logs without that marker appearing in metadata
  fields or normal log output;
- a direct `capture_io = true` smoke writes `ilonasin-io.log` and proves the
  seeded marker is absent from the scrubbed IO log while the
  `prompt_cache_key` key itself is redacted.

Then run:

```sh
rg -n "prompt_cache_key|PromptCacheKey|chatAffinityKey" internal/openai internal/provider internal/server docs/ilonasin-architecture.md docs/codex-compatibility-audit.md
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/openai
go test ./internal/provider
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Chat clients can send safe top-level `prompt_cache_key` for local credential
  affinity.
- The value is never forwarded, persisted, logged, displayed, or exposed.
- Existing Chat affinity behavior remains intact except for the new priority
  slot between `session_id` and `user`.
- Provider behavior remains explicit rather than silently accepting an
  unsupported upstream field.

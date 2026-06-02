# 326 Request Affinity Key Pooling

## Context

Plan 325 added deterministic credential-pool affinity from the verified local
ilonasin client token, provider instance, and provider model. That removes
resolver-order dominance across distinct local API keys, but it intentionally
does not solve the common single-local-token case.

The code already accepts OpenAI Chat `session_id` and `user` fields, and
`docs/openrouter-api.md` documents `session_id` and `prompt_cache_key` as
request-level affinity/cache concepts. The Responses route currently allows
`prompt_cache_key` but does not parse it into `ResponsesRequest`.

This slice adds a local-only request affinity key to credential attempt
planning. A single local API key can then spread different sessions/cache keys
across upstream credentials, while the same session/cache key stays sticky for
cache affinity.

## Scope

1. Add a request-affinity field to the serving execution contexts.
   - Use trimmed Chat `session_id` when present and non-blank.
   - Otherwise use trimmed Chat `user` when present and non-blank.
   - For Responses, read `prompt_cache_key` and transfer it through
     `ToChatCompletionRequest` as local affinity input only when it is a trimmed
     non-empty string up to 256 characters.
   - If no request-affinity field exists, preserve the plan 325 local-token-only
     ordering.
2. Include the request-affinity key in the credential affinity hash after local
   token ID, provider instance, and provider model.
3. Keep the affinity key local-only.
   - Do not store it in request metadata.
   - Do not render it in the TUI.
   - Do not log it.
   - Do not add management API fields.
   - Do not use prompts, messages, raw input, request bodies, response bodies,
     tool payloads, bearer tokens, upstream account IDs, or provider payloads as
     affinity input.
4. Preserve provider request behavior.
   - Chat `session_id` and `user` continue to be forwarded exactly as today where
     existing request marshaling forwards them.
   - Responses `prompt_cache_key` remains allowed but should not be newly
     forwarded in this slice.
   - Do not change Codex-generated prompt cache keys or upstream request IDs.
5. Preserve quota filtering, retry/fallback semantics, and model credential
   alignment from plan 325.
6. Do not add config, storage schema, provider adapter behavior, management API,
   TUI changes, or permanent tests.

## Out Of Scope

- Adaptive least-in-flight or least-recently-used load balancing.
- Persisted per-session mappings.
- Concurrency counters.
- Subscription remaining-quota weighting.
- Provider balance or billing queries.
- Cross-provider or cross-model fallback.
- Forwarding Responses `prompt_cache_key` upstream.

Those should be later slices once the request-affinity boundary is explicit.

## Implementation Steps

1. Add `PromptCacheKey string` to `openai.ResponsesRequest`.
2. Parse `prompt_cache_key` permissively so existing accepted Responses requests
   remain accepted. Use it for affinity only when it is a trimmed non-empty
   string up to 256 characters; null, empty, blank, non-string, or too-long
   values should be ignored for affinity rather than rejected in this slice.
3. Carry `PromptCacheKey` into the converted `ChatCompletionRequest` using a
   local-only field such as `AffinityKey`.
4. Add a local-only `AffinityKey` field to `openai.ChatCompletionRequest` with
   a non-serializing `json:"-"` tag.
5. Populate Chat affinity from trimmed `session_id`, then trimmed `user`, after
   decoding. Blank strings are ignored for affinity, preserving existing request
   validation behavior.
6. Update `planCredentialAttempts` to accept an affinity key and include it in
   the hash only when non-empty.
7. Update streaming and non-streaming execution call sites.
8. Review the diff for privacy, provider payload stability, and unchanged retry
   behavior.

## Verification

Run a temporary focused smoke, then remove it before commit. It should prove:

- two different Chat `session_id` values for the same local token/provider/model
  can produce different first credentials with deterministic helper inputs;
- the same Chat `session_id` remains sticky;
- Chat `session_id` takes precedence over Chat `user`;
- Chat `user` is used when `session_id` is absent;
- blank Chat `session_id` and `user` values are ignored for affinity rather than
  becoming sticky blank keys;
- null, empty, blank, non-string, and too-long Responses `prompt_cache_key`
  values remain accepted but are ignored for affinity;
- Responses `prompt_cache_key` is parsed and affects local credential affinity;
- Responses `prompt_cache_key` is not forwarded by the existing Codex request
  marshaling in this slice;
- the local `AffinityKey` field has a `json:"-"` tag and does not appear in
  OpenAI or Codex marshaled request bodies;
- no affinity key is stored in request metadata;
- quota filtering still preserves the affinity ring order;
- fallback events are not emitted merely because affinity changed the initial
  order.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/openai
go test ./internal/server
go test ./...
go vet ./...
```

The `find` output should be compared against the pre-existing permanent test
file list. This slice must not add a permanent test file.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temporary directory.

## Acceptance

- A single local ilonasin API key can spread distinct request sessions/cache keys
  across the same provider/model credential pool.
- The same request session/cache key remains sticky for the same local token,
  provider, and model while credentials remain eligible.
- The affinity key remains local-only and metadata/privacy boundaries are
  unchanged.
- Provider payload behavior, quota filtering, retry/fallback recording, and
  same-provider/model constraints remain unchanged.

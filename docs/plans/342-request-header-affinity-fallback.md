# 342 Request Header Affinity Fallback

## Context

Credential pooling now uses real client request signals where available:

- Codex Responses `prompt_cache_key`;
- OpenAI Chat `session_id`, then `user`, then selected safe metadata keys;
- Anthropic Messages `metadata.user_id` JSON with nested `session_id`, then
  `metadata.session_id`;
- daemon-local least-in-flight and round-robin selection when no affinity exists.

Fresh local recorder captures on 2026-06-03 confirmed:

- `codex-cli 0.135.0` sends Responses `prompt_cache_key` and also sends
  `session-id`, `thread-id`, `x-client-request-id`, and `x-codex-window-id`;
- `Claude Code 2.1.159` sends Anthropic `metadata.user_id` with nested
  `session_id` and also sends `X-Claude-Code-Session-Id`;
- generic OpenAI Chat and Anthropic-compatible clients may send only model and
  message content, so request metadata is optional, not an out-of-box signal.

The body-derived paths cover the common Codex and Claude Code cases. This slice
adds a narrow fallback for known session headers when a client supplies the
header but no usable body-level affinity. That improves robustness without
inventing identity from prompts, local API tokens, IP addresses, user agents, or
message content. Observed per-request IDs such as `x-client-request-id` are
deliberately excluded because they are not stable session/cache affinity.

## Scope

1. Add a small `internal/server` helper that derives local-only affinity from
   selected request headers only when the decoded request has an empty
   `openai.ChatCompletionRequest.AffinityKey`.
2. Recognize only these headers:
   - `session-id`;
   - `thread-id`;
   - `x-codex-window-id`;
   - `x-claude-code-session-id`.
3. Normalize header values conservatively:
   - trim whitespace;
   - reject empty values and values over 256 runes;
   - for `x-codex-window-id`, use only the prefix before `:` so Codex window
     suffixes do not split one thread into multiple upstream accounts;
   - reject JSON-looking, JWT-looking, account/device/secret/token-shaped, and
     authorization-shaped values.
4. Apply the fallback in:
   - `POST /v1/chat/completions`;
   - `POST /v1/responses`;
   - `POST /v1/messages`.
5. Keep body-derived affinity higher priority than header affinity.
6. Keep header affinity local-only:
   - do not store it in request metadata;
   - do not log it;
   - do not render it in the TUI or management API;
   - do not forward it upstream;
   - do not include it in errors.
7. Preserve current no-affinity behavior. If no usable body field or header is
   present, requests continue using least-in-flight plus round-robin pooling.
8. Do not change credential storage, quota storage, provider adapters, request
   metadata schema, management DTOs, TUI, config, IO logging, or provider
   payload shapes.

## Out Of Scope

- Persisted session-to-credential mappings.
- New management or TUI surfaces for affinity.
- Deriving affinity from prompts, messages, tools, request bodies, local API
  tokens, upstream account IDs, IP addresses, user-agent strings, or bearer
  tokens.
- Cross-provider or cross-model fallback.
- Remaining-quota weighting.
- Changing Codex or Claude Code request compatibility.

## Implementation Steps

1. Add a focused helper in `internal/server` near the route/pooling boundary.
2. Call the helper after request decoding/conversion and before request
   validation reaches credential planning.
3. Ensure the helper mutates only the local `AffinityKey` field and never
   request fields that are marshaled to providers.
4. Review the diff for privacy, duplicate validation, and unchanged provider
   payload behavior before running checks.

## Verification

Use a temporary focused check, then remove it before commit, covering:

- body-derived affinity wins over all supported headers;
- `session-id`, `thread-id`, and `x-claude-code-session-id` can provide
  fallback affinity;
- `x-codex-window-id` trims the suffix after `:`;
- `x-client-request-id` is ignored because request IDs are usually
  per-request, not session/cache affinity;
- blank, overlong, JSON-looking, JWT-looking, account/device/secret/token, and
  authorization-shaped values are ignored;
- ignored header values leave `AffinityKey` empty so no-affinity balancing still
  applies;
- accepted header affinity intentionally uses the sticky affinity path, while no
  usable body or header signal continues using no-affinity round-robin;
- marshaled OpenAI, Codex Responses, and Anthropic-converted provider requests
  do not include `AffinityKey` or copied header values;
- request metadata helpers do not persist header affinity.
- fallback, health, and quota metadata remain unchanged and contain only the
  existing credential IDs, provider/model fields, status classes, retry/reset
  timing, and fallback reasons, never header affinity values.
- route-level checks prove the fallback is applied after body-derived affinity
  and before credential planning for Chat Completions, Responses, and Anthropic
  Messages.
- invalid or ignored headers leave each route on the no-affinity balancing path.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting
`ilonasin serve` with a temporary `ILONASIN_HOME`, checking management health
over the Unix socket, running bounded `ilonasin manage`, and cleaning up all
temporary files and processes.

## Acceptance

- Known agent session headers can preserve sticky same-provider, same-model
  credential affinity when body-level affinity is absent.
- Existing Codex and Claude Code body-derived affinity remains the preferred
  path.
- Generic clients without usable affinity still spread through the current
  no-affinity balancing path.
- No header affinity values are persisted, logged, rendered, exposed, or
  forwarded upstream.

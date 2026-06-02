# 329 Anthropic Metadata Affinity

## Context

Current pooling already uses:

- OpenAI Chat `session_id`, then `user`;
- Responses `prompt_cache_key`;
- local API token, provider instance, and provider model as the deterministic
  base;
- existing deterministic local-token/provider/model ordering plus
  least-in-flight reservation when no request affinity exists.

Fresh local capture against fake recorder endpoints on 2026-06-03 showed:

- `codex-cli 0.135.0` sends Responses `prompt_cache_key` by default, equal to
  the Codex thread/session ID, plus `session-id`, `x-codex-turn-metadata`, and
  `x-codex-window-id` headers.
- `Claude Code 2.1.159` sends Anthropic `metadata.user_id` by default as a JSON
  string containing `device_id`, `account_uuid`, and `session_id`, plus an
  `X-Claude-Code-Session-Id` header.
- Plain OpenAI-compatible and Anthropic-compatible SDK clients may send only
  model and messages unless the harness opts into request metadata.

The Anthropic parser currently accepts `metadata`, but `ToChatCompletion` drops
it before the shared credential-pool planner sees the request. That means
default Claude Code traffic has no sticky request affinity and uses only
the existing no-affinity deterministic base plus least-in-flight reservation.

## Scope

1. Keep this slice limited to Anthropic request parsing/conversion and this
   plan.
   - Touch `internal/anthropic` only unless a compile boundary requires a small
     adjacent change.
   - Do not touch currently dirty `internal/server` or `internal/provider`
     files.
   - Verify from clean `HEAD` that the already-committed server path consumes
     `openai.ChatCompletionRequest.AffinityKey` in credential planning before
     claiming end-to-end pooling behavior.
2. Derive a local-only Anthropic affinity key from safe request metadata.
   - Prefer `metadata.user_id` when it is a string containing JSON object field
     `session_id`.
   - Otherwise use a plain string `metadata.session_id` if present.
   - Trim whitespace and require non-empty values up to 256 runes.
   - Ignore malformed JSON, non-string metadata values, blank strings, and
     overlong strings rather than rejecting requests.
   - Do not use plain `metadata.user_id` as a fallback, because Claude Code's
     captured value can contain device and account fields when nested parsing
     fails.
3. Transfer the derived value to `openai.ChatCompletionRequest.AffinityKey`
   inside `anthropic.Request.ToChatCompletion`.
4. Keep the affinity key local-only.
   - Do not forward it upstream.
   - Do not store it in request metadata.
   - Do not render it in the TUI.
   - Do not log it.
   - Do not expose it through management APIs or errors.
   - Do not use prompts, messages, request bodies, response bodies, tool
     payloads, bearer tokens, upstream account IDs, raw provider payloads, or
     device IDs as affinity input.
5. Preserve existing generic-client behavior.
   - If no usable Anthropic metadata field exists, leave `AffinityKey` empty so
     existing local-token deterministic base ordering plus least-in-flight
     reservation remains active.
   - Do not add synthetic affinity from model, message count, content, IP
     address, user agent, or local API key beyond the existing base hash.

## Verification

Use a temporary focused test file, then remove it before commit, covering:

- Claude Code style `metadata.user_id` JSON string extracts `session_id`;
- plain `metadata.session_id` works when present;
- malformed JSON, non-string metadata, blank strings, overlong strings, and
  plain `metadata.user_id` values without nested `session_id` are ignored;
- Claude Code style `metadata.user_id` JSON containing `device_id`,
  `account_uuid`, and `session_id` uses only `session_id`;
- Claude Code style `metadata.user_id` JSON without `session_id` does not fall
  back to the whole JSON blob;
- `ToChatCompletion` sets `AffinityKey` and does not add serialized Chat fields.
- clean `HEAD` server code already consumes `ChatCompletionRequest.AffinityKey`
  in credential attempt planning.
- marshaling the converted Chat request for an upstream provider does not
  include raw `metadata.user_id`, nested `session_id`, `device_id`,
  `account_uuid`, or top-level `metadata`, `session_id`, or `user` fields.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/anthropic
go test ./...
go vet ./...
```

Build `cmd/ilonasin`, start `ilonasin serve` with temporary `ILONASIN_HOME` and
`[server] bind = "127.0.0.1:0"`, check `/_ilonasin/manage/health` over the
management socket, run a short `ilonasin manage` TUI smoke, and clean up.

Because unrelated dirty server/provider files exist, verify the staged patch in
an isolated worktree from `HEAD` before commit.

## Acceptance

- Default Claude Code Anthropic-compatible requests get stable session affinity
  when they include the captured `metadata.user_id` shape.
- Generic Anthropic-compatible requests with no useful metadata still spread by
  current least-in-flight behavior.
- No new raw metadata values are persisted, logged, forwarded, or displayed.

# 191 Anthropic Messages Compatibility

## Goal

Add a fresh, narrow Anthropic Messages-compatible local API surface for Claude
Code, after reverting the prior plan 188 implementation.

The first target is Claude Code pointed at ilonasin and routed to
`codex/gpt-5.5` through the existing Codex provider adapter.

## Ground Truth

- `docs/ilonasin-architecture.md` keeps local API auth, upstream credentials,
  provider adapters, routing, HTTP transport, TUI, config, and SQLite storage as
  separate boundaries.
- Existing Chat and Responses routes already handle model resolution,
  credential pooling, OAuth refresh on 401, quota observations, fallback
  metadata, and safe request metadata recording.
- Claude Code is installed locally at `/home/ivan/.local/bin/claude`, version
  `2.1.159`; the smoke bypasses that wrapper and uses the Nix store Claude
  Code binary at version `2.1.150`.
- Claude Code Anthropic mode can send local auth as `X-Api-Key` from
  `ANTHROPIC_API_KEY`.
- The current Codex chat validation rejects Chat Completions `max_tokens`,
  `temperature`, `top_p`, `top_k`, and `stop`, so Anthropic compatibility must
  validate those locally but avoid forwarding them on Codex requests.

## Scope

1. Add authenticated `POST /v1/messages`.
2. Accept ilonasin local client tokens via `Authorization: Bearer <token>` and
   `X-Api-Key: <token>`.
3. Add a small `internal/anthropic` compatibility package for Anthropic wire
   decoding, validation, request translation, response JSON, and buffered SSE
   payloads.
4. Support the minimal Claude Code path:
   - required `model`;
   - required positive `max_tokens`;
   - `messages` with `user` and `assistant`;
   - top-level `system` as string or text block array;
   - text content blocks;
   - `image` blocks with URL sources;
   - `tool_use` and `tool_result` blocks;
   - `tools` as Anthropic custom tools mapped to Chat function tools;
   - `tool_choice` absent, `auto`, or `{ "type": "auto" }`;
   - `cache_control` on top-level requests, content blocks, and tools accepted
     for Claude Code compatibility and stripped before provider dispatch;
   - Claude Code request controls `thinking`, `context_management`, and
     `output_config` accepted and stripped before provider dispatch;
   - `stream`, `temperature`, `top_p`, `top_k`, `stop_sequences`, and
     `metadata` accepted where they can be represented or safely ignored.
5. Allow Claude Code model aliases that it accepts locally, such as `sonnet`,
   to route to `gpt-5.5` only when exactly one Codex provider instance is
   configured. If multiple Codex instances exist, return the normal addressing
   error instead of guessing.
6. Reject unsupported fields and content block types locally with Anthropic
   shaped errors.
7. Reuse the existing non-streaming chat execution path for both non-stream and
   streamed Anthropic responses.
8. Emit buffered Anthropic SSE for `stream: true`. This is client stream
   compatibility, not true upstream token streaming.
9. Record request metadata under `anthropic_messages` without storing raw
   prompts, completions, request bodies, response bodies, tool arguments, tool
   results, raw SSE chunks, bearer tokens, full account IDs, or request IDs.
10. Smoke with Claude Code using a fresh local client token, temporary Claude
   state, and `ANTHROPIC_BASE_URL` pointed at the local ilonasin daemon.

## Non-Goals

- No native Anthropic upstream provider.
- No `/v1/messages/count_tokens` unless Claude Code blocks on it.
- No provider-side prompt caching, citations, thinking blocks, MCP server
  pass-through, server tools, computer-use tools, or true streaming in this
  slice.
- No permanent tests.
- No TUI work in this slice.
- No subscription keepalive changes.
- No changes to the concurrent plan 300 logging work.

## Implementation

1. Add `internal/anthropic`:
   - strict request decoder using `json.Decoder.UseNumber`;
   - request-to-`openai.ChatCompletionRequest` translator;
   - chat-result-to-Anthropic message response encoder;
   - buffered SSE event writer payload helpers;
   - Anthropic-shaped error envelope.
2. Add `internal/server/anthropic_route.go`:
   - read and IO-log request input like existing routes;
   - decode Anthropic request;
   - translate to a chat request using the resolved provider type;
   - resolve model, provider, adapter, and credentials;
   - execute through `executeNonStreamingChat`;
   - record through `recordNonStreamingChat`;
   - write Anthropic JSON or buffered SSE.
3. Wire route and metadata:
   - register `POST /v1/messages`;
   - add route label;
   - add `metadataEndpointAnthropicMessages`.
4. Adjust local auth parsing so `X-Api-Key` is treated as an ilonasin local
   token only on `/v1/messages` and only when no bearer authorization is
   present.
5. Keep all route errors Anthropic shaped for `/v1/messages`; existing OpenAI
   route errors remain unchanged.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
git diff --check
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
cat >"$tmp/config.toml" <<EOF
[server]
bind = "127.0.0.1:11435"

[logging]
level = "info"
format = "json"
outputs = ["file"]
capture_io = false

[subscription_keepalive]
enabled = false

[providers.codex]
type = "codex"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$tmp/config.toml" &
pid="$!"
trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; rm -rf "$tmp" "$tmpbin"' EXIT
for _ in $(seq 1 100); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] && curl --silent --fail --unix-socket "$sock" \
    http://ilonasin/_ilonasin/manage/health >/dev/null; then
    break
  fi
  sleep 0.1
done
test -n "${sock:-}"
timeout 3s script -q -e -c "stty cols 100 rows 40; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null || true
kill "$pid"
wait "$pid" 2>/dev/null || true
rm -rf "$tmp" "$tmpbin"
```

Temporary behavior smokes:

- fake-upstream or in-package smoke for `X-Api-Key` auth and `/v1/messages`;
- invalid requests return Anthropic-shaped errors;
- non-stream success returns `type: "message"`;
- stream success returns `Content-Type: text/event-stream` and emits ordered
  Anthropic events: `message_start`, `content_block_start`,
  `content_block_delta` with `text_delta` or `input_json_delta`,
  `content_block_stop`, `message_delta` with `stop_reason`, and
  `message_stop`;
- tool loop smoke covers `tools`, assistant `tool_use` with stable
  `id/name/input`, follow-up `tool_result`, Chat `tool_call_id` mapping, and
  local rejection of unmatched or duplicate `tool_result`;
- for Codex, Anthropic `max_tokens`, `temperature`, `top_p`, `top_k`,
  `stop_sequences`, and `metadata` are validated or ignored without setting
  Chat fields or present flags that Codex validation rejects.

Live Claude Code smoke:

```sh
token_json="$(curl --silent --fail --unix-socket "$sock" \
  -H 'Content-Type: application/json' \
  --data '{"label":"claude-code-anthropic-smoke"}' \
  http://ilonasin/_ilonasin/manage/local-tokens)"
token="$(printf '%s' "$token_json" | jq -r '.token')"
token_id="$(printf '%s' "$token_json" | jq -r '.metadata.id')"
env -u ANTHROPIC_AUTH_TOKEN \
  ANTHROPIC_BASE_URL="http://127.0.0.1:<port>" \
  ANTHROPIC_API_KEY="$token" \
  CLAUDE_CONFIG_DIR="<tmp-claude>" \
  claude --bare --no-session-persistence -p --model codex/gpt-5.5 \
  "Reply with one short sentence."
curl --silent --fail --unix-socket "$sock" \
  -H 'Content-Type: application/json' \
  --data "{\"id\":$token_id}" \
  http://ilonasin/_ilonasin/manage/local-tokens/disable >/dev/null
```

The live smoke should use the existing configured ilonasin database for Codex
OAuth credentials, with temporary config overriding only bind/log/cache behavior
and `capture_io = false`. A fully temporary `ILONASIN_HOME` requires a Codex
device login through `/_ilonasin/manage/oauth-device-login/start` and
`/complete` before the Claude smoke.

After smoke, disable the fresh local token and scan temp logs, metadata, and
captured terminal output for local token text, prompt markers, completion
markers, bearer-looking secrets, raw request bodies, raw response bodies, tool
arguments, tool results, full account IDs, and provider request IDs.

Use a shell trap for live smokes that kills the daemon, disables the fresh token
when one was created, removes temporary Claude state, and removes temporary log
and cache directories.

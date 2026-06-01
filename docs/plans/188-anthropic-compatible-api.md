# 188 Anthropic Compatible API

## Goal

Expose a minimal Anthropic Messages-compatible API surface so Claude Code can
talk to ilonasin as a local gateway and route requests to `codex/gpt-5.5`.

## Ground Truth

- `docs/ilonasin-architecture.md` requires local API auth, upstream provider
  credentials, provider adapters, routing, HTTP transport, TUI, config, and
  SQLite storage to remain separate boundaries.
- Claude Code docs say `ANTHROPIC_BASE_URL` points at a gateway root and
  Claude Code sends Anthropic-format inference requests to `/v1/messages`.
- Claude Code docs say `ANTHROPIC_API_KEY` is sent as `X-Api-Key`, and
  `ANTHROPIC_AUTH_TOKEN` is sent as an Authorization bearer token.
- Anthropic Messages requests require `model`, `messages`, and `max_tokens`.
  Streaming uses events such as `message_start`, `content_block_start`,
  `content_block_delta`, `content_block_stop`, `message_delta`, and
  `message_stop`.
- Existing ilonasin Responses support already reuses the Chat Completions
  execution path for credential selection, 401 recovery, quota observations,
  fallback metadata, and safe usage recording.

## Scope

1. Add authenticated `POST /v1/messages`.
2. Accept local auth from either `Authorization: Bearer <ilonasin_token>` or
   `X-Api-Key: <ilonasin_token>` so Claude Code works without a wrapper. This
   intentionally broadens local client-token auth parsing for all local API
   routes, while still verifying against the same ilonasin token store.
3. Decode a strict subset of Anthropic Messages requests:
   - required `model`;
   - required positive `max_tokens`;
   - `messages` with `user`, `assistant`, and `system` roles;
   - top-level `system` as a string or array of text blocks;
   - text content blocks;
   - image content blocks with URL sources, translated to existing OpenAI-style
     image URL content;
   - `stream`, `temperature`, `top_p`, `top_k`, `stop_sequences`;
   - tool definitions shaped as Anthropic `custom` tools, translated to Chat
     Completions function tools when representable;
   - tool result content blocks, translated to Chat Completions tool messages
     when they match prior assistant tool uses.
4. Reject unsupported Anthropic fields and unsupported content blocks with clear
   local errors before upstream dispatch.
5. Translate Anthropic model strings with the existing ilonasin model addressing
   contract when clients can send slash-addressed model IDs. Claude Code
   validates model names locally, so slash-addressed IDs never reach the
   daemon. For Claude Code only, allow an Anthropic-route fallback from any
   Claude-shaped model name to `gpt-5.5` on the single configured Codex provider
   instance. If multiple Codex provider instances exist, keep returning the
   normal model-addressing error instead of guessing.
6. Reuse `executeNonStreamingChat` for the first implementation. This keeps
   credential routing, OAuth refresh, fallback, quota observations, and request
   metadata identical to existing chat and responses routes.
   For `stream: true`, this slice emits buffered Anthropic SSE after the
   upstream call completes. It is client-stream compatibility, not upstream
   streaming.
7. Return Anthropic-compatible non-streaming JSON when `stream` is false.
8. Return Anthropic-compatible SSE when `stream` is true, using one text block
   or one or more tool-use blocks derived from the upstream chat result.
9. Add metadata endpoint labeling for Anthropic Messages without storing raw
   prompts, completions, request bodies, response bodies, tool args/results, raw
   SSE chunks, bearer tokens, full account IDs, or provider request IDs.
10. Run a Claude Code smoke with temporary Claude state, `ANTHROPIC_BASE_URL`
    pointed at the local ilonasin server, `ANTHROPIC_API_KEY` set to a fresh
    local client token, and `--model sonnet`, which Claude Code accepts locally
    before the daemon maps the request to Codex `gpt-5.5`.
11. For Codex provider routing, validate Anthropic `max_tokens` locally and
    keep it in request metadata, but do not forward it to the Codex adapter.
    Also strip `temperature`, `top_p`, `top_k`, and `stop_sequences` before
    Codex dispatch because the current Codex provider validation rejects those
    Chat Completions fields.

## Non-Goals

- No `/v1/messages/count_tokens` endpoint in this slice unless Claude Code
  blocks on it during the smoke.
- No Anthropic prompt caching, container, metadata, beta context management, web
  search, server tools, thinking blocks, citations, files, or MCP pass-through.
- No native Anthropic provider adapter.
- No persistent tests.
- No config mutation from the TUI or Claude smoke.
- No changes to the subscription keepalive behavior.
- No changes to the concurrent logging work in the main worktree.
- No true upstream streaming in this slice.

## Implementation Notes

- Put Anthropic wire decoding and encoding in `internal/openai` for this slice
  because existing local wire DTOs, validation helpers, and Chat conversion
  helpers already live there. Revisit a dedicated package when there is more
  than this narrow compatibility shim.
- Keep Anthropic auth acceptance inside the server auth boundary, not provider
  credential resolution.
- Use `json.Decoder.UseNumber` where request fields can carry numbers.
- Preserve raw tool arguments and tool results only in memory for request
  translation and client response emission.
- For Claude Code compatibility, support both plain text and array content
  forms for `messages[].content`.
- If the upstream returns tool calls, render Anthropic `tool_use` content blocks
  with parsed JSON input when possible, or `{}` for empty arguments.
- If the upstream returns both text and tool calls, preserve both in order with
  text first, matching the information available from Chat Completions.
- Log only static route labels and safe metadata scalars.

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
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
rm -rf "$tmp" "$tmpbin"
```

Before the Claude Code smoke, run direct HTTP checks against the temporary
server:

- unauthenticated `/v1/messages` returns an Anthropic-shaped auth error;
- `X-Api-Key` and `Authorization: Bearer` both authenticate;
- missing `max_tokens` is rejected;
- unknown fields are rejected;
- non-stream JSON response has `type: "message"` and Anthropic `usage`;
- stream response emits `message_start`, `content_block_start`,
  `content_block_delta`, `content_block_stop`, `message_delta`, and
  `message_stop`;
- tool-use and tool-result conversion is exercised with fake upstream or a
  controlled live-compatible request where feasible;
- SQLite metadata and logs are scanned for sentinel prompt text, completion
  text, tool arguments, tool results, bearer tokens, raw request bodies, raw
  response bodies, and raw SSE chunks unless IO logging was explicitly enabled
  for the request.

Then start a temporary server against existing real ilonasin credentials with
temporary logs/cache where needed, create a fresh local client token through the
management API, run:

```sh
ANTHROPIC_BASE_URL="http://127.0.0.1:<port>" \
ANTHROPIC_API_KEY="<local-token>" \
CLAUDE_CONFIG_DIR="<tmp-claude>" \
claude --bare --no-session-persistence -p --model sonnet "Reply with one short sentence."
```

Clean up the token and temporary directories afterward.

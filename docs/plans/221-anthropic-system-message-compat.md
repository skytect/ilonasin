# 221 Anthropic System Message Compatibility

## Context

The live Claude Code wrapper targets the running ilonasin daemon at
`http://127.0.0.1:11435` and sends `POST /v1/messages?beta=true`.

Running:

```sh
claude -p "hi"
```

currently fails with:

```text
API Error: 400 messages[1].role is unsupported
```

The daemon logs show local auth succeeds and the Anthropic route rejects the
request during decode. A structural IO-log summary of the request shows:

- model: `pragnition-codex/gpt-5.5`;
- top-level `system` is an array;
- `messages` roles are `user`, then `system`;
- stream is `true`;
- tools are present.

The current decoder in `internal/anthropic/types.go` only accepts `user` and
`assistant` message roles. Current Anthropic TypeScript SDK docs expose
`MessageParam.role` as `user`, `assistant`, or `system`, so the decoder is
stale for Claude Code.

The worktree currently contains unrelated uncommitted auth-retry changes in:

- `internal/server/chat_nonstream.go`;
- `internal/server/chat_stream.go`;
- `internal/server/credentials.go`.

This slice must not modify or stage those files.

## Goal

Accept Anthropic `system` messages inside the `messages` array and translate
them into safe OpenAI/Codex system instruction messages without changing the
rest of Anthropic request handling.

## Scope

1. Update `internal/anthropic/types.go` to allow message role `system`.
2. Decode `system` message content with the same rules as top-level system:
   - string content;
   - text block arrays;
   - `cache_control` accepted and stripped;
   - non-text blocks rejected.
3. Translate each Anthropic `system` message into an OpenAI Chat message:
   - role `system`;
   - content as joined text;
   - preserve message ordering relative to other messages.
4. Keep top-level `system` behavior unchanged: it is still prepended as an
   OpenAI Chat `system` message when present.
5. Preserve existing handling for:
   - user text/image/tool_result blocks;
   - assistant text/tool_use blocks;
   - tools and `tool_choice`;
   - request controls accepted and stripped for Codex;
   - metadata-only request recording;
   - buffered Anthropic SSE.
6. Do not add a special Claude-only branch. This is Anthropic Messages schema
   compatibility.
7. Do not change server routes, auth, provider adapters, storage, management,
   TUI, config, IO logging policy, schema, or public route names.
8. Do not modify or stage unrelated dirty files.

## Non-Goals

- No complete Anthropic Messages implementation in this slice.
- No `/v1/messages/count_tokens`.
- No native Anthropic upstream provider.
- No true upstream streaming.
- No TUI work.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
tmp=$(mktemp -d)
tmpbin="$tmp/bin"
mkdir -p "$tmpbin"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
port=$(python - <<'PY'
import socket
s=socket.socket()
s.bind(('127.0.0.1',0))
print(s.getsockname()[1])
s.close()
PY
)
cat >"$tmp/config.toml" <<EOF
[server]
bind = "127.0.0.1:$port"

[paths]
database = "$tmp/home/ilonasin.sqlite"
log_dir = "$tmp/home/logs"
cache_dir = "$tmp/home/cache"

[logging]
capture_io = false

[subscription_keepalive]
enabled = false

[providers.deepseek]
type = "deepseek"

[providers.codex]
type = "codex"
EOF
cleanup() {
  if [ -n "${pid:-}" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$tmp/config.toml" >"$tmp/serve.log" 2>&1 &
pid=$!
for i in $(seq 1 50); do
  if [ -d "$tmp/home/run" ] && find "$tmp/home/run" -name 'manage-*.sock' -type s | rg . >/dev/null; then
    break
  fi
  sleep 0.1
done
sock="$(find "$tmp/home/run" -name 'manage-*.sock' -type s | head -n 1)"
test -S "$sock"
snapshot="$(curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot)"
printf '%s' "$snapshot" | jq -e '.providers | length >= 2' >/dev/null
timeout 3s script -q -e -c "stty cols 140 rows 45; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >/dev/null || true
```

Temporary focused smokes:

- decode a request containing top-level `system`, then user, system, and
  assistant messages;
- prove the translated Chat request has exact message order: top-level
  `system`, user, message-array `system`, assistant;
- prove translated `system` message content is valid JSON string content;
- prove `cache_control` on system text blocks is accepted, stripped, and does
  not appear in the translated Chat message;
- prove `system` message image/tool blocks are rejected at the Anthropic
  compatibility boundary before provider dispatch;
- prove existing unsupported roles remain rejected.

Remove temporary smoke files before commit.

Live smoke:

Run the wrapper command against the current configured daemon:

```sh
claude -p "hi"
```

Acceptance for this slice is that the previous local decode failure string
`messages[1].role is unsupported` is absent. A later failure is acceptable only
if logs prove it occurs after Anthropic decode and is a distinct compatibility
gap to address in the next Anthropic plan.

## Acceptance

- Anthropic `system` messages decode successfully.
- Anthropic `system` messages translate to OpenAI Chat `system` messages.
- Existing user, assistant, tool, and top-level system behavior is unchanged.
- Temporary focused smokes, compile, vet, serve smoke, manage smoke, and the
  live Claude Code smoke gate pass as defined above.

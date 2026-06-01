# 232 Anthropic Count Tokens Compatibility

## Context

`docs/ilonasin-architecture.md` separates the local API surface, strict request
validation, routing, provider adapters, metadata recording, and TUI. The local
Anthropic Messages compatibility surface currently exposes `POST /v1/messages`
only.

A direct smoke with Claude Code 2.1.159 against a repo-built daemon succeeds:

```text
Hi! How can I help?
```

The user-facing `claude -p "hi"` wrapper still failed against port `11435`
because the wrapper pins `ANTHROPIC_BASE_URL=http://127.0.0.1:11435`, where the
running/profile daemon was older than repo head. That is a daemon deployment
problem, not a `/v1/messages` compatibility failure in this source tree.

The next missing Anthropic-compatible route in this source tree is
`POST /v1/messages/count_tokens`. Anthropic's API shape accepts a Messages-like
request and returns an object containing `input_tokens`. Ilonasin has no
provider-neutral exact tokenizer or upstream Anthropic provider boundary, so
this slice should provide a conservative local compatibility estimate instead
of pretending to return exact provider counts.

## Goal

Add authenticated `POST /v1/messages/count_tokens` compatibility so Anthropic
clients that probe token counts get an Anthropic-shaped success response, while
the route remains clearly local and metadata-only.

## Scope

1. Register `POST /v1/messages/count_tokens`.
2. Accept local ilonasin tokens via `Authorization: Bearer <token>` and
   `X-Api-Key: <token>` on the count-tokens route, matching `/v1/messages`.
3. Update the Anthropic-route predicate in `internal/server/auth.go` so both
   `/v1/messages` and `/v1/messages/count_tokens`:
   - accept `X-Api-Key` as a local ilonasin token when no bearer token is
     present;
   - return Anthropic-shaped authentication errors;
   - get a concrete route label instead of `unknown`.
4. Reuse the existing Anthropic request field/content parsing where practical,
   but add a count-tokens decode path that does not require generation-only
   `max_tokens`. Keep `DecodeRequest` for `/v1/messages` unchanged so normal
   Messages requests still require `max_tokens`.
5. Reuse model alias resolution so supported model addressing and Claude-family
   fallback rules stay consistent with `/v1/messages`.
6. Add a small `internal/anthropic` count response helper:
   - response JSON shape: `{"input_tokens": <positive integer>}`;
   - count text, system text, tool names/descriptions/schema bytes, tool-use
     IDs/names/input, tool-result text, image URL strings, and small structural
     overhead;
   - do not call provider adapters or upstream network;
   - do not store raw prompts, completions, request bodies, tool arguments, or
     tool results in normal metadata.
7. Record metadata under a new endpoint label such as
   `anthropic_count_tokens` with:
   - client token ID;
   - provider/model resolution;
   - message count, tool count, image count;
   - HTTP status and error class;
   - total latency;
   - `prompt_tokens` set to the estimated input token count on success.
8. Keep IO logging behavior unchanged: request and response bodies may be logged
   only when `[logging].capture_io = true`, through the existing scrubber.
9. Return Anthropic-shaped errors on invalid JSON, unsupported fields, invalid
   model, provider-not-configured, and unsupported request shapes.
10. Do not change `/v1/messages` behavior, provider adapters, storage schema,
   TUI layout, subscription usage, keepalive, or config.
11. Do not add permanent tests.

## Non-Goals

- No exact Anthropic tokenizer.
- No upstream token-count calls.
- No native Anthropic provider.
- No `/v1/messages` behavior change.
- No TUI work in this slice.

## Verification

Run temporary focused smokes, then remove them before commit:

- decode and estimate a request with top-level system, text messages, tools,
  `tool_use`, `tool_result`, image URL source, and cache-control fields;
- prove repeated runs return a positive, deterministic `input_tokens`;
- prove invalid requests return Anthropic-shaped errors;
- prove `X-Api-Key` auth works on `/v1/messages/count_tokens`;
- prove `Authorization: Bearer` auth still works on `/v1/messages/count_tokens`;
- prove count-token requests do not require `max_tokens`, while `/v1/messages`
  still requires it;
- prove `routeLabel` returns `v1_messages_count_tokens`;
- prove success metadata records `Endpoint=anthropic_count_tokens`,
  `PromptTokens=input_tokens`, `TotalTokens=input_tokens`, `CompletionTokens=0`,
  `MessageCount`, `ToolCount`, `ImageCount`, `HTTPStatus=200`, empty
  `ErrorClass`, and non-zero total latency;
- prove one invalid/error request records `Endpoint=anthropic_count_tokens`,
  `HTTPStatus=400`, a non-empty `ErrorClass`, message/tool/image counts where
  available, and does not store payload text;
- prove `/v1/messages` direct Claude Code smoke still succeeds against a
  repo-built daemon.

Then run:

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
cat >"$tmp/config.toml" <<EOF2
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
EOF2
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
token="$(curl --silent --fail --unix-socket "$sock" -X POST http://ilonasin/_ilonasin/manage/local-tokens -d '{"label":"count-tokens-smoke"}' | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
test -n "$token"
curl --silent --fail \
  -H "X-Api-Key: $token" \
  -H "Content-Type: application/json" \
  --data '{"model":"codex/gpt-5.5","messages":[{"role":"user","content":"hello"}]}' \
  "http://127.0.0.1:$port/v1/messages/count_tokens" | jq -e '.input_tokens > 0' >/dev/null
timeout 3s script -q -e -c "stty cols 140 rows 45; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >/dev/null || true
```

Live smoke after local checks:

- start a repo-built daemon on a temporary port with the real SQLite database,
  temporary logs/cache, exactly one Codex provider instance, and
  `capture_io = false`;
- create a fresh local token through the management socket;
- run the underlying Claude Code binary directly with `--bare`,
  `ANTHROPIC_BASE_URL=http://127.0.0.1:<port>`, and
  `ANTHROPIC_API_KEY=<token>`;
- disable the token and remove temporary Claude state.

Acceptance for the live smoke is that `/v1/messages` still returns text and no
new count-tokens route change regresses Claude Code.

## Acceptance

- `POST /v1/messages/count_tokens` returns Anthropic-shaped
  `{"input_tokens": n}` responses for supported requests.
- The route is authenticated like `/v1/messages`.
- The estimate is deterministic and clearly local.
- Metadata remains payload-free.
- `/v1/messages` behavior is unchanged.
- Compile, vet, serve smoke, manage smoke, focused count-token smokes, direct
  Claude Code smoke, whitespace checks, and three implementation reviews pass.

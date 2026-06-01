# 219 Chat Request Metadata Base Split

## Context

`docs/ilonasin-architecture.md` treats request metadata as a metadata-only
side-plane for endpoint, routing, usage, latency, retry, fallback, and safe
request-shape fields. Chat Completions metadata records only scalar counts,
safe provider/model routing labels, safe option labels, and token-limit counts;
it does not store prompts, completions, request bodies, response bodies, raw
provider payloads, raw stream chunks, tool arguments/results, full bearer
tokens, full provider request IDs, or full account IDs.

Plans 210 through 218 split all other request metadata helpers and the
Responses base constructor into focused files. `request_metadata.go` now only
contains `requestMetadataBase`, the Chat/Anthropic-compatible base constructor.
Keeping a generic file for one Chat-specific constructor no longer adds
structure. Moving it to `request_metadata_chat.go` makes the metadata files
match their concerns.

The worktree currently contains unrelated uncommitted changes in
`internal/server/chat_nonstream.go`, `internal/server/chat_stream.go`, and
`internal/server/credentials.go`. This slice must not modify or stage those
files.

## Goal

Move Chat request metadata base construction into a focused file without
changing any recorded Chat or Anthropic-compatible metadata field.

## Scope

1. Add `internal/server/request_metadata_chat.go`.
2. Move `requestMetadataBase` from `request_metadata.go` into the new file.
3. Delete `request_metadata.go` if it becomes empty.
4. Preserve exact field mapping:
   - started-at time;
   - client token ID;
   - caller-supplied endpoint;
   - caller-supplied stream flag;
   - sanitized provider type;
   - message count from `len(req.Messages) + len(req.CodexResponsesInput)`;
   - tool count from `len(req.Tools) + len(req.CodexResponsesTools)`;
   - image count from `countRequestImages(req)`;
   - requested/resolved provider instance and model from the model address;
   - max output tokens through `requestedMaxOutputTokens(req)`;
   - safe provider option metadata through `applySafeOptionMetadata`.
5. Keep all call sites unchanged in:
   - Chat route early metadata;
   - non-streaming chat recording;
   - streaming chat recording and stream-unavailable metadata;
   - Anthropic Messages early metadata.
6. Do not change the Responses metadata constructor, route behavior, request
   validation, provider translation, option sanitizers, image counting,
   token-limit extraction, endpoint constants, finalization, quota
   observations, throughput math, storage, management, TUI, config, IO
   logging, schema, or public route shape.
7. Do not modify or stage unrelated dirty files.

## Non-Goals

- No behavior change.
- No schema change.
- No new metadata fields.
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

Also run a temporary focused in-package smoke proving `requestMetadataBase`
still populates endpoint, stream flag, counts, sanitized provider type,
requested/resolved model routing fields, max output tokens, and safe option
metadata.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- the moved constructor is behavior-equivalent;
- all Chat, streaming, and Anthropic call sites still call it;
- `request_metadata.go` is removed only if empty;
- no route, provider, storage, management, TUI, config, schema, IO logging, or
  unrelated dirty files changed.

## Acceptance

- Chat request metadata base construction lives in
  `request_metadata_chat.go`.
- Chat and Anthropic-compatible metadata field values are unchanged.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass, or any failure is proven to come from unrelated pre-existing dirty
  work.

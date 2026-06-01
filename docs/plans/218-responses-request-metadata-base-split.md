# 218 Responses Request Metadata Base Split

## Context

`docs/ilonasin-architecture.md` treats request metadata as a metadata-only
side-plane for endpoint, routing, usage, latency, retry, fallback, and safe
request-shape fields. The local Responses route, added for Codex-compatible
clients, records metadata without storing prompts, completions, request bodies,
response bodies, raw provider payloads, raw stream chunks, tool arguments,
tool results, full bearer tokens, full provider request IDs, or full account
IDs.

Plans 210 through 217 split most request metadata helpers into focused files.
`request_metadata.go` now contains both the Chat Completions base constructor
and the Responses base constructor. The Responses constructor has its own
endpoint, stream, message/tool/image-count, provider/model, service-tier, and
reasoning metadata behavior, so it can move to a focused file without changing
runtime behavior.

The worktree currently contains unrelated uncommitted changes in
`internal/server/chat_nonstream.go`, `internal/server/chat_stream.go`, and
`internal/server/credentials.go`. This slice must not modify or stage those
files.

## Goal

Move Responses request metadata base construction into a focused file without
changing any recorded Responses metadata field.

## Scope

1. Add `internal/server/request_metadata_responses.go`.
2. Move `responsesRequestMetadataBase` from `request_metadata.go` into the new
   file.
3. Preserve exact field mapping:
   - started-at time;
   - client token ID;
   - endpoint `metadataEndpointResponses`;
   - `Stream: true`;
   - sanitized provider type;
   - message count from `len(req.Input)`;
   - tool count from `len(req.Tools)`;
   - image count from `countResponsesImages(req)`;
   - requested/resolved provider instance and model from the model address;
   - allowlisted `service_tier`;
   - allowlisted `reasoning.effort`;
   - allowlisted `reasoning.summary`.
4. Keep all call sites unchanged in:
   - `handleResponses`;
   - `recordResponsesEarly`.
5. Do not change the Chat Completions base constructor, Responses route
   behavior, Responses decoder, request validation, provider translation,
   option sanitizers, image counting, token-limit extraction, endpoint
   constants, finalization, quota observations, throughput math, storage,
   management, TUI, config, IO logging, schema, or public route shape.
6. Do not modify or stage unrelated dirty files.

## Non-Goals

- No behavior change.
- No schema change.
- No new Responses metadata fields.
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

Also run a temporary focused in-package smoke proving
`responsesRequestMetadataBase` still populates endpoint, stream flag, counts,
sanitized provider type, requested/resolved model routing fields, allowlisted
service tier, allowlisted reasoning effort/summary, and ignores invalid
allowlisted fields.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- the moved constructor is behavior-equivalent;
- `handleResponses` and `recordResponsesEarly` still call it;
- Chat metadata construction remains in `request_metadata.go`;
- no route, provider, storage, management, TUI, config, schema, IO logging, or
  unrelated dirty files changed.

## Acceptance

- Responses request metadata base construction lives in
  `request_metadata_responses.go`.
- Responses metadata field values are unchanged.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass, or any failure is proven to come from unrelated pre-existing dirty
  work.

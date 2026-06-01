# 211 Request Metadata Images Split

## Context

`docs/ilonasin-architecture.md` requires metadata-only observability and
forbids storing request bodies, response bodies, prompts, completions, raw
provider payloads, and raw stream chunks in normal operation. Image metadata is
allowed only as a count.

`internal/server/request_metadata.go` still mixes base metadata construction,
finalization, quota observation helpers, throughput helpers, and image-counting
logic for both Chat Completions and Responses inputs.

## Goal

Move request image-counting helpers into a focused file without changing what
is counted or causing any raw image URLs, content parts, or request bodies to be
stored.

## Scope

1. Add `internal/server/request_metadata_images.go`.
2. Move these helpers from `request_metadata.go` into the new file:
   - `countResponsesImages`;
   - `countRequestImages`;
   - `countRawResponseImages`.
3. Preserve exact counting behavior:
   - Chat message content parts of type `"image_url"` are counted;
   - malformed chat message content is ignored for counting;
   - Codex Responses input raw items count content parts with type
     `"input_image"`;
   - malformed raw Codex Responses input is ignored for counting;
   - Responses request input content parts with type `"input_image"` are
     counted;
   - non-image parts are ignored.
4. Keep `requestMetadataBase` and `responsesRequestMetadataBase` calling the
   same helpers for `ImageCount`.
5. Do not change request metadata base fields, option sanitization,
   finalization, quota observations, throughput helpers, route handlers,
   provider adapters, storage, management, TUI, config, IO logging, or public
   route shape.

## Non-Goals

- No behavior change.
- No schema change.
- No new image metadata fields.
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

Also run a temporary focused in-package smoke covering:

- chat text-only messages count as zero images;
- chat messages with multiple valid `image_url` parts count each image;
- malformed chat message content is ignored for image counting;
- Codex raw Responses input with multiple `input_image` parts is counted;
- malformed Codex raw Responses input is ignored;
- Responses request input with multiple `input_image` parts is counted;
- non-image Responses content parts are ignored;
- `requestMetadataBase` and `responsesRequestMetadataBase` continue to populate
  `ImageCount` from the moved helpers.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- moved functions are behavior-equivalent;
- `requestMetadataBase` still calls `countRequestImages`;
- `responsesRequestMetadataBase` still calls `countResponsesImages`;
- the moved helpers only return counts and do not store or log raw image data;
- no option sanitization, finalization, quota, throughput, route, execution,
  provider, storage, management, TUI, or IO logging code changed.

## Acceptance

- Request image-counting logic lives in `request_metadata_images.go`.
- Request metadata construction still records only image counts.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass.

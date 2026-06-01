# 214 Request Metadata Token Limit Split

## Context

`docs/ilonasin-architecture.md` allows metadata-only telemetry, including token
counts and usage fields, while forbidding prompts, completions, bodies, raw
provider payloads, tool arguments/results, and stream chunks in normal
operation. The current metadata ledger records the requested maximum output
token count as a safe numeric field.

Plan 024 introduced `max_completion_tokens` and established request validation
and provider-boundary translation. Later metadata work added
`MaxOutputTokens` as a safe request metadata field. `request_metadata.go` now
keeps `requestedMaxOutputTokens` alongside base constructors and chat
finalization. Plans 210 through 213 have already moved option metadata, image
counting, throughput math, and quota observation construction into focused
files.

Token-limit extraction is another distinct metadata concern: choosing the safe
numeric output-token cap that should be recorded for a request.

## Goal

Move requested output-token extraction into a focused metadata helper file
without changing validation, provider translation, or recorded metadata values.

## Scope

1. Add `internal/server/request_metadata_tokens.go`.
2. Move `requestedMaxOutputTokens` from `request_metadata.go` into the new
   file.
3. Preserve exact precedence:
   - if `MaxCompletionTokens` is present, return it;
   - otherwise, if `MaxTokens` is present, return it;
   - otherwise return `0`.
4. Keep `requestMetadataBase` assigning `MaxOutputTokens` through
   `requestedMaxOutputTokens`.
5. Do not change request parsing, request validation, mutual-exclusion rules,
   provider adapter translation, Responses metadata, Anthropic metadata,
   chat finalization, quota observations, image counting, option sanitization,
   throughput math, storage, management, TUI, config, IO logging, schema, or
   public route shape.

## Non-Goals

- No behavior change.
- No schema change.
- No new token-limit support.
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

Also run a temporary focused in-package smoke proving:

- no token-limit fields returns `0`;
- `MaxTokens` alone is returned;
- `MaxCompletionTokens` alone is returned;
- if both pointers are populated, `MaxCompletionTokens` wins, matching the
  current helper behavior even though normal validated requests reject both;
- `requestMetadataBase` still populates `MaxOutputTokens` from the helper.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- the helper is behavior-equivalent;
- `requestMetadataBase` still records `MaxOutputTokens`;
- OpenAI request validation and provider token-limit translation are unchanged;
- no storage, management, TUI, config, route, provider, Anthropic, Responses,
  or IO logging code changed.

## Acceptance

- Requested output-token extraction lives in
  `request_metadata_tokens.go`.
- Chat request metadata records the same `MaxOutputTokens` values as before.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass.

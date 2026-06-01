# 210 Request Metadata Options Split

## Context

`docs/ilonasin-architecture.md` treats metadata-only observability as a core
boundary. `internal/server/request_metadata.go` currently mixes base request
metadata construction, final request metadata finalization, quota observation
helpers, image counting, token counting, throughput helpers, and
provider-option sanitization.

Option sanitization is a distinct concern: it turns only safe, allowlisted chat
provider-option and Responses request-option values into metadata fields,
without persisting raw option bodies.

## Goal

Move request option metadata sanitization into a focused file without changing
which metadata fields are populated or which values are accepted/rejected.

## Scope

1. Add `internal/server/request_metadata_options.go`.
2. Move these helpers from `request_metadata.go` into the new file:
   - `applySafeOptionMetadata`;
   - `intFromMetadataNumber`;
   - `safeServiceTier`;
   - `safeReasoningEffort`;
   - `safeReasoningSummary`;
   - `safeThinkingType`.
3. Keep `safeMetadataToken` in `request_metadata.go` because base metadata uses
   it directly for provider type sanitization.
4. Preserve exact chat provider-option behavior:
   - top-level chat `service_tier` sets both requested and effective service
     tier when allowlisted;
   - Codex provider options can override requested service tier and set
     reasoning effort/summary;
   - DeepSeek provider options can set reasoning effort and thinking type;
   - OpenRouter provider options can set reasoning effort, reasoning max tokens,
     reasoning enabled, and reasoning exclude;
   - invalid or non-allowlisted values remain empty or unset.
5. Preserve exact Responses metadata behavior:
   - Responses `service_tier` sets requested service tier when allowlisted;
   - Responses `reasoning.effort` and `reasoning.summary` are allowlisted with
     the same sanitizers;
   - invalid or non-allowlisted Responses values remain empty.
6. Do not change request metadata base fields, response metadata finalization,
   image counting, token counting, quota observations, throughput helpers,
   route handlers, provider adapters, storage, management, TUI, config, or
   public route shape.

## Non-Goals

- No behavior change.
- No schema change.
- No new provider option support.
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

- top-level chat `service_tier` allowlist and invalid rejection;
- Codex option metadata for service tier, reasoning effort, and reasoning
  summary;
- DeepSeek option metadata for reasoning effort and thinking type;
- OpenRouter option metadata for effort, max tokens from `int`,
  integer-valued `float64`, and `json.Number`, plus enabled/exclude booleans;
- invalid OpenRouter `max_tokens` values are ignored;
- invalid service tier, reasoning effort, reasoning summary, and thinking type
  values are ignored.
- `responsesRequestMetadataBase` still records allowlisted Responses
  `service_tier`, `reasoning.effort`, and `reasoning.summary`, and ignores
  invalid values.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- moved functions are behavior-equivalent;
- `requestMetadataBase` still calls `applySafeOptionMetadata`;
- `responsesRequestMetadataBase` still uses the moved shared sanitizers for
  service tier and reasoning fields;
- `safeMetadataToken` remains available for base metadata;
- `request_metadata.go` retains image counting, quota observation, finalization,
  and throughput helpers unchanged;
- no route, execution, provider, storage, management, TUI, or IO logging code
  changed.

## Acceptance

- Provider option metadata sanitization lives in
  `request_metadata_options.go`.
- Request metadata construction and finalization behavior is unchanged.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass.

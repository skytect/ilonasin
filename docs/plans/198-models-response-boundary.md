# 198 Models Response Boundary

## Context

`docs/ilonasin-architecture.md` separates provider discovery, strict local API
response shaping, and Codex-compatible client metadata concerns. The current
`internal/server/models.go` handler still does all of these in one function:
model discovery orchestration, cache fallback, OpenAI `/models` response DTOs,
Codex model metadata DTOs, capability flag interpretation, and response
sorting.

The behavior is mostly aligned after recent credential-pool work, but the route
file is still carrying response-construction detail that can be isolated without
changing runtime behavior.

## Goal

Move `/models` response DTOs and response construction into a small server
boundary helper so `handleModels` stays focused on orchestration and cache
semantics.

## Scope

1. Add a new `internal/server/models_response.go`.
2. Move the OpenAI model row DTO, Codex model metadata DTO, reasoning-effort
   DTO, capability helpers, input-modality ordering, service-tier defaults, and
   `displayNameOrID` into that file.
3. Add a helper such as `modelsResponseFromMetadata(rows []provider.ModelMetadata)`
   that:
   - preserves the existing top-level JSON shape: `object`, `data`, `models`;
   - preserves namespaced OpenAI model IDs as
     `<provider_instance_id>/<provider_model_id>`;
   - preserves current Codex-compatible `models[]` metadata;
   - preserves deterministic ordering by provider instance ID, then model ID.
4. Keep `internal/server/models.go` responsible for:
   - loading cache rows;
   - resolving eligible credentials;
   - running live discovery;
   - recording model-discovery health;
   - replacing cache on successful live discovery;
   - falling back to cache and writing error envelopes.
5. Do not change provider adapters, credential pooling, route auth, cache
   schema, management DTOs, TUI, request logging, or any public response field.

## Non-Goals

- No behavior change.
- No new endpoints.
- No config, storage, migration, or management API changes.
- No permanent tests.
- No Codex/OpenRouter/DeepSeek compatibility changes.

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

Also run a focused local `/v1/models` smoke against a fake authenticated model
cache or fake provider if the extraction touches response fields in a way the
compile checks cannot cover.

## Acceptance

- `handleModels` is shorter and no longer contains inline response DTOs or
  Codex metadata construction.
- `/models` and `/v1/models` response shape remains unchanged.
- Compile, vet, serve smoke, manage smoke, and whitespace checks pass.

# Plan 197: Remove Provider Placeholder Capability

## Context

Early scaffold slices used `provider.Placeholder` to keep the Codex provider
visible while chat, model discovery, OAuth, and subscription support were still
being added. The current architecture describes provider instances through
their actual capabilities: API-key support, OAuth support, chat support, model
discovery support, and OAuth refresh support.

Codex is now a real built-in provider type for the implemented paths, but
`Placeholder` still exists in:

- provider defaults and instances,
- server route capability checks,
- upstream credential guards,
- management provider DTOs,
- TUI provider visibility filters.

Keeping this scaffold-era flag preserves legacy architecture in the live
provider model and forces special `placeholder but codex is allowed` exceptions.

## Scope

Remove `Placeholder` from the live provider capability model and from management
and TUI filtering. Keep actual capability fields as the source of truth.

## Plan

1. Remove `Placeholder` from `provider.Defaults` and `provider.Instance`.
2. Stop setting `Placeholder` for the Codex built-in.
3. Remove `placeholder` from `management.ProviderInstance` and snapshot
   conversion.
4. Simplify server route capability checks to use only `Chat`, `APIKey`,
   `OAuth`, adapter availability, and provider-specific validation.
5. Simplify upstream API-key and fallback-policy guards to use `APIKey` and
   `OAuth` capability fields only.
6. Simplify TUI and management visibility filters to stop checking
   `Placeholder`.
7. Preserve behavior for current built-ins:
   - DeepSeek and OpenRouter remain API-key providers.
   - Codex remains OAuth-only and not API-key visible.
   - Fallback-policy visibility still supports API-key providers and Codex OAuth.
8. Do not change config shape, storage schema, provider request transport,
   request validation, model discovery, subscription usage, or TUI visuals.

## Verification

Run:

```sh
if rg -n 'Placeholder|placeholder' internal cmd; then
  exit 1
fi
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
git diff --check
tmp=$(mktemp -d)
tmpbin="$tmp/bin"
mkdir -p "$tmpbin"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
port=$(python - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
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
printf '%s' "$snapshot" | jq -e '.providers[] | select(.id=="codex") | .oauth == true and .api_key == false and .chat == true'
printf '%s' "$snapshot" | jq -e 'all(.providers[]; has("placeholder") | not)'
timeout 3s script -q -e -c "stty cols 140 rows 45; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null || true
```

Also run a temporary focused source/API smoke if compile checks do not fully
cover a capability path, then remove it before commit.

## Non-Goals

- No new provider type.
- No storage schema change.
- No config migration.
- No TUI visual change.
- No permanent tests.

# 246 Codex Chat Service Tier

## Context

`docs/ilonasin-architecture.md` says Codex-style fast mode, reasoning effort,
and similar behavior should be represented through request fields where
practical, and provider adapters should translate strict common requests into
provider-specific requests.

Current code already routes Codex Chat Completions through the Codex Responses
adapter. That adapter supports `provider_options.codex.service_tier` and maps
`fast` to Codex `priority`, but `ValidateChatRequest` rejects the top-level
Chat Completions `service_tier` field for Codex because it is grouped with
OpenRouter-only fields.

`docs/codex-compatibility-audit.md` still calls out direct Codex Chat
Completions with reasoning/service tier as a failing path. Reasoning is already
available through `provider_options.codex.reasoning`; this slice should fix the
smaller clear boundary issue: top-level Chat `service_tier` for Codex direct
chat.

## Goal

Allow direct Codex Chat Completions requests to use the top-level
`service_tier` field and translate it through the existing Codex Responses
request path, without changing other provider behavior.

## Scope

1. Stop rejecting top-level `service_tier` for Codex Chat Completions.
2. Preserve current DeepSeek behavior: top-level `service_tier` remains
   unsupported.
3. Preserve current OpenRouter behavior: top-level `service_tier` is forwarded
   through `MarshalUpstreamChatRequest`.
4. In Codex Responses request construction, treat top-level `req.ServiceTier`
   as a Codex service-tier request when `provider_options.codex.service_tier`
   is absent. If both are present, preserve current metadata semantics and let
   `provider_options.codex.service_tier` win.
5. Keep existing Codex mapping behavior:
   - `default` means no upstream `service_tier`;
   - provider-options-only `fast` means upstream `priority`;
   - top-level `priority` and `flex` pass through if supported by the
     discovered model;
   - top-level `auto` and `scale` remain unsupported for Codex even though the
     shared OpenAI decoder accepts them.
6. Use the existing effective service-tier return path so request metadata can
   record the adapter-selected tier. For Codex `default`, no upstream
   `service_tier` is sent and the effective tier should not be recorded as a
   real upstream tier.
7. Add temporary focused smoke coverage, then remove it before commit. The
   smoke should use a fake Codex upstream and prove:
   - top-level `service_tier: "priority"` is accepted for Codex;
   - the upstream Codex Responses body contains `service_tier: "priority"`;
   - top-level `service_tier: "default"` is accepted and omitted upstream;
   - when both top-level `service_tier` and
     `provider_options.codex.service_tier` are present, provider options win;
   - unsupported top-level Codex tiers such as `auto` or `scale` fail locally
     with a clear unsupported-request error rather than being forwarded;
   - DeepSeek still rejects top-level `service_tier`.
8. Do not change management APIs, storage, TUI, subscription keepalive, model
   discovery credential selection, logging policy, or public routes beyond this
   validation/adapter translation fix.
9. Do not add permanent tests.

## Non-Goals

- No live real-credential Codex smoke in this slice.
- No reasoning field redesign.
- No model discovery pooling changes.
- No new provider options.
- No Responses API behavior change.
- No docs/audit status rewrite beyond this plan.

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
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null
timeout 4s script -q -e -c "stty cols 120 rows 32; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage.out" || true
rg "api|providers|usage|logs" "$tmp/manage.out" >/dev/null
```

Also run a temporary focused fake-upstream smoke and remove it before commit.

## Acceptance

- Codex direct Chat Completions accepts top-level `service_tier`.
- Codex direct Chat Completions serializes supported service tier into the
  upstream Codex Responses request.
- DeepSeek still rejects top-level `service_tier`.
- OpenRouter top-level `service_tier` behavior is unchanged.
- No management, storage, TUI, subscription keepalive, model discovery pooling,
  logging, or public-route shape changes.
- Compile, vet, build, serve smoke, manage smoke, focused fake-upstream smoke,
  and implementation review pass.

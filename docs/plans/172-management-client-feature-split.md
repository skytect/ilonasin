# 172 Management Client Feature Split

## Context

`docs/ilonasin-architecture.md` says `ilonasin manage` should be a client of the
daemon-owned local management API. The server-side management package is already
split by feature: tokens, upstreams, OAuth, pruning, snapshots, and subscription
usage live in separate files.

After Plan 171, the client type has the correct general name,
`management.Client`, but `internal/management/http_client.go` still mixes the
client transport with every feature method. That preserves a broad legacy
client file and makes future management API changes harder to keep modular.

## Goal

Split the management client feature methods into feature-specific files without
changing behavior.

After this slice:

- `http_client.go` owns the client type, constructor, transport helpers, and
  HTTP error mapping only.
- feature methods live beside their corresponding management feature files.
- all request paths, response DTOs, long-poll behavior, timeout behavior, and
  error mapping remain unchanged.

## Scope

1. Move local-token client methods into `internal/management/client_tokens.go`.
   - `ListLocalTokens`
   - `CreateLocalToken`
   - `DisableLocalToken`
2. Move snapshot client method into `internal/management/client_snapshot.go`.
   - `LoadManagementSnapshot`
3. Move upstream and fallback client methods into
   `internal/management/client_upstreams.go`.
   - `AddUpstreamAPIKey`
   - `DisableUpstreamCredential`
   - `EnableFallbackPolicy`
   - `DisableFallbackPolicy`
4. Move OAuth client methods into `internal/management/client_oauth.go`.
   - `StartOAuthDeviceLogin`
   - `CompleteOAuthDeviceLogin`
   - `RefreshOAuthCredential`
5. Move observability client methods into
   `internal/management/client_observability.go`.
   - `PruneTelemetry`
   - `GetSubscriptionUsage`
   - `RefreshSubscriptionUsage`
6. Keep `internal/management/http_client.go` limited to:
   - `Client`
   - `NewUnixClient`
   - `do`
   - `doWithClient`
   - `longPollClient`
   - `managementHTTPError`

## Out of Scope

- Changing management routes.
- Changing request or response DTOs.
- Changing timeout, long-poll, socket, transport, or error behavior.
- Changing TUI wiring.
- Changing storage, provider, config, logging, or subscription usage semantics.
- Adding permanent tests.

## Implementation Steps

1. Add the five feature-specific client files.
2. Remove moved methods from `http_client.go`.
3. Let `goimports` or `gofmt` remove any stale imports.
4. Review the diff before smoke checks and confirm this is a relocation-only
   change.
5. Run compile, vet, daemon, management route, TUI, and source-layout guards.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
pid=""
cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
cfg="$tmp/config.toml"
cat >"$cfg" <<'EOF'
[server]
bind = "127.0.0.1:0"
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" >"$tmp/serve.log" 2>&1 &
pid="$!"
for _ in $(seq 1 80); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null; then
    break
  fi
  sleep 0.1
done
if [ -z "${sock:-}" ]; then
  echo "management socket not found"
  cat "$tmp/serve.log"
  exit 1
fi
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null
set +e
printf '\tq' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage.typescript" 2>/dev/null || true
  exit "$manage_status"
fi
git diff --check
rg -n "func \(c Client\) (ListLocalTokens|CreateLocalToken|DisableLocalToken)" internal/management/client_tokens.go
rg -n "func \(c Client\) LoadManagementSnapshot" internal/management/client_snapshot.go
rg -n "func \(c Client\) (AddUpstreamAPIKey|DisableUpstreamCredential|EnableFallbackPolicy|DisableFallbackPolicy)" internal/management/client_upstreams.go
rg -n "func \(c Client\) (StartOAuthDeviceLogin|CompleteOAuthDeviceLogin|RefreshOAuthCredential)" internal/management/client_oauth.go
rg -n "func \(c Client\) (PruneTelemetry|GetSubscriptionUsage|RefreshSubscriptionUsage)" internal/management/client_observability.go
rg -n "type Client|func NewUnixClient|func \(c Client\) do\(|func \(c Client\) doWithClient|func \(c Client\) longPollClient|func managementHTTPError" internal/management/http_client.go
if rg -n "func \(c Client\) (ListLocalTokens|CreateLocalToken|DisableLocalToken|LoadManagementSnapshot|AddUpstreamAPIKey|DisableUpstreamCredential|EnableFallbackPolicy|DisableFallbackPolicy|StartOAuthDeviceLogin|CompleteOAuthDeviceLogin|RefreshOAuthCredential|PruneTelemetry|GetSubscriptionUsage|RefreshSubscriptionUsage)" internal/management/http_client.go; then
  echo "feature methods remain in http_client.go"
  exit 1
fi
```

## Acceptance

- Management client feature methods compile from feature-specific files.
- `http_client.go` no longer owns feature-specific management API methods.
- Public management route behavior is unchanged.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.
- The unrelated dirty `AGENTS.md` file is not staged or committed.

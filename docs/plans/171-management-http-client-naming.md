# 171 Management HTTP Client Naming

## Context

The target architecture says `ilonasin manage` should be a client of the
daemon-owned local management API. The current TUI already uses the management
socket client for snapshots, local tokens, upstream credentials, fallback
policies, OAuth, telemetry pruning, and subscription usage.

However, the concrete management client type is still named
`HTTPTokenClient`, and its constructor is still named `NewUnixLocalTokenClient`.
Those names matched an earlier local-token-only management surface, but they no
longer describe the current client boundary. The type is now the general local
management API client used by the TUI.

This stale naming is small, but it preserves legacy architecture language in
the public internal API and obscures the fact that the TUI is a management API
client rather than a direct token/storage client.

## Goal

Rename the management HTTP client to reflect its current role without changing
behavior.

After this slice:

- `management.Client` is the local management API client type.
- `management.NewUnixClient` constructs a Unix-socket management client.
- `ilonasin manage` uses the general management client naming.
- Existing methods and behavior remain unchanged.

## Scope

1. Rename `HTTPTokenClient` to `Client`.
2. Rename `NewUnixLocalTokenClient` to `NewUnixClient`.
3. Update receiver types in `internal/management/http_client.go`.
4. Update `internal/app/commands.go` to call `management.NewUnixClient`.
5. Rename the local variable in `Manage` from `tokenClient` to
   `managementClient`.
6. Do not change request paths, socket path logic, HTTP transport behavior,
   long-poll behavior, error mapping, method names, management DTOs, TUI
   interfaces, storage behavior, provider behavior, config, or tests.

## Out of Scope

- Splitting the management client by feature.
- Changing management route names.
- Changing socket identity or security behavior.
- Changing TUI model dependencies.
- Changing request/response DTOs.
- Storage or provider changes.
- Permanent tests.

## Implementation Steps

1. Rename the type and constructor in `internal/management/http_client.go`.
2. Update all method receivers in that file.
3. Update the constructor call and local variable in `internal/app/commands.go`.
4. Run `gofmt` on touched files.
5. Review the diff before running checks, with special attention that this is
   a naming-only change and no request path, timeout, long-poll, socket, or
   error behavior changed.

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
if rg -n "HTTPTokenClient|NewUnixLocalTokenClient|tokenClient" internal/management/http_client.go internal/app/commands.go; then
  echo "legacy token-specific management client naming remains"
  exit 1
fi
rg -n "type Client|func NewUnixClient|managementClient" internal/management/http_client.go internal/app/commands.go
```

## Acceptance

- The general management client is named `Client`.
- The Unix-socket constructor is named `NewUnixClient`.
- `ilonasin manage` uses `managementClient` naming.
- No request path, socket path, transport, timeout, long-poll, or error
  behavior changes.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

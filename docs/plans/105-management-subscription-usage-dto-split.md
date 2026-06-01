# 105 Management Subscription Usage DTO Split

## Context

Plan 104 added Codex subscription usage snapshots and management routes. The
route-specific response types currently live in `internal/management/snapshot.go`
beside the full management snapshot DTO and snapshot sanitizer.

That works, but it makes `snapshot.go` carry a feature-specific route surface.
The architecture target keeps management API operations modular, with daemon
routes, DTOs, and storage-facing interfaces grouped around the operation they
serve. Subscription usage already has its own management service file, so its
request and response DTOs should live there too.

## Goal

Move subscription-usage management DTOs out of `snapshot.go` and into the
subscription-usage management module, preserving JSON shape and behavior.

After this slice, `snapshot.go` still includes subscription usage in the full
management snapshot, but the route-specific DTO definitions are owned by
`subscription_usage.go`.

## Scope

1. Move these types from `internal/management/snapshot.go` to
   `internal/management/subscription_usage.go`:
   - `SubscriptionUsageRow`
   - `SubscriptionUsageAggregate`
   - `KeepaliveStatus`
   - `SubscriptionUsageResponse`
2. Keep field names, JSON tags, and type names unchanged.
3. Keep `ManagementSnapshotResponse.SubscriptionUsage` unchanged.
4. Keep the sanitizer in `snapshot.go` for now because it sanitizes the complete
   snapshot response.
5. Do not change Codex usage fetching, SQLite schema, refresh behavior,
   keepalive status semantics, TUI rendering, or management route paths.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No behavior changes.
- No new routes or config fields.
- No migration changes.
- No split of the broader snapshot sanitizer in this slice.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Move the four DTO type definitions into `subscription_usage.go` near the
   `SubscriptionUsageClient` interface.
2. Remove the definitions from `snapshot.go`.
3. Run `gofmt`.
4. Review the diff to confirm it is a type move only.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
cfg="$tmp/config.toml"
cat >"$cfg" <<EOF
[server]
bind = "127.0.0.1:0"
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" &
pid="$!"
for _ in $(seq 1 80); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] && curl --silent --fail --unix-socket "$sock" \
    http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null; then
    break
  fi
  sleep 0.1
done
kill "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
git diff --check
```

Acceptance:

- Compile/package check passes.
- Vet passes.
- Fresh binary builds.
- Direct `serve` smoke starts a daemon and exposes the subscription usage route
  over the management socket.
- `git diff --check` passes.

## Review Questions

1. Is keeping snapshot-wide sanitization in `snapshot.go` correct for this
   slice?
2. Does moving the DTOs into `subscription_usage.go` improve the management API
   operation boundary without hiding shared snapshot behavior?
3. Is a direct daemon/socket smoke enough for this behavior-preserving move?

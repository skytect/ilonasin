# Plan 195: Subscription Pool Window Reset DTO

## Context

Plan 194 removed legacy average/minimum fields from subscription pool DTOs so the
management contract reflects summative pooled usage. One duplicate pool field
shape remains: `SubscriptionUsageAggregate` still exposes
`earliest_primary_reset_at` and `earliest_secondary_reset_at`, while the TUI and
the current pool contract read reset times from each pool `windows[]` entry as
`earliest_reset_at`.

Keeping both top-level primary/secondary reset fields and per-window reset
fields preserves the old primary/secondary aggregate DTO model. The daemon-owned
management response should expose pool reset metadata at the same level as the
window totals that use it.

## Scope

Remove top-level primary/secondary reset fields from subscription pool
aggregates, and keep earliest reset metadata only on
`SubscriptionUsagePoolWindow`.

## Plan

1. Remove these fields from `SubscriptionUsageAggregate`:
   - `EarliestPrimaryResetAt`
   - `EarliestSecondaryResetAt`
2. Keep earliest reset calculations inside the private aggregation bucket.
3. Pass bucket-level earliest primary/secondary reset times into
   `subscriptionUsagePoolWindows`.
4. Keep `SubscriptionUsagePoolWindow.EarliestResetAt` unchanged.
5. Preserve sorting, stale counts, labels, window inclusion, future reset
   filtering, and total percent-point behavior.
6. Do not change TUI rendering, storage schema, config, provider behavior, auth,
   routing, or management routes.

## Verification

Run:

```sh
if rg -n 'EarliestPrimaryResetAt|EarliestSecondaryResetAt|earliest_primary_reset_at|earliest_secondary_reset_at' internal; then
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
timeout 3s script -q -e -c "stty cols 140 rows 45; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null || true
```

Also run a temporary focused management JSON smoke, then remove it before
commit. The smoke should seed rows with future primary and secondary resets,
marshal `SubscriptionUsageResponse`, and assert:

- pool JSON does not include `earliest_primary_reset_at` or
  `earliest_secondary_reset_at`,
- each pool window still includes `earliest_reset_at`.

## Non-Goals

- No TUI visual changes.
- No subscription storage schema changes.
- No management route additions.
- No permanent tests.

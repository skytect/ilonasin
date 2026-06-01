# Plan 194: Subscription Pool Total DTO

## Context

`docs/ilonasin-architecture.md` keeps management DTOs as the daemon-owned
contract consumed by the TUI. Recent TUI slices changed subscription pool
rendering to show summative pooled usage, not averages or minimum per-account
remaining values.

The management subscription usage DTO still carries legacy average/minimum
fields:

- `SubscriptionUsageAggregate.AveragePrimaryUsedPercent`
- `SubscriptionUsageAggregate.MinimumPrimaryRemainingPercent`
- `SubscriptionUsageAggregate.AverageSecondaryUsedPercent`
- `SubscriptionUsageAggregate.MinimumSecondaryRemainingPercent`
- `SubscriptionUsagePoolWindow.AverageUsedPercent`
- `SubscriptionUsagePoolWindow.MinimumRemainingPercent`

The TUI no longer reads them. Keeping them in the daemon response preserves the
old pooled-average model and makes the management contract less clear.

## Scope

Remove the legacy average/minimum pool fields from the management DTO and build
pool windows directly from total used points, total remaining points, total
capacity points, account count, stale count, and earliest reset.

## Plan

1. Remove average/minimum pool fields from:
   - `internal/management/subscription_usage_dto.go`
   - `internal/management/subscription_usage_response.go`
2. Keep account-level percentage fields unchanged.
3. Keep pool `Windows` shape with:
   - `kind`
   - `label`
   - `account_count`
   - `stale_count`
   - `total_used_percent_points`
   - `total_remaining_percent_points`
   - `total_capacity_percent_points`
   - `earliest_reset_at`
4. Preserve sorting, stale counts, window labels, future-reset filtering, and
   bounded percent-point behavior.
5. Do not change TUI rendering, storage schema, config, provider behavior, auth,
   or routing.
6. Add source checks proving live code no longer references the removed fields.

## Verification

Run:

```sh
if rg -n 'AveragePrimaryUsedPercent|AverageSecondaryUsedPercent|MinimumPrimaryRemainingPercent|MinimumSecondaryRemainingPercent|AverageUsedPercent|MinimumRemainingPercent' internal; then
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

Also run a temporary focused management-render smoke if the code change is not
obviously covered by compile checks, and remove it before commit.

The temporary focused smoke should seed two `metadata.SubscriptionUsageSnapshot`
rows for the same provider and limit, call the management subscription usage
response path, marshal it to JSON, and assert:

- pool windows still include `total_used_percent_points`,
  `total_remaining_percent_points`, and `total_capacity_percent_points`,
- pool JSON does not include `average_*` or `minimum_*` keys,
- total capacity is the account count multiplied by `100`.

## Non-Goals

- No TUI visual changes.
- No subscription storage schema changes.
- No management route additions.
- No permanent tests.

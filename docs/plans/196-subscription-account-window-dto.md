# Plan 196: Subscription Account Window DTO

## Context

Recent slices moved subscription pool usage to a window-first management DTO.
The account side still has the older primary/secondary fields duplicated next
to `SubscriptionUsageRow.Windows`, and the TUI keeps fallback code that
reconstructs account windows from those top-level fields when `Windows` is
empty.

That compatibility layer preserves the old primary/secondary DTO model in both
management and TUI code. The management API should expose subscription usage
windows through one account-window contract, and the TUI should render that
contract directly.

## Scope

Remove account-level primary/secondary subscription usage fields from the
management DTO, and make the TUI render account usage from
`SubscriptionUsageRow.Windows` only.

## Plan

1. Remove these fields from `SubscriptionUsageRow`:
   - `PrimaryLabel`
   - `PrimaryUsedPercent`
   - `PrimaryRemainingPercent`
   - `PrimaryWindowMinutes`
   - `PrimaryResetAt`
   - `SecondaryLabel`
   - `SecondaryUsedPercent`
   - `SecondaryRemainingPercent`
   - `SecondaryWindowMinutes`
   - `SecondaryResetAt`
2. Build `SubscriptionUsageRow.Windows` directly from
   `metadata.SubscriptionUsageSnapshot` in `subscriptionUsageRow`.
3. Update pool aggregation to iterate account `Windows` and bucket by window
   kind instead of reading removed account fields.
4. Remove the TUI fallback reconstruction in `subscriptionAccountWindowLines`.
5. Preserve account labels, totals, reset filtering, stale/error state, sorting,
   bounded percentage behavior, and pool total percent-point behavior.
6. Do not change SQLite storage schema, provider behavior, auth, routing, config,
   management route paths, or TUI visuals.

## Verification

Run:

```sh
if rg -n 'PrimaryLabel|PrimaryUsedPercent|PrimaryRemainingPercent|PrimaryWindowMinutes|PrimaryResetAt|SecondaryLabel|SecondaryUsedPercent|SecondaryRemainingPercent|SecondaryWindowMinutes|SecondaryResetAt' internal/management/subscription_usage_dto.go internal/tui/observability_subscription.go; then
  exit 1
fi
if rg -n '`json:"(primary|secondary)_(label|used_percent|remaining_percent|window_minutes|reset_at)' internal/management/subscription_usage_dto.go; then
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

Also run a temporary focused management/TUI smoke, then remove it before
commit. The smoke should seed a primary and secondary subscription snapshot,
marshal `SubscriptionUsageResponse`, and assert:

- account JSON has `windows[]` entries,
- account JSON does not contain primary/secondary top-level usage keys,
- pool windows still contain summed used, remaining, capacity, and reset fields,
- TUI account rendering uses the seeded `windows[]` data.

## Non-Goals

- No storage schema changes.
- No management route additions.
- No TUI visual redesign.
- No permanent tests.

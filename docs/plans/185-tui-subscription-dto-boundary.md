# 185 TUI Subscription DTO Boundary

## Context

`docs/ilonasin-architecture.md` says `ilonasin manage` should be a client of the
daemon-owned local management API, and the final codebase should not keep
residual legacy architecture. The subscription usage management response now
populates normalized account and pool `Windows`, including summed pool capacity
and remaining points.

`internal/tui/observability_subscription.go` still has fallback code that
reconstructs pool windows from old aggregate average fields when `row.Windows`
is empty. That path is named `legacyPool...`, renders from less precise average
fields, and preserves a stale DTO-compatibility layer in the TUI.

## Goal

Make the TUI subscription usage renderer consume the current management DTO
shape directly and remove the stale pool-window reconstruction helpers.

After this slice:

- subscription pool cards render only from `SubscriptionUsageAggregate.Windows`,
- empty aggregate windows render no pool gauge lines instead of reconstructing
  from average fields,
- `legacyPoolCapacityPoints` and `legacyPoolRemainingPoints` are removed,
- the TUI no longer carries compatibility code for pre-window aggregate DTOs,
- management DTOs, server routes, storage, config, and provider behavior are
  unchanged.

## Scope

1. Update `internal/tui/observability_subscription.go`.
   - Remove the `len(row.Windows) == 0` aggregate fallback in
     `subscriptionPoolWindowLines`.
   - Delete `legacyPoolCapacityPoints` and `legacyPoolRemainingPoints`.
2. Keep account-window fallback in `subscriptionAccountWindowLines`.
   - Account summary fields are still part of the current DTO and the fallback
     is local display convenience, not stale aggregate compatibility.
3. Add source guards to prove no `legacyPool` helper remains.
4. Do not change management response construction, DTO fields, storage,
   provider adapters, routes, config, key handling, or Bubble Tea update flow.
5. Do not add dependencies or permanent tests.

## Out of Scope

- Removing average/minimum aggregate DTO fields.
- Changing subscription usage API response JSON.
- Changing subscription account cards or keepalive cards.
- Further visual redesign.

## Implementation Steps

1. Remove the aggregate fallback and helper functions from
   `internal/tui/observability_subscription.go`.
2. Run `gofmt`.
3. Review the diff before smoke checks.
4. Run compile, vet, serve route smoke, seeded subscription usage TUI smoke,
   source guards, and whitespace checks.

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
sqlite3 "$tmp/home/ilonasin.sqlite" <<EOF
INSERT INTO provider_credentials(id, provider_instance_id, kind, label, created_at, updated_at)
VALUES
  (301, 'codex', 'oauth', 'alice', datetime('now'), datetime('now')),
  (302, 'codex', 'oauth', 'bob', datetime('now'), datetime('now'));
INSERT INTO subscription_usage_snapshots(
  observed_at, provider_instance_id, credential_id, account_display_label,
  plan_label, limit_id, limit_name, plan_type, reached_type,
  primary_used_percent, primary_window_minutes, primary_reset_at,
  secondary_used_percent, secondary_window_minutes, secondary_reset_at,
  source, error_class, stale
) VALUES
  (datetime('now'), 'codex', 301, 'alice@example.com',
   'Plus', 'codex', 'Codex', 'plus', '',
   40.0, 300, datetime('now', '+2 hours'), 25.0, 10080, datetime('now', '+3 days'),
   'seed', '', 0),
  (datetime('now'), 'codex', 302, 'bob@example.com',
   'Plus', 'codex', 'Codex', 'plus', '',
   80.0, 300, datetime('now', '+1 hours'), 50.0, 10080, datetime('now', '+2 days'),
   'seed', '', 0);
EOF
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >"$tmp/subscription.json"
rg -q '"total_remaining_percent_points"' "$tmp/subscription.json"
rg -q '"total_capacity_percent_points"' "$tmp/subscription.json"
set +e
printf '\t\t\033[4~q' | timeout 3s script -q -e -c \
  "sh -c 'stty cols 110 rows 80; exec env ILONASIN_HOME=\"$tmp/home\" \"$tmpbin/ilonasin\" manage --config \"$cfg\"'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage.typescript"
  exit "$manage_status"
fi
rg -q 'Subscription pools' "$tmp/manage.typescript"
rg -q 'pool remaining' "$tmp/manage.typescript"
rg -q '80.0/200.0' "$tmp/manage.typescript"
rg -q '125.0/200.0' "$tmp/manage.typescript"
if rg -n 'legacyPool|AveragePrimaryUsedPercent|AverageSecondaryUsedPercent' internal/tui/observability_subscription.go; then
  exit 1
fi
! rg -n '"ilonasin/internal/(provider|storage|credentials|config)"|bubbletea|log/slog' internal/tui/observability_subscription.go
git diff --check
```

## Acceptance

- The TUI subscription pool renderer no longer reconstructs windows from legacy
  aggregate averages.
- The seeded management response proves current aggregate windows include
  summed remaining and capacity fields.
- The seeded TUI smoke renders summative pool remaining values.
- Compile, vet, serve route smoke, manage PTY smoke, source guards, and
  whitespace checks pass.
- Existing unrelated files are not staged or committed.

# 176 TUI Subscription Usage Cards

## Context

The management TUI renders subscription usage as plain text rows. The user wants
the UI to stop feeling like tabs plus text, especially for subscription limits,
and wants account email/display labels visible enough to distinguish pooled
Codex subscription accounts.

OAuth login already extracts the Codex email from ID-token claims and stores it
as `AccountDisplayLabel` when safe. Subscription usage and provider account DTOs
already expose this display label. This slice should make that identity and the
5h/weekly usage windows visually obvious without changing storage or provider
behavior.

## Goal

Make the TUI subscription usage section visually scannable with Lipgloss cards
and progress bars, and make account identity labels prominent in account and
subscription rows.

## Scope

1. Update `internal/tui/observability_subscription.go`:
   - render subscription account rows as compact cards,
   - show account display labels prominently,
   - render 5h and weekly usage as bounded progress bars,
   - render reset times next to each bar,
   - render pooled account aggregates with average-used and minimum-left bars,
   - keep stale/error state visible.
2. Update `internal/tui/accounts_oauth.go`:
   - make OAuth and provider account display labels the primary visual identity,
   - keep credential ID, provider, plan, expiry, refresh, and disabled state.
3. Add small TUI render helpers where they are reused by account and
   subscription rendering, and keep percent bar formatting in the same visual
   helper boundary.
4. Preserve safe display handling for labels and status values. Account display
   labels may show safe email-like values, but must still redact secret,
   account-ID, request-ID, raw payload, balance, and credential markers.
5. Do not change management DTOs, SQLite, provider adapters, subscription
   keepalive execution, config, routing, or key handling.
6. Do not add permanent tests.

## Non-Goals

- No new subscription route fields.
- No storage migration for a separate email field.
- No changes to OAuth login parsing.
- No change to subscription usage fetching or pooling math.
- No broad TUI redesign outside account identity and subscription usage.

## Implementation

1. Add reusable helpers for account labels, progress bars, card borders, percent
   formatting, and reset labels.
2. Replace the current subscription text rows with account cards and pool cards.
3. Adjust OAuth/provider account rows so the label/email is visually first.
4. Bound card widths before viewport clipping so narrow terminals do not cut
   ANSI escape sequences or card borders mid-content.
5. Run `gofmt`.
6. Review the diff before smoke checks.
7. Run compile, vet, daemon, subscription route, seeded manage PTY smoke, and a
   narrow-width manage PTY smoke.

## Smoke Checks

Run a compile/vet/daemon smoke and seed the temporary SQLite database with two
Codex OAuth/provider accounts plus subscription snapshots before launching the
TUI. The smoke must verify both a tall viewport and a narrow viewport:

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
# Seed temp-only rows for render coverage:
# - provider_credentials and oauth_tokens for two Codex OAuth accounts,
# - provider_accounts with alice@example.com and bob@example.com display labels,
# - subscription_usage_snapshots with 5h and weekly windows for both accounts.
set +e
printf '\t\tq' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage.typescript" 2>/dev/null || true
  exit "$manage_status"
fi
grep -q "Subscription usage" "$tmp/manage.typescript"
grep -q "Subscription pools" "$tmp/manage.typescript"
grep -q "Subscription keepalive" "$tmp/manage.typescript"
grep -q "alice@example.com" "$tmp/manage.typescript"
grep -q "bob@example.com" "$tmp/manage.typescript"
grep -q "█" "$tmp/manage.typescript"
git diff --check
```

Also run a second manage PTY smoke with a narrow `COLUMNS` value and the same
seed data, asserting that account labels and bar glyphs still render.

## Acceptance

- Subscription usage uses bars/cards instead of plain list rows.
- Account display labels, including safe email labels when present, are
  prominent in account and subscription views.
- Compile, vet, daemon route checks, manage PTY smoke, and diff whitespace
  checks pass.
- Existing unrelated dirty files are not staged or committed.

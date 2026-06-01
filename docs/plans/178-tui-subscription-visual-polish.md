# 178 TUI Subscription Visual Polish

## Context

Plan 176 made subscription usage less plain by adding Lipgloss cards and bars,
but the observability tab still reads like boxed text. The user wants a more
imaginative Bubble Tea/Lipgloss treatment, including clearer usage bars and a
visible email or account display label so pooled Codex accounts are easy to
distinguish.

The existing management DTOs already expose safe account display labels, window
percentages, reset times, pooled aggregate windows, and keepalive status.
Codex docs and source notes still constrain this UI to percentages and reset
times, not exact remaining requests or tokens.

## Goal

Make the subscription usage section feel like a compact terminal dashboard
without changing provider, storage, route, or DTO behavior.

After this slice:

- subscription accounts render as richer usage cards with prominent identity,
  state, plan, provider, credential ID, observed time, and two visual window
  gauges,
- pooled usage renders as aggregate capacity cards with average-used and
  minimum-left indicators,
- account display labels, including safe email labels, are explicitly labeled
  and easy to scan,
- narrow terminals still render bounded ANSI-safe content.

## Scope

1. Update `internal/tui/visual_helpers.go`.
   - Add small reusable styles for badges, labels, values, card accents, and
     gauge rows.
   - Keep the existing safe display and ANSI width handling.
   - Keep progress bars deterministic and display-only. Do not add animation or
     new asynchronous Bubble Tea messages.
2. Update `internal/tui/observability_subscription.go`.
   - Render account cards with an explicit `email` or `identity` field derived
     from the existing safe account display label.
   - Prefer `row.Windows` when present so labels and reset data come from the
     window-oriented route shape.
   - Fall back to the legacy primary and secondary fields for compatibility.
   - Render pool cards from `row.Windows` when present and fall back to legacy
     aggregate fields.
   - Keep stale/error state prominent.
3. Update `internal/tui/accounts_oauth.go` only as needed to label account
   identity more clearly in OAuth and provider account cards.
4. Preserve all privacy boundaries.
   - Do not render full account IDs, bearer tokens, provider payloads,
     balances, credits, prompts, completions, request bodies, response bodies,
     raw SSE chunks, tool data, or provider request IDs.
   - Safe email-like account display labels may be rendered because they are
     already sanitized by management and TUI display helpers.
5. Do not change management DTOs, SQLite, provider adapters, subscription
   keepalive execution, config, routing, key handling, or dependencies.
6. Do not add permanent tests.

## Out of Scope

- Adding `bubbles/progress`, `bubbles/table`, or a different viewport in this
  slice. The current need is richer static rendering, not a new interactive
  component.
- New route fields for exact remaining quota.
- Storage migration for separate email fields.
- Any config mutation from the TUI.

## Implementation Steps

1. Add visual primitives for status badges, identity rows, metric chips, and
   usage gauges.
2. Rewrite subscription account and pool card composition around those helpers.
3. Make OAuth/provider account cards label the visible account display as
   `email` when it looks email-like, otherwise `identity`.
4. Run `gofmt`.
5. Review the diff before smoke checks.
6. Run compile, vet, daemon route checks, normal seeded manage PTY smoke, and
   narrow seeded manage PTY smoke.

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
db="$tmp/home/ilonasin.sqlite"
sqlite3 "$db" <<EOF
INSERT INTO provider_credentials(id, provider_instance_id, kind, label, created_at, updated_at)
VALUES
  (101, 'codex', 'oauth', 'alice', datetime('now'), datetime('now')),
  (102, 'codex', 'oauth', 'bob', datetime('now'), datetime('now'));
INSERT INTO subscription_usage_snapshots(
  observed_at, provider_instance_id, credential_id, account_display_label,
  plan_label, limit_id, limit_name, plan_type, reached_type,
  primary_used_percent, primary_window_minutes, primary_reset_at,
  secondary_used_percent, secondary_window_minutes, secondary_reset_at,
  source, error_class, stale
) VALUES
  (datetime('now'), 'codex', 101, 'alice@example.com', 'Plus', 'codex', 'Codex', 'plus', '',
   42.5, 300, datetime('now', '+2 hours'), 18.0, 10080, datetime('now', '+3 days'), 'seed', '', 0),
  (datetime('now'), 'codex', 102, 'bob@example.com', 'Pro', 'codex', 'Codex', 'pro', '',
   87.0, 300, datetime('now', '+1 hours'), 66.0, 10080, datetime('now', '+2 days'), 'seed', '', 0);
EOF
set +e
printf '\t\tq' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' COLUMNS=100 LINES=80 '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage-normal.typescript" >/dev/null
normal_status="$?"
printf '\t\tq' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' COLUMNS=50 LINES=80 '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage-narrow.typescript" >/dev/null
narrow_status="$?"
set -e
for status in "$normal_status" "$narrow_status"; do
  if [ "$status" -ne 0 ] && [ "$status" -ne 124 ]; then
    cat "$tmp"/manage-*.typescript
    exit "$status"
  fi
done
for capture in "$tmp/manage-normal.typescript" "$tmp/manage-narrow.typescript"; do
  grep -q "alice@example.com" "$capture"
  grep -q "bob@example.com" "$capture"
  grep -Eq "email|identity" "$capture"
  grep -q "█" "$capture"
  grep -q "Subscription pools" "$capture"
  grep -q "account-points" "$capture"
  if grep -Eiq "bearer|sk-|iln_|access_token|refresh_token|payload|prompt|completion|request_id|acct_" "$capture"; then
    cat "$capture"
    exit 1
  fi
done
git diff --check
```

The seeded PTY capture verifies:

- `alice@example.com` and `bob@example.com` appear,
- an `email` or `identity` label appears,
- the subscription section renders gauge glyphs,
- pool cards render aggregate labels,
- no forbidden secret or payload markers appear.

## Acceptance

- Subscription usage is visually richer than plain text rows, with dashboard
  cards and clear bars.
- Safe account email/display labels are visible on subscription and account
  cards.
- Existing route and storage behavior is unchanged.
- Compile, vet, daemon management route checks, seeded manage PTY smokes, and
  whitespace checks pass.
- `AGENTS.md` remains unstaged if it is still dirty.

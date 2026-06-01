# 181 TUI Subscription Dashboard

## Context

The subscription usage TUI already has Lipgloss cards and bars, but it still
reads mostly as stacked text. The user wants the management UI to be more
imaginative with Bubble Tea and Lipgloss, and specifically called out visible
usage bars and account email addresses so pooled subscription accounts are easy
to tell apart.

The management API already exposes safe account display labels, per-window
percentages, reset times, pool aggregate percentages, account counts, and stale
state. This slice should use those fields only. The TUI must not mutate
`config.toml`, storage, provider adapters, or management DTOs.

## Goal

Make the subscription usage display feel like a compact terminal dashboard
instead of a text list, and backfill safe OAuth account labels during token
refresh when upstream returns a fresh ID token.

After this slice:

- the subscription section starts with a visual summary strip,
- account cards have a prominent email or identity row,
- wide terminals lay account and pool cards into two columns,
- each 5h and weekly window has a stronger usage bar with risk labels,
- pooled accounts show total remaining account-points and average-used bars,
- narrow terminals stay bounded and readable.
- OAuth refresh can update empty account display labels from safe ID-token email
  metadata without storing the ID token.

## Scope

1. Update `internal/tui/visual_helpers.go`.
   - Add section banner, two-column card layout, window badge, risk label,
     remaining bar, capacity bar, and highlighted identity helpers.
   - Keep ANSI-aware truncation and width handling.
   - Do not add dependencies or animated Bubble Tea state.
2. Update `internal/tui/observability_subscription.go`.
   - Render a summary strip above account cards.
   - Render account cards through the new card grid helper.
   - Render pool cards through the new card grid helper.
   - Add more visible window badges, total remaining account-point bars, and
     average-used context bars.
3. Update `internal/tui/accounts_oauth.go`.
   - Make safe email/display identity a highlighted row in OAuth and provider
     account cards.
4. Preserve privacy boundaries.
   - Render only already-sanitized display labels, provider IDs, credential
     IDs, plan labels, limit labels, percentages, and reset timestamps.
   - Do not render account IDs, bearer tokens, raw provider payloads, balances,
     credits, prompts, completions, request or response bodies, raw SSE chunks,
     tool data, or provider request IDs.
5. Update the existing OAuth refresh path.
   - Carry `id_token` from refresh responses as transient provider result data.
   - Parse the same safe ChatGPT ID-token metadata used by device login.
   - Update `provider_accounts.display_label` and `plan_label` for the
     credential when non-empty safe values are present.
   - Do not store the ID token or raw claims.
6. Do not change management DTOs, routes, config, key handling, or keepalive
   behavior.
7. Do not add permanent tests.

## Out of Scope

- Adding `bubbles/progress`, `bubbles/table`, or `lipgloss/table`.
- Adding animation, new messages, or a new tab.
- Changing subscription usage fetching, pooling math, or keepalive scheduling.
- Adding a separate email field to storage or DTOs.

## Implementation Steps

1. Add static dashboard primitives to `visual_helpers.go`.
2. Recompose subscription account and pool cards around those primitives.
3. Highlight OAuth/provider account identities with the same helper.
4. Run `gofmt`.
5. Review the code before smoke checks.
6. Run compile, vet, daemon route checks, seeded normal and narrow manage PTY
   smokes, OAuth refresh metadata smoke, and whitespace checks.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
smokedir="$(mktemp -d ./.tmp-oauth-refresh-smoke.XXXXXX)"
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
pid=""
cleanup() {
  rm -rf "$smokedir"
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
cat >"$smokedir/oauth_refresh_metadata_smoke.go" <<'EOF'
package main

import (
  "context"
  "encoding/base64"
  "encoding/json"
  "os"
  "path/filepath"
  "strings"
  "time"

  "ilonasin/internal/config"
  "ilonasin/internal/credentials"
  "ilonasin/internal/provider"
  sqlitestore "ilonasin/internal/storage/sqlite"
)

type fakeRefresher struct {
  idToken string
}

func (f fakeRefresher) RefreshOAuthToken(context.Context, provider.OAuthRefreshRequest) (provider.OAuthRefreshResult, error) {
  expires := time.Now().UTC().Add(time.Hour)
  return provider.OAuthRefreshResult{
    AccessToken: "fresh-access-token",
    IDToken: f.idToken,
    ExpiresAt: &expires,
  }, nil
}

func main() {
  ctx := context.Background()
  dbPath := filepath.Join(os.TempDir(), "ilonasin-oauth-refresh-smoke.sqlite")
  _ = os.Remove(dbPath)
  defer os.Remove(dbPath)
  store, err := sqlitestore.Open(ctx, dbPath)
  if err != nil {
    panic(err)
  }
  defer store.Close()
  cfg := config.Default(".")
  cfg.Providers = map[string]config.ProviderConfig{"codex": {Type: "codex"}}
  registry, err := provider.NewRegistry(cfg)
  if err != nil {
    panic(err)
  }
  svc := credentials.UpstreamService{Registry: registry, Repo: store, Now: time.Now}
  created, err := svc.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
    ProviderInstanceID: "codex",
    Label: "seed",
    AccessToken: "old-access-token",
    RefreshToken: "old-refresh-token",
    AccountID: "account-123",
  })
  if err != nil {
    panic(err)
  }
  rows, err := store.ListProviderAccounts(ctx)
  if err != nil {
    panic(err)
  }
  if len(rows) != 1 || rows[0].DisplayLabel != "" {
    panic("seed display label mismatch")
  }
  svc.OAuthRefresher = fakeRefresher{idToken: jwt(map[string]any{
    "email": "alice@example.com",
    "https://api.openai.com/auth": map[string]any{
      "chatgpt_account_id": "account-123",
      "chatgpt_plan_type": "pro",
    },
  })}
  if err := svc.RefreshOAuthCredential(ctx, created.ID); err != nil {
    panic(err)
  }
  rows, err = store.ListProviderAccounts(ctx)
  if err != nil {
    panic(err)
  }
  if len(rows) != 1 || rows[0].DisplayLabel != "alice@example.com" || rows[0].PlanLabel != "pro" {
    panic("refresh metadata not stored")
  }
  svc.OAuthRefresher = fakeRefresher{idToken: jwt(map[string]any{
    "email": "wrong@example.com",
    "https://api.openai.com/auth": map[string]any{
      "chatgpt_account_id": "other-account",
      "chatgpt_plan_type": "enterprise",
    },
  })}
  if err := svc.RefreshOAuthCredential(ctx, created.ID); err != nil {
    panic(err)
  }
  rows, err = store.ListProviderAccounts(ctx)
  if err != nil {
    panic(err)
  }
  if rows[0].DisplayLabel != "alice@example.com" || rows[0].PlanLabel != "pro" {
    panic("mismatched account metadata overwrote label")
  }
}

func jwt(payload map[string]any) string {
  header := enc(map[string]any{"alg": "none", "typ": "JWT"})
  body := enc(payload)
  return strings.Join([]string{header, body, "sig"}, ".")
}

func enc(value any) string {
  data, err := json.Marshal(value)
  if err != nil {
    panic(err)
  }
  return base64.RawURLEncoding.EncodeToString(data)
}
EOF
(
  cd "$smokedir"
  go run oauth_refresh_metadata_smoke.go
)
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
  "sh -c 'stty cols 112 rows 80; exec env ILONASIN_HOME=\"$tmp/home\" COLUMNS=112 LINES=80 \"$tmpbin/ilonasin\" manage --config \"$cfg\"'" \
  "$tmp/manage-normal.typescript" >/dev/null
normal_status="$?"
printf '\t\tq' | timeout 3s script -q -e -c \
  "sh -c 'stty cols 52 rows 80; exec env ILONASIN_HOME=\"$tmp/home\" COLUMNS=52 LINES=80 \"$tmpbin/ilonasin\" manage --config \"$cfg\"'" \
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
  grep -q "Subscription pools" "$capture"
  grep -q "Codex subscription limits" "$capture"
  grep -q "pool remaining" "$capture"
  grep -q "avg used" "$capture"
  grep -q "█" "$capture"
  if grep -Eiq "bearer|sk-|iln_|access_token|refresh_token|payload|prompt|completion|request_id|acct_" "$capture"; then
    cat "$capture"
    exit 1
  fi
done
perl -MText::Tabs=expand -pe 's/\e\[[0-9;?]*[ -\/]*[@-~]//g; s/[\r\x00-\x08\x0b\x0c\x0e-\x1f\x7f]//g' \
  "$tmp/manage-normal.typescript" |
  awk '/^Script (started|done) / { next } length($0) > 112 { print; bad=1 } END { exit bad }'
perl -MText::Tabs=expand -pe 's/\e\[[0-9;?]*[ -\/]*[@-~]//g; s/[\r\x00-\x08\x0b\x0c\x0e-\x1f\x7f]//g' \
  "$tmp/manage-narrow.typescript" |
  awk '/^Script (started|done) / { next } length($0) > 52 { print; bad=1 } END { exit bad }'
git status --short
git diff --check
```

## Acceptance

- Subscription usage visibly uses dashboard cards, bars, and summary chips.
- Safe email/display labels are prominent in subscription and account cards.
- Missing email labels render as `email not captured`.
- OAuth refresh responses with an ID token update safe account display metadata
  for existing credentials without storing the ID token.
- Wide and narrow PTY captures render account identities and bars.
- Compile, vet, daemon routes, seeded manage PTY smokes, and whitespace checks
  pass.
- Existing unrelated dirty files are not staged or committed.

# 113 Subscription Usage Window Summary

## Context

Plan 104 added Codex subscription usage refresh and the local management route
`GET /_ilonasin/manage/subscription-usage`. The route already exposes per-account
usage rows and pool aggregates, but the JSON shape is still centered on
`primary` and `secondary` fields. The user needs a clearer route response for
"how much is left", reset timing, 5 hour and weekly windows, and pooled account
totals.

Codex source evidence from `/tmp/codex-src-0.135.0/codex-rs` still supports the
Plan 104 boundary:

- `backend-client/src/client.rs` distinguishes ChatGPT `/wham/...` style from
  Codex API `/api/codex/...` style.
- `protocol/src/protocol.rs` models `RateLimitSnapshot` with `primary`,
  `secondary`, `used_percent`, `window_minutes`, and `resets_at`.
- `codex-api/src/rate_limits.rs` parses the same primary and secondary rate
  limit windows from headers and `codex.rate_limits` events.
- The source includes credits metadata, but this project must not store or
  render balances or credits.

Codex exposes percentages and reset times, not exact remaining token or request
counts. Ilonasin must not invent exact remaining counts.

## Goal

Keep the existing subscription usage route compatible while adding a clearer
window-oriented summary for per-account rows and pooled subscription accounts.

After this slice, callers can read:

- per-account 5 hour and weekly usage, remaining percentage, and reset time,
- pooled 5 hour and weekly aggregate state,
- pooled account-percent totals across accounts,
- stale/error counts so callers can decide whether pooled totals are current.

## Scope

1. Extend management DTOs in `internal/management/subscription_usage.go`.
   - Add a reusable `SubscriptionUsageWindow` for account windows.
   - Add a reusable `SubscriptionUsagePoolWindow` for pool windows.
   - Add `windows` to `SubscriptionUsageRow`.
   - Add `windows` to `SubscriptionUsageAggregate`.
   - Preserve every existing JSON field for compatibility.
2. Populate account window summaries.
   - Include `kind` as `primary` or `secondary`.
   - Include `label`, `used_percent`, `remaining_percent`, `window_minutes`,
     and `reset_at`.
   - Keep 300 minute windows labeled `5h` and 10080 minute windows labeled
     `weekly`.
3. Populate pooled window summaries.
   - Include average used percent and minimum remaining percent.
   - Include total used, remaining, and capacity as account-percent points.
   - Include earliest reset.
   - Include account and stale counts.
   - Use the same conservative data already present in pool aggregates.
4. Preserve privacy and boundaries.
   - Do not store or render full account IDs, bearer tokens, provider payloads,
     balances, credits, prompts, completions, request bodies, response bodies,
     raw SSE chunks, tool data, or provider request IDs.
   - Do not add provider calls, migrations, TUI mutations, routing changes,
     keepalive execution, or config mutation.

## Out of Scope

- Exact token/request quota remaining.
- New upstream Codex calls beyond the existing usage refresh.
- Billing, balance, credit, or payment display.
- TUI layout changes.
- Keepalive request execution.
- SQLite schema changes.
- Permanent tests.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
git diff --check
tmpbin=$(mktemp -d)
tmp=$(mktemp -d)
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$tmp/config.toml"
```

For the daemon smoke, use the existing direct serve/manage pattern with a
temporary config, verify the management socket responds to
`/_ilonasin/manage/subscription-usage`, then terminate the daemon and remove
temporary files.

## Acceptance

- `GET /_ilonasin/manage/subscription-usage` keeps the old fields.
- Account rows include `windows`.
- Pool rows include `windows`.
- Pool windows make clear that totals are account-percent points, not exact
  token or request counts.
- Compile, vet, diff whitespace, serve, and manage smokes pass.

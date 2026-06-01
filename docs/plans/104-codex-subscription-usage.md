# 104 Codex Subscription Usage And Keepalive

## Context

The management TUI now has room for account and observability views, but it
still cannot show Codex subscription window usage. The current quota metadata
only records failures seen while routing normal requests. That does not answer
how much of a subscription account's Codex window is used, when the 5 hour and
weekly windows reset, or how pooled accounts look in aggregate.

The user also wants an optional keepalive that sends a minimal request on every
subscription account at 0700, 1200, 1700, and 2200, with 1 max output token, so
rolling 5 hour windows stay active on predictable boundaries.

Codex source evidence in `/tmp/codex-src-0.135.0/codex-rs`:

- `backend-client/src/client.rs` calls `GET {backend_root}/wham/usage` for
  ChatGPT backend style and `GET {backend_root}/api/codex/usage` for Codex API
  style.
- The payload maps into a primary `codex` limit and `additional_rate_limits`.
- `protocol/src/protocol.rs` models `RateLimitSnapshot` with `primary`,
  `secondary`, plan type, and reached type.
- `codex-api/src/rate_limits.rs` also parses `codex.rate_limits` SSE events and
  rate-limit headers into the same shape.
- The usage payload may include credits and balance. Ilonasin must not persist
  or render balances or credits.

## Goal

Expose safe, local-management-only Codex subscription usage for every enabled
Codex OAuth subscription account and the pooled total, then add an opt-in
keepalive scheduler that can run one tiny Codex request per account at fixed
local times.

After this slice, operators can see 5 hour and weekly subscription window
state, reset times, stale/error status, and pool totals without exposing raw
account IDs, bearer tokens, provider payloads, balances, credits, prompts, or
responses.

## Scope

1. Add a Codex subscription usage provider boundary.
   - Create a provider-facing client method for Codex usage snapshots.
   - Derive a Codex usage backend root separately from the model API base.
     The built-in Codex model base is
     `https://chatgpt.com/backend-api/codex`; the usage backend root for that
     instance is `https://chatgpt.com/backend-api`, so the usage URL is
     `https://chatgpt.com/backend-api/wham/usage`.
   - Do not append `/wham/usage` to the existing model base.
   - Use `GET {backend_root}/wham/usage` for ChatGPT backend style Codex
     instances.
   - Preserve room for `GET {base}/api/codex/usage` if a Codex API style base
     is introduced later.
   - Apply the same OAuth bearer and transient account-routing headers used for
     Codex requests.
   - Read only bounded JSON responses. Do not store raw payloads.
   - Parse only safe fields: limit id/name, primary and secondary used percent,
     window minutes, reset timestamp, plan type, reached type, and observed
     time.
   - Ignore credit balance and credit status fields for this slice.
2. Add durable safe subscription usage snapshots.
   - Add a SQLite table such as `subscription_usage_snapshots`.
   - Store provider instance ID, local credential ID, plan label/type, limit id,
     safe limit name, primary and secondary used percent, window minutes,
     reset times, reached type, observed time, stale/error class, and an
     optional source string.
   - Do not store full account IDs, bearer tokens, raw payloads, raw headers,
     balances, credits, provider request IDs, prompts, completions, request
     bodies, response bodies, or raw SSE chunks.
   - Prune snapshots with telemetry pruning or keep a bounded latest-per-window
     history. Prefer latest-per-account rows if it keeps the slice smaller.
3. Expose usage through the daemon management API.
   - Add a local-only route, for example
     `GET /_ilonasin/manage/subscription-usage`.
   - Include per-account rows keyed by provider instance and local credential
     ID, with safe account display label and plan label from existing
     `oauth_tokens`/`provider_accounts` metadata. Do not derive display
     identity from full account IDs.
   - Include aggregate pool rows per provider instance and limit id.
   - Aggregate totals should be conservative: average used percent, minimum
     remaining percent, account count, stale count, and earliest reset.
   - Do not imply exact token counts or exact remaining requests unless the
     upstream provides them safely. Codex source exposes percentages and reset
     times, not absolute remaining request counts.
4. Add refresh behavior.
   - Add a management operation to refresh subscription usage immediately.
   - Refresh all enabled Codex OAuth credentials for Codex provider instances.
   - Reuse the existing OAuth refresh controller before a usage call when an
     access token is stale, then call usage with the resolved bearer only in
     memory.
   - Keep failures per-account. One failing account must not fail the whole
     refresh unless every account fails before producing any safe row.
   - Classify errors safely, such as `unavailable`, `auth_failed`,
     `rate_limited`, `invalid_response`, or `body_too_large`.
   - Do not log raw response bodies or provider payloads.
5. Add opt-in keepalive configuration and scheduler scaffolding.
   - Add static config for this feature. The TUI must not mutate `config.toml`.
   - Default disabled.
   - Configuration should include enabled, local timezone or `local`, schedule
     times defaulting to `07:00`, `12:00`, `17:00`, `22:00`, model, max output
     tokens defaulting to `1`, and a prompt/body that is fixed by code rather
     than user-provided.
   - Do not send any keepalive request until there is hard source evidence for
     a Codex-compatible output cap field, or a dedicated fake/upstream smoke
     proves the accepted wire field against the provider adapter. Current local
     Codex chat validation rejects `max_tokens` and `max_completion_tokens`,
     the local Codex Responses request struct has no output cap field, and
     Codex 0.135.0 `ResponsesApiRequest` has no `max_output_tokens` field.
   - If this slice cannot identify and verify a supported wire-level output cap,
     implement only disabled config, schedule calculation, status/reporting,
     and guards that report `unavailable_output_cap_unverified`; do not perform
     outbound keepalive calls.
   - Once a cap is verified, add explicit Codex request support before enabling
     the keepalive path. Use a provider-internal keepalive request type or an
     adapter method that serializes only the verified Codex field, so scheduler
     code does not construct raw provider JSON.
   - Run only for enabled Codex OAuth subscription accounts. Do not run for API
     keys or non-Codex providers.
   - Send one minimal request per eligible account per scheduled time only
     after the verified cap is available.
   - Use a verified 1-output-token cap; keep input minimal.
   - Never store or render the keepalive prompt or response.
   - Record safe request metadata and subscription usage refresh results only.
   - Add jitter or an in-process idempotency guard so daemon restarts do not
     duplicate the same scheduled tick for one account.
6. Surface in the TUI.
   - Add subscription usage rows to the observability tab or a narrow account
     subsection if simpler.
   - Show provider instance, local credential ID, safe account label, plan,
     5 hour window used percent/reset, weekly window used percent/reset, stale
     or error class, and pool aggregate rows.
   - Do not render balances, credits, full account IDs, bearer tokens, raw
     payloads, prompts, completions, request bodies, response bodies, or raw
     SSE chunks.
7. Keep current routing boundaries.
   - Do not change credential fallback or pooling semantics in this slice.
   - Do not switch accounts because usage says a window is high.
   - Do not implement cross-provider or cross-model routing.
   - Do not estimate provider limits from pricing tables.

## Non-Goals

- No provider billing, balance, credit, or payment display.
- No raw Codex payload persistence.
- No exact remaining token/request count if Codex only provides percentages.
- No decoded full account ID persistence, even if usage calls need the
  transient `ChatGPT-Account-ID` outbound header.
- No keepalive request may run without a verified wire-level output cap. If
  Plan 104 does not establish that cap, the runtime behavior must remain
  status-only even when config says enabled.
- No TUI config editing.
- No permanent test files.
- No push.

## Implementation Notes

Use names that distinguish two concepts:

- `quota_events`: local observations from failed routed requests.
- `subscription_usage`: explicit upstream usage snapshots from Codex usage
  endpoints.

Suggested shape:

- `provider.CodexSubscriptionUsageClient` or a provider method near the Codex
  adapter.
- `provider.CodexKeepaliveClient` or a provider method near the Codex adapter
  for the direct capped keepalive request, so scheduler code does not construct
  raw provider JSON.
- `management.SubscriptionUsageClient` for local daemon routes.
- `metadata.SubscriptionUsageSnapshot` for safe storage records.
- `management.SubscriptionUsageResponse` with per-account and aggregate rows.

Window mapping:

- Treat Codex `primary` as the shorter window. For current product language,
  display a 300 minute primary window as `5h`.
- Treat Codex `secondary` as the longer window. Display 10080 minute windows
  as `weekly`.
- If window minutes differ, show the minute/hour/day value literally instead
  of forcing the label.

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
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Additional direct smokes:

- fake Codex usage endpoint returns primary 300 minute and secondary 10080
  minute windows, management route returns safe per-account rows,
- multiple OAuth accounts aggregate into a pool row,
- one account usage failure produces a safe per-account error while other
  accounts still refresh,
- keepalive disabled by default does nothing,
- keepalive enabled sends exactly one 1-max-output-token request per eligible
  account for one synthetic scheduled tick,
- privacy scan of logs, SQLite metadata, snapshots, and TUI render finds no
  full bearer tokens, full account IDs, raw provider payloads, prompts,
  completions, request bodies, response bodies, balances, credits, or provider
  request IDs.

## Review Questions

1. Should usage refresh be its own management route, part of the snapshot, or
   both?
2. Is latest-per-account storage enough for this slice, or should we keep a
   bounded history for trend display?
3. Should the keepalive scheduler live in `app.Serve` orchestration or in a
   separate daemon service package with explicit dependencies?

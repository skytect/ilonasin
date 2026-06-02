# 287 TUI Wrapped Usage And Log Density

## Goal

Address the next visible TUI density issues from the current screenshots:

- avoid ellipsizing multi-line account/provider rows when the pane can wrap;
- make request logs scan like a compact table;
- separate multi-line subscription account items with a blank line;
- group Codex subscription account rows by model/limit so GPT-5.5 usage and
  GPT-5.4 Spark usage are not interleaved.

## Scope

1. Keep the four top-level tabs and all pane IDs/order unchanged.
2. Keep pane-local scrolling and clipping behavior unchanged.
3. Add wrapping only inside targeted row renderers, before pane clipping:
   - OAuth credential rows;
   - provider account rows;
   - subscription account blocks;
   - request metadata rows.
4. Keep account email-like labels visible when management DTOs safely expose
   them.
5. Keep provider/account/token identifiers sanitized through existing helpers.
6. Render request metadata as table-like rows with stable column ordering for
   status, route, model, token mix/total, cache hit, latency, TTFT, credential,
   and attempt/fallback counters. Use compact continuation lines for fields
   that do not fit.
7. Add a blank line between subscription account items because each account can
   span several lines.
8. Group subscription accounts by a stable provider/limit key:
   - group key includes sanitized `ProviderInstanceID` plus sanitized `LimitID`
     when present;
   - `LimitName` is used for display and only participates in the fallback key
     when `LimitID` is empty;
   - rows with the same provider/limit key stay together;
   - groups render in first-seen order;
   - each group gets a compact safe group header;
   - preserve per-account window bars and summative pool rows.

## Boundaries

- No management API, DTO, storage, schema, provider, server route, Anthropic,
  logging policy, subscription keepalive, config, or action behavior changes.
- No global pane layout, pane ID, navigation, tab, or scroll model changes.
- No new raw prompts, completions, request bodies, response bodies, provider
  payloads, SSE chunks, tool arguments, tool results, raw token values, bearer
  tokens, full account IDs, request IDs, or payload paths rendered.
- No permanent tests.

## Implementation

Touch only:

- `internal/tui/providers_oauth.go`
- `internal/tui/log_requests.go`
- `internal/tui/usage_subscription.go`

Use existing display sanitizers and visual helpers. Existing global helpers may
still truncate unsafe or overlong values. Add small local helpers in the touched
files only if needed for:

- wrapping targeted safe row text at chip boundaries before pane clipping;
- building compact request table rows;
- grouping subscription rows by stable safe provider/limit keys.

Do not modify `internal/tui/panes.go` or shared clipping/layout behavior in
this slice.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused render smoke, then remove it before commit:

- seed OAuth rows and provider accounts with safe email-like labels and long
  provider/plan metadata;
- seed request rows with multiple routes, cache hit rates, token counts,
  latency, TTFT, credentials, and fallback counters;
- seed subscription rows for at least two model/limit labels with multiple
  accounts and 5h/weekly windows;
- render Providers, Usage, and Logs at 80, 120, 160, and 220 columns;
- assert stripped rendered lines fit target width;
- assert account emails remain visible;
- assert unsafe marker strings are redacted;
- assert seeded long safe account/provider/request-model text in the targeted
  row bodies wraps across continuation lines instead of being pane-clipped with
  `...`;
- assert logs show table-like stable columns rather than free-form repeated
  prose;
- assert subscription account blocks have blank-line separation;
- assert subscription group headers use safe `subscriptionLimitLabel`-equivalent
  display and do not render unsafe limit markers;
- assert rows are grouped by stable provider/limit key with GPT-5.5-style rows
  before GPT-5.4 Spark-style rows in the seeded order;
- assert subscription pool rows remain separate from grouped account rows;
- assert pooled subscription rows remain summative only and do not render
  averages.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health and snapshot over the management socket.
4. Run `manage` under short timeouts at narrow and wide terminal sizes.
5. Verify API, providers, usage, and logs chrome renders.
6. Remove all temporary artifacts.

## Acceptance

- Safe account labels are visible without awkward ellipsized rows where the
  pane can wrap.
- Logs scan as compact structured rows.
- Multi-line usage account blocks are visually separated.
- Subscription accounts are grouped by model/limit label instead of
  interleaving models.
- Existing privacy, management, config, navigation, and pane boundaries remain
  intact.

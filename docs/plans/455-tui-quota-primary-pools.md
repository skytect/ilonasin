# 455 TUI Quota Primary Pools

## Context

Recent quota-pane feedback shows three concrete problems:

- subscription pools are below account cards, so pooled remaining capacity is not
  visible first;
- GPT 5.5 and Spark-style limits share one quota pane even though GPT 5.5 is the
  primary view most of the time;
- the account cards make the pane feel impossible to scroll through before the
  pool and keepalive sections are reached.

The current renderer already has safe account labels, limit-priority helpers,
combined used/remaining gauges, and summative pool data. This slice should
reorganize that existing rendering instead of changing quota collection or
management APIs.

## Goal

Make subscription quota usage immediately useful by showing primary pooled GPT
5.5 capacity first, moving secondary Spark-style limits into a separate lower
priority pane, and keeping account cards available below the pool summary.

## Scope

1. Update only TUI usage pane composition, subscription usage rendering, and this
   plan.
2. Split the current quota pane into two usage panes:
   - a primary `quota` pane for priority-0 limits such as GPT 5.5;
   - a secondary `spark quota` pane for priority-1 limits such as GPT 5.4,
     Spark, or BengalFox.
3. Keep unknown or other limits in the primary quota pane after priority-0
   limits, so they are not hidden.
4. Render subscription pools before account cards in each quota pane.
5. Keep pool rows summative-only: total used, total left, capacity, accounts,
   stale count, and reset timing. Do not introduce average-based pool metrics.
6. Improve pool-row wrapping so reset labels do not split into awkward fragments
   such as `earliest` / `re` / `set` across separate lines.
7. Keep account emails or safe identities visible and wrapped in account cards.
8. Preserve keepalive status in the primary quota pane.
9. Do not change management DTOs, storage, provider behavior, refresh logic,
   config, quota math, routing, logging, or management APIs.
10. Do not add permanent tests.

## Verification

Use temporary focused render checks at `70`, `100`, and `140` columns, then
remove them before commit, covering:

- usage panes include separate primary quota and Spark quota panes;
- primary quota output renders pools before account cards;
- Spark-style limits render in the Spark quota pane and not interleaved before
  GPT 5.5 primary output;
- unknown limits remain visible in primary quota output;
- pool rows remain summative-only and do not display average text;
- reset labels render as readable phrases without awkward word fragments;
- account email or safe identity remains visible and wrapped;
- keepalive status remains visible in the primary quota pane and does not appear
  in the Spark quota pane;
- no rendered line overflows the target width.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health and
snapshot over the Unix socket, running bounded `ilonasin manage` at narrow and
wide terminal widths, and cleaning up all temporary files and processes.

## Acceptance

- Pooled quota capacity is visible before account cards.
- GPT 5.5 usage is the primary quota view.
- Spark-style quota details are separated into their own usage pane.
- Pool data remains summative and readable.
- Account identity remains visible.
- No runtime behavior outside TUI rendering changes.
- No permanent tests are added.

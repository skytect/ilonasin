# 446 TUI Usage Overview Strip

## Context

The Usage tab has provider-level token rows, latency rows, stream rows, and
quota panes. The token/performance pane already has bars in individual rows,
but the top summary is still mostly text chips. That makes the user scan several
rows before seeing the overall shape of recent traffic:

- input/output/reasoning/cache token mix;
- cache hit/miss/write rates;
- average latency and time-to-first-token;
- stream and chunk volume.

The architecture requires a polished metadata-only management TUI. This slice
improves visual summarization using existing snapshot data only.

## Goal

Replace the Usage token/performance summary line with a compact overview strip
that uses stacked bars and meter rows to show global token mix, cache/reasoning
rates, latency, and streaming shape.

## Scope

1. Update `internal/tui/usage_metrics.go`.
2. Keep all inputs derived from existing `UsageSummary`, `LatencySummary`, and
   `StreamSummary` rows.
3. Add a small summary struct/helper if needed to avoid duplicating aggregate
   math.
4. Render the top Usage summary as:
   - request/token totals;
   - global token mix using the existing stacked token bar helper;
   - cache and reasoning rates using existing compact meter helpers;
   - weighted average latency and TTFT;
   - stream/chunk totals.
5. Compute global rates from summed token counts, not by averaging provider
   percentages:
   - hit rate = summed cache-hit tokens / summed prompt tokens;
   - miss rate = summed cache-miss tokens / summed prompt tokens;
   - write rate = summed cache-write tokens / summed prompt tokens;
   - reasoning rate = summed reasoning tokens / summed completion tokens;
   - zero denominator returns zero.
6. Keep provider detail rows, latency detail rows, stream detail rows, pane
   layout, management DTOs, storage, config, routing, provider behavior,
   logging, and mutation behavior unchanged.
7. Do not add permanent tests.

## Verification

Use temporary focused render checks, then remove them before commit:

- overview renders at widths 70, 100, and 140 without line overflow;
- zero usage still renders the existing empty state;
- mixed prompt/completion/reasoning/cache values show a non-empty stacked bar;
- hit, miss, write, and reasoning meters are computed from summed token counts;
- zero prompt and completion denominators render zero rates;
- cache miss and write visibility does not collapse into only a generic cache
  value;
- weighted latency and TTFT are computed from request counts;
- stream and chunk totals are preserved.

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

- The Usage token/performance pane starts with a visual overview, not only text
  chips.
- Existing provider, latency, and stream detail rows are unchanged.
- Existing management behavior and metadata-only boundaries are unchanged.
- The TUI still fits narrow and wide terminal smoke runs.
- No permanent tests are added.

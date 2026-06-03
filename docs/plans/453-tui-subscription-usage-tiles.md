# 453 TUI Subscription Usage Tiles

## Context

Recent TUI feedback says the usage view still reads as too much wrapped text,
especially in subscription quota views. The current implementation already has
safe account display labels, grouped GPT 5.5 and GPT 5.4/Spark sections, and
combined used/remaining quota bars. The remaining issue is density and scanning:
account rows are still plain stacked text blocks and wide panes do not use enough
horizontal space.

## Goal

Make subscription usage easier to scan by rendering account quota rows as compact
visual tiles that prioritize account identity and gauges, without changing data
collection, management APIs, quota math, or sanitization.

## Scope

1. Update only TUI subscription rendering and this plan.
2. Render each subscription account as a compact repeated tile with:
   - email or safe identity first;
   - state, credential, observed time, and key metadata as compact chips;
   - one combined used/remaining gauge per quota window;
   - wrapped text, never ellipsized.
3. Use a two-column account tile grid on wider panes and keep one column on
   narrow panes.
4. Keep both the top pool summary and detailed pool rows summative only, using
   existing total used, total left, capacity, account count, stale count, and
   reset timing. Do not remove detailed pool visibility.
5. Preserve current grouping priority, with GPT 5.5 above GPT 5.4/Spark.
6. Preserve email visibility and existing redaction/sanitization helpers.
7. Do not change management DTOs, subscription refresh, storage, config,
   provider behavior, logs, routing, or quota calculations.
8. Do not add permanent tests.

## Verification

Use temporary focused render checks at `70`, `100`, and `140` columns, then
remove them before commit, covering:

- narrow subscription account rendering stays one-column;
- wide subscription account rendering uses a two-column tile grid;
- email or safe account identity is visible, wrapped, and never ellipsized;
- account quota windows render one combined used/remaining gauge with used and
  left values;
- top pool summary and detailed pool rows remain summative-only and do not
  display averages.
- rendered lines do not overflow the target width.

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

- Subscription account rows are visually denser and more scannable.
- Wide panes use horizontal space for account tiles.
- Email or safe account identity remains visible and wrapped.
- Pool data remains summative, not average-based.
- No runtime behavior outside TUI rendering changes.
- No permanent tests are added.

# 438 TUI Performance Meter Rows

## Context

The Usage performance pane already shows latency bars, but throughput is still a
compact text-only row. The user asked for more visual UI and less plain text.
Slice 437 added a shared meter-row helper for this kind of follow-up.

## Goal

Turn Usage throughput metrics into compact visual meter rows without changing
latency math, stream rows, pane layout, or management data contracts.

## Scope

1. Update `internal/tui/usage_metrics.go` and small shared visual helpers only
   if needed.
2. Keep `latencySummaryRow` provider, status, request count, average latency,
   upstream latency, and TTFT values visible.
3. Keep existing latency duration bars unchanged.
4. Replace text-only TPS display in both performance paths:
   - the narrow `latencyShapeLines` TPS row in `internal/tui/usage_metrics.go`;
   - the wide `latencyShapeLine` helper in `internal/tui/metric_visual.go`.
5. Render meter rows for:
   - output TPS;
   - total TPS;
   - post-TTFT TPS.
6. Use a fixed local visual ceiling for TPS meters, only for display scale.
   Do not mutate or reinterpret management data.
7. Preserve stream summary rows, token usage rows, subscription rows, health
   rows, logs, API, providers, key handling, pane layout, and scrolling.
8. Do not add permanent tests.

## Verification

Run:

```sh
gofmt -w internal/tui/usage_metrics.go internal/tui/metric_visual.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary focused render check, then remove it before commit:

- seed a latency row with output, total, and post-TTFT TPS values;
- render the Usage pane at narrow and wide widths, including the wide
  `latencyShapeLine` path;
- assert `output`, `total`, and `post` TPS labels and values remain visible;
- assert the rendered lines fit the target widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO
   capture disabled, keepalive disabled, and configured DeepSeek/Codex provider
   instances.
3. Verify management health and snapshot over the management socket.
4. Run bounded `ilonasin manage` at narrow and wide terminal widths.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- Throughput in Usage is visual, not text-only.
- Existing latency and stream data remains visible.
- No runtime behavior outside TUI rendering changes.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

# 447 TUI Log Overview Strip

## Context

The Logs tab request pane now has a table and per-request detail rows. It still
opens with either an empty metadata ledger or the table header, so the user has
to scan individual rows to understand recent activity shape:

- success, warning, and error distribution;
- route family distribution;
- total token mix and cache usage;
- average latency and TTFT;
- IO capture and metadata-only posture.

The architecture requires metadata-only observability by default and a polished
management TUI. This slice adds a visual overview while preserving the existing
request table and details.

## Goal

Add a compact Logs request overview strip above the request table using only
existing request metadata rows.

## Scope

1. Update `internal/tui/log_requests.go`.
2. Add small helpers to summarize existing `RequestSummary` rows:
   - status counts;
   - endpoint counts;
   - token totals and cache/reasoning mix;
   - average latency and TTFT.
3. Reuse the existing request row status classifier semantics:
   - non-empty error class, HTTP 429, and HTTP 500 or greater count as error;
   - HTTP 400-499 except 429 count as warning;
   - everything else, including status 0, counts as ok/fresh.
4. Render the overview before the table when request rows exist.
5. Keep the existing empty state, request table columns, request detail rows,
   fallback pane, pruning pane, management DTOs, storage, config, provider
   behavior, routing, logging, and mutation behavior unchanged.
6. Do not add permanent tests.

## Verification

Use temporary focused render checks, then remove them before commit:

- overview renders at widths 70, 100, and 140 without line overflow;
- empty request rows keep the existing empty state;
- mixed HTTP status and error-class rows produce ok/warn/error counts matching
  the existing row status classifier, including 429, 500+, and status 0;
- endpoint counts are grouped by the existing short endpoint labels;
- token mix and cache/reasoning values are summed from request rows;
- average latency and TTFT are computed from request rows.

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

- Logs request pane has a visual metadata overview before individual rows.
- Existing request table and per-request details are unchanged.
- Existing metadata-only boundaries are unchanged.
- The TUI still fits narrow and wide terminal smoke runs.
- No permanent tests are added.

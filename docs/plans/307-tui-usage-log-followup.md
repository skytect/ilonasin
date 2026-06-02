# 307 TUI Usage Log Followup

## Context

Recent TUI feedback shows the previous usage/log readability work still leaves
several visible issues:

- safe pane body content can still be ellipsized by the final pane renderer;
- request logs should read more like a compact table, with wrapped details
  under each summary row;
- multi-line subscription account items need blank separation;
- GPT 5.5 subscription usage should group above GPT 5.4 Spark usage;
- pooled quota rows should show summative values with one used/left bar.

This slice is TUI-only. It must not change management DTOs, subscription quota
math, storage, provider behavior, server routes, OAuth refresh, config mutation,
or logging capture policy.

## Plan

1. Add a pane-body wrapping path in `internal/tui/panes.go` so dashboard pane
   body lines wrap before scroll math and visible rendering. Keep intentional
   clipping for pane titles, footer/status chrome, and fixed table cells. Since
   this is shared dashboard infrastructure, verify API, providers, usage, and
   logs panes at narrow and wide widths.
2. Refine `internal/tui/log_requests.go` so each request item has:
   - a fixed-width summary row and header;
   - wrapped continuation rows for model route, token mix, cache rates,
     credential/retry/performance details, and extras;
   - blank separation between request items.
3. Refine `internal/tui/usage_subscription.go` so subscription accounts and
   pools are separated by blank lines, labels use full wrapping display helpers,
   and groups sort by raw limit identity with GPT 5.5 before GPT 5.4 Spark.
   Raw sort keys stay non-rendered implementation details; visible provider,
   limit, and account text must continue through sanitizer helpers.
4. Keep pooled quota rendering explicitly summative: one combined used/left bar
   per pool window and labels `sum used`, `sum left`, and `capacity`.
5. Review the code before checks for ANSI width handling, accidental visible
   `...` in targeted usage/log body fields, privacy redaction, and unintended
   non-TUI changes. Preserve global truncating sanitizers for chrome and other
   non-targeted fields.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary render smoke at narrow and wide widths that seeds:

- long safe subscription emails and request model routes;
- unsafe marker-shaped values that must redact;
- request rows with multi-line details;
- mixed GPT 5.5 and GPT 5.4 Spark subscription limits;
- pooled subscription windows with summative percent-point counts.

It must assert:

- targeted usage/log body content wraps without literal ellipses for seeded
  safe values, while intentional chrome/table-cell truncation remains allowed;
- unsafe seeded values still render as `[redacted]`;
- raw sort keys do not render directly;
- request logs include a table header and separated wrapped detail rows;
- GPT 5.5 subscription groups render before GPT 5.4 Spark;
- pooled rows show one combined bar per seeded window and summative labels only;
- stripped rendered lines fit the target width;
- API and providers panes still render at narrow and wide widths, with stable
  pane scroll maximums and visible scroll markers when content overflows.

Remove temporary smoke files before commit.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temp directory.

## Acceptance

- Safe usage/log pane body content wraps rather than ellipsizing.
- Logs read as compact table rows with wrapped details beneath each item.
- Multi-line subscription items are visually separated.
- GPT 5.5 usage appears above GPT 5.4 Spark usage.
- Pooled quota rows are visually and textually summative only.
- No non-TUI behavior changes are introduced.

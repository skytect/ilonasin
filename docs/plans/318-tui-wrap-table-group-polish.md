# 318 TUI Wrap Table Group Polish

## Context

Recent TUI feedback shows remaining readability issues in the Usage and Logs
sections:

- safe values should wrap instead of visibly ellipsizing;
- log records should scan as compact tables, not loose text blocks;
- multi-line records need blank separation so adjacent items do not blur
  together;
- GPT 5.5 subscription usage should appear above GPT 5.4 Spark usage, grouped
  by limit rather than interwoven by account/provider ordering.

This is a TUI-only rendering slice. It must not change management DTOs, quota
math, storage, provider behavior, server routes, OAuth refresh, config mutation,
Anthropic compatibility, or logging capture policy.

## Plan

1. Audit targeted Usage and Logs render paths for visible `...` output caused by
   display sanitizers or fixed cells. Keep intentional clipping only for pane
   titles and fixed-width table summary cells where the full value is repeated
   in a wrapped continuation field.
2. Prefer full wrapped display helpers for safe body fields in request logs,
   fallback logs, usage metrics, health/quota rows, and subscription usage.
   Unsafe values must still render as `[redacted]`.
3. Keep request and fallback logs table-like:
   - preserve compact aligned summary headers and rows;
   - move long identifiers and details into wrapped continuation fields;
   - add visible blank lines between rendered records.
4. Tighten Usage item spacing:
   - add blank separation between multi-line token usage, performance, health,
     quota, subscription account, and subscription pool records;
   - keep pane-local scrolling and scroll math unchanged.
5. Make subscription grouping explicit:
   - sort account and pool groups by limit priority before provider;
   - render GPT 5.5 groups before GPT 5.4 and Spark groups;
   - keep account email/display labels visible through wrapping.
6. Review the diff for ANSI width behavior, sanitizer use, accidental non-TUI
   edits, quota semantics drift, and unintended table-cell ellipses.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary focused render smoke, then remove it before commit. Seed:

- long safe account emails, provider IDs, model routes, credential labels, and
  log reasons;
- unsafe marker-shaped values in request, fallback, subscription, health, and
  quota fields;
- multiple request/fallback records with wrapped continuation details;
- mixed GPT 5.5, GPT 5.4, and Spark subscription limits;
- pooled subscription windows with summative percent-point counts.

It must assert:

- targeted safe body content wraps without literal `...`;
- unsafe seeded values still render as `[redacted]`;
- logs include compact table headers plus wrapped detail rows;
- multi-line records are separated by blank lines;
- GPT 5.5 subscription groups render before GPT 5.4 and Spark groups;
- pooled subscription rendering remains one combined summative row per pool
  window, with no per-account breakdown inside pooled rows;
- stripped rendered lines fit the target width;
- direct `ilonasin serve` and `ilonasin manage` smokes pass in a temporary home
  and clean up afterward.

## Acceptance

- Safe Usage and Logs body values wrap instead of ellipsizing.
- Logs read as compact table rows with wrapped details beneath each record.
- Multi-line records have visible blank separation.
- GPT 5.5 usage appears above GPT 5.4 Spark usage.
- Pooled quota rows stay summative only, with one combined row per pool window.
- The final diff touches only this plan and TUI render files.

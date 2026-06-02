# 303 TUI Usage Log Readability

## Context

The current `usage` and `logs` panes are structurally correct, but recent visual
feedback shows several remaining readability issues:

- safe long values can still be clipped by fixed pane rendering;
- request logs have table-like rows, but continuation details need clearer
  separation;
- subscription accounts need more obvious spacing when email/account labels wrap;
- GPT 5.5 usage should visually group before GPT 5.4 Spark usage;
- pooled subscription rows should read as summative capacity, not averaged or
  duplicated used/remaining bars.

This slice is render-only TUI work. It must not change management DTOs, quota
math, storage, provider behavior, server routes, OAuth refresh, config mutation,
or logging capture policy.

## Plan

1. Keep fixed chrome clipping in `internal/tui/panes.go` unchanged for pane
   titles, borders, tab/footer chrome, and table summary cells where a fixed
   width table requires compact cells.
2. Pre-wrap usage/log renderer output in:
   - `internal/tui/usage_subscription.go`;
   - `internal/tui/usage_metrics.go`, only if needed for overwide token rows;
   - `internal/tui/log_requests.go`;
   - existing shared visual helpers, only if a usage/log renderer already calls
     them and they currently force visible ellipses.
   This lets safe long emails, provider IDs, model routes, and labels continue
   onto following rows before final pane placement.
3. Keep request logs table-like:
   - retain the aligned compact summary header and row;
   - keep model, token, retry, cache, tier, reasoning, and thinking details on
     wrapped continuation rows;
   - add blank separation between multi-line request items.
4. Tighten subscription quota grouping:
   - render a clear model/limit group header;
   - keep GPT 5.5 groups before GPT 5.4 Spark groups;
   - add blank separation between account items;
   - keep safe email/account display labels visible through wrapping.
5. Make pooled subscription rows explicitly summative:
   - one combined used/left bar per pool window;
   - labels use `sum used`, `sum left`, and `capacity`;
   - no average wording.
6. Review code before checks for ANSI width handling, privacy redaction,
   accidental `...` rendering in content, and unintended non-TUI changes.

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

- long safe account emails and model routes;
- unsafe marker-shaped labels that must redact;
- request logs with wrapped continuation details;
- mixed GPT 5.5 and GPT 5.4 Spark subscription limits;
- pooled subscription windows with summative percent-point counts.

It must assert:

- pane body content wraps without literal ellipses for safe values;
- unsafe values still render as `[redacted]`, including seeded prompt,
  completion, raw payload, SSE chunk, tool argument, request ID, and token-like
  marker strings;
- request logs include a table header and separated multi-line rows;
- GPT 5.5 subscription groups render before GPT 5.4 Spark;
- pooled usage rows show one combined bar per seeded window and summative
  labels only;
- seeded pool percent-point values render from existing aggregate DTO fields,
  with no recomputation as averages;
- stripped rendered lines fit the target width.

Also verify `git diff --name-only` contains only the plan and TUI files.

Remove temporary smoke files before commit.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temp directory.

## Acceptance

- Safe long usage/log values wrap in pane bodies instead of ellipsizing.
- Logs read as a compact table with clear separated detail rows.
- Subscription account rows have blank separation when they span multiple lines.
- GPT 5.5 groups appear above GPT 5.4 Spark groups.
- Pooled subscription rows are visually and textually summative only.
- No non-TUI behavior changes are introduced.

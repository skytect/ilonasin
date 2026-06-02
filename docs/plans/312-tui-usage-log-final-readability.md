# 312 TUI Usage Log Final Readability

## Context

Recent TUI feedback shows a few remaining readability problems in the Usage and
Logs sections:

- request model routes should wrap instead of disappearing behind ellipses;
- fallback logs should scan more like a table;
- multi-line usage, health, quota, and pool items need blank separation;
- GPT 5.5 subscription groups must render above GPT 5.4 Spark groups;
- pooled subscription rows must remain visually and textually summative.

This is a TUI-only recovery slice for the currently uncommitted render changes.
It must not alter management DTOs, quota math, storage, provider behavior,
server routes, OAuth refresh, config mutation, Anthropic compatibility, or
logging capture policy.

## Plan

1. Keep fixed-width summary table cells compact where a table physically
   requires a single row, but render identifying details such as request model
   routes as wrapped continuation fields.
2. Convert fallback metadata rows into a compact table header plus per-event
   rows, with wrapped route, reason, and credential detail lines underneath.
3. Add blank separation between multi-line Usage items in token, performance,
   health, quota, and subscription pool renderers.
4. Preserve subscription group sorting so GPT 5.5 limits render before GPT 5.4
   and Spark limits, including compact spellings such as `gpt5.5`.
5. Review the diff for sanitizer use, ANSI width behavior, accidental non-TUI
   edits, and unintended changes to pooled quota semantics.

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

- long safe request model routes;
- long safe fallback route, reason, and credential labels;
- unsafe marker-shaped values in request, fallback, subscription, health, and
  quota fields;
- mixed GPT 5.5, GPT 5.4, and Spark subscription rows;
- pooled subscription windows with summative percent-point counts.

It must assert:

- targeted safe request and fallback details wrap without literal `...`;
- unsafe seeded values still render as `[redacted]`;
- fallback logs include a table header plus wrapped route, reason, and
  credential continuation details;
- GPT 5.5 subscription groups render before GPT 5.4 and Spark groups;
- pooled rows show one combined bar per seeded window with `sum used`,
  `sum left`, and `capacity`, and no average wording;
- stripped rendered lines fit the target width.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temporary directory.

## Acceptance

- Request model routes and fallback route/reason details wrap as continuation
  lines rather than being hidden in compact cells.
- Logs read as compact table rows with wrapped details beneath each item.
- Multi-line usage and quota items are visually separated.
- GPT 5.5 subscription usage appears above GPT 5.4 Spark usage.
- Pooled quota rows remain summative only.
- The final diff touches only this plan and TUI render files.

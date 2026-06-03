# 431 TUI Pane Scroll Affordances

## Context

The TUI already renders multiple independently scrollable panes, but pane
headers only show the pane title. Overflow is indicated at the bottom with a
numeric marker after the user scrolls or notices it. The user asked for less
whole-view scrolling and clearer use of screen-sized panes.

## Goal

Make scrollable panes more self-explanatory and scan-friendly by adding compact
overflow/focus affordances to pane titles without adding explanatory text blocks.

## Scope

1. Update `internal/tui/panes.go`.
2. Keep the existing pane layout, focus model, mouse scroll behavior, keyboard
   navigation, and per-pane scroll offsets.
3. Add compact title chips or markers that indicate:
   - focused pane;
   - visible line position when a pane has overflow;
   - total content line count or overflow amount in a concise form.
4. Preserve the bottom scroll marker for precise offset feedback.
5. Avoid ellipsizing the meaningful pane title when there is enough width;
   degrade cleanly in narrow panes.
6. Do not change pane content rendering, management DTOs, storage, provider
   behavior, API behavior, config, or subscription usage semantics.
7. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- non-overflow panes render the title without a scroll chip;
- overflow panes render a compact position/total marker in the title;
- focused panes remain visually distinguishable;
- narrow panes do not overflow their pane title width;
- bottom scroll marker behavior remains unchanged.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Users can tell which panes are scrollable from the pane header.
- Existing independent pane scrolling behavior is unchanged.
- No runtime behavior outside TUI rendering changes.

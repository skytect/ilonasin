# 444 TUI Dashboard Chrome Density

## Context

The TUI now has independently scrollable dashboard panes and richer API,
provider, usage, and log content. The surrounding chrome is still sparse:

- the header is a plain sentence;
- the tab bar consumes a full line but carries little state;
- pane titles are visually plain outside focus and scroll markers;
- the footer key map often falls back to key-only chips at narrower widths.

The architecture says `ilonasin manage` should be a polished Bubble
Tea/Lipgloss management TUI, not a debug panel. This slice improves the frame
around existing pane content while keeping all management data, mutations, and
daemon behavior unchanged.

## Goal

Make the dashboard chrome denser and easier to scan without changing any
management API, storage, provider, routing, or TUI action behavior.

## Scope

1. Update `internal/tui/layout.go`, `internal/tui/panes.go`,
   `internal/tui/visual_styles.go`, and small shared visual helpers as needed.
2. Replace the plain header with a compact status strip that uses existing
   sanitized runtime/provider/token/account counts.
3. Make the tab bar look like a section selector with stable active/inactive
   styling and no explanatory text block.
4. Improve pane title chrome so focus and scroll position are clearer while
   preserving independent pane scrolling.
5. Make the footer key map degrade more gracefully at narrow widths.
6. Keep the existing chrome line budget:
   - header remains one line;
   - tab bar remains one line;
   - status remains zero or one line;
   - footer remains one line.
7. Keep pane layout, pane counts, pane content, scroll offsets, click targets,
   keyboard actions, management DTOs, storage, config, provider behavior,
   routing, and logging unchanged.
8. Do not add permanent tests.

## Verification

Use temporary focused render checks, then remove them before commit:

- render the dashboard at widths 70, 100, and 140;
- render full `Model.View()` output at widths 70, 100, and 140;
- active tabs remain visible without line overflow;
- footer key map fits each width;
- pane titles fit their inner widths with focus and scroll markers;
- `tabAtViewPosition` still selects each rendered tab;
- `dashboardTop` and `paneAtViewPosition` still align with the rendered chrome;
- `viewportHeight` remains stable for no-status and status-line states;
- no text content is ellipsized except existing bounded chrome truncation.

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

- Dashboard chrome uses compact visual status and section treatment.
- Existing pane content and management behavior are unchanged.
- Existing viewport reservation and mouse hit target behavior are unchanged.
- The TUI still fits narrow and wide terminal smoke runs.
- No permanent tests are added.

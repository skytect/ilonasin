# 297 TUI Container Wrap Density

## Context

`docs/ilonasin-architecture.md` treats `ilonasin manage` as a first-class
Bubble Tea/Lipgloss UI. The recent TUI wrapping slice addressed request logs and
subscription usage rows, but shared card/banner helpers and some log panes still
truncate rendered content with literal `...`.

This slice is UI-only. It must not change management DTOs, SQLite, provider
behavior, server routes, OAuth refresh, config mutation, logging capture policy,
or subscription keepalive behavior.

## Plan

1. Keep intentional fixed-width chrome clipping for pane borders, scroll
   markers, and terminal viewport placement.
2. Replace visible ellipsis truncation inside TUI content containers with
   ANSI-aware wrapping:
   - section banners;
   - pane subheads;
   - compact cards;
   - metric accent cards.
3. Use targeted full-value safe display helpers before wrapping for changed log
   bodies. Do not feed long safe provider, model, credential, or policy labels
   through capped display helpers before the wrapper can split them.
4. Keep secret fragment abbreviation as-is. Prefix/last4 display is an
   intentional credential-safe abbreviation, not a failed wrap.
5. Route fallback and pruning log panes through wrapped metric lines so long
   safe provider, model, credential, and policy labels wrap instead of pushing
   past the pane or getting clipped later.
6. Keep request log table summary cells compact and non-ellipsis. Details that
   need full values stay on wrapped continuation lines.
7. Do a local code pass before checks for ANSI width handling, card border
   integrity, privacy regressions, and accidental non-TUI changes.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary render smoke at narrow and wide widths that exercises:

- long safe section/chip/card text;
- fallback rows with long provider/model/credential labels;
- pruning rows with long timestamps and counters;
- request log rows with long model routes;
- unsafe marker-shaped values.

It must assert changed content containers and log bodies do not contain literal
`...` except intentional secret fragments. Fixed pane/title/footer chrome may
still clip when it has no room. Unsafe marker-shaped values must still redact,
stripped rendered lines must fit the target width, representative panes from
all four sections must still render, and bordered card/grid output must remain
visually rectangular. Remove temporary files before commit.

Run direct CLI smokes:

```sh
tmp="$(mktemp -d)"
# build ilonasin into the temp dir
# start ilonasin serve with temp ILONASIN_HOME and config
# wait for the management socket
# curl /_ilonasin/manage/health over the socket
# run ilonasin manage under script with narrow and wide terminal sizes
# assert api/providers/usage/logs render
# clean up temp files and daemon
```

## Acceptance

- Shared content containers wrap instead of showing visible ellipsis.
- Fallback and pruning log panes use compact wrapped rows.
- Request log table summaries remain compact while wrapped details carry long
  values.
- No daemon, management DTO, storage, provider, or privacy-policy behavior
  changes are introduced.
- Focused compile, full compile, vet, direct serve/manage smoke, and senior
  implementation reviews pass.

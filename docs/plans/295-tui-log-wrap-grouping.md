# 295 TUI Log Wrap Grouping

## Context

`docs/ilonasin-architecture.md` treats `ilonasin manage` as a first-class
Bubble Tea/Lipgloss management UI, not a debug panel. Current TUI follow-up
feedback asks for:

- visible safe email/display labels without ellipsizing;
- request logs that read more like a compact table;
- blank separation between multi-line subscription account items;
- GPT 5.5 subscription usage grouped above GPT 5.4 Spark rather than
  interleaved;
- no regression to metadata-only privacy boundaries.

This slice is UI-only. It must not change management DTOs, storage, server
routes, provider behavior, OAuth refresh behavior, config mutation, or logging
capture policy.

The worktree also contains an unrelated in-progress privacy slice:

- `docs/plans/294-refresh-failure-description-scrubber.md`;
- `internal/credentials/upstream.go`;
- `internal/management/snapshot_sanitize.go`.

Those files are out of scope for this slice and must not be reverted or folded
into this commit.

## Plan

1. Preserve safe long display strings in the TUI sanitizer so email addresses,
   labels, routes, and model identifiers can wrap instead of being clipped to a
   fixed rune count. Unsafe marker redaction stays unchanged, including token,
   secret, raw payload/body, request ID, SSE chunk, tool argument/result, and
   JWT-shaped markers.
2. Add a small ANSI-aware wrapping helper and a targeted non-truncating display
   sanitizer only for TUI render paths that need to display long safe values.
   Use Charm's ANSI wrapping rather than rune splitting so Lipgloss styles are
   not corrupted.
3. Route targeted request log and subscription account bodies through wrapping
   paths. Fixed chrome such as the footer/key map may still clip intentionally.
   Avoid incompatible double-wrapping layers that damage card borders or styled
   table rows. Keep intentional secret prefix/last4 abbreviation as-is.
4. Move the pane scroll marker to its own line when content overflows so it does
   not truncate the last visible content line. Do not otherwise alter pane/card
   layout behavior.
5. Render recent request logs with a compact table-style leading row:
   status, deliberately compact route label, HTTP status, relative local time,
   stream mode, credential ID, attempt/auth/fallback counts, latency, and total
   tokens. Keep wrapped model, credential label, token, retry, and detail lines
   below the table row so meaningful values are not silently hidden in fixed
   cells.
6. Add blank separation between multi-line subscription account items and
   groups.
7. Sort both subscription account groups and pool rows by raw limit names/IDs,
   with GPT 5.5 before GPT 5.4 Spark, while continuing to sanitize rendered
   labels. Raw sort keys are non-rendered implementation details and must not be
   logged or displayed.
8. Review the code before checks for ANSI width handling, accidental
   truncation, privacy regressions, and unintended non-TUI behavior changes.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a direct serve/manage smoke:

```sh
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
go build -o "$tmp/ilonasin" ./cmd/ilonasin
# start ilonasin serve with ILONASIN_HOME and a temp config
# wait for the management Unix socket
# curl /_ilonasin/manage/health over the socket
# run ilonasin manage under script with a bounded timeout
# assert api/providers/usage/logs render
```

Run a temporary render smoke at narrow and wide widths that seeds long safe
account emails, long model
names, unsafe marker-shaped labels, request rows, and mixed GPT 5.5/GPT 5.4
Spark account/pool usage. It must assert:

- safe long account/model/log bodies do not contain literal `...`;
- unsafe marker-shaped values still render as `[redacted]`;
- request logs include a table header plus an aligned summary row;
- wrapped multi-line subscription account rows have blank separation;
- GPT 5.5 account groups and pool rows render before GPT 5.4 Spark.

Temporary render smoke files must be removed before commit. Confirm cleanup with
`find . -name '*_test.go' -type f -print`.

## Acceptance

- Long safe account/model/log values are rendered through wrapping paths rather
  than literal ellipsis.
- Logs have an aligned table-style summary row and wrapped continuation lines.
- Subscription account rows have blank separation when they span multiple
  lines.
- GPT 5.5 account and pool groups sort above GPT 5.4 Spark.
- TUI privacy redaction still treats unsafe marker-shaped content as redacted.
- No non-TUI behavior changes are introduced.
- Focused compile, full compile, whitespace, serve smoke, manage smoke, and
  senior implementation reviews pass.

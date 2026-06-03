# 430 TUI Local Token Row Density

## Context

The API tab now sits beside other pane-based sections, but local downstream
token rows still use plain metric lines. Long labels can be compacted by
`safeDisplay`, and the row does not use the same wrapped compact rendering as
provider and log rows.

## Goal

Make local API token rows denser and more consistent with the rest of the TUI
while preserving selected-row state, token fragments, lifecycle timestamps, and
label privacy behavior.

## Scope

1. Update `internal/tui/api_local_tokens.go`.
2. Render token row parts with wrapped metric lines so narrow panes wrap
   cleanly.
3. Preserve the selected cursor, enabled/disabled status, token ID, safe label,
   token fragment, created time, and disabled time.
4. Keep the created-token reveal line behavior unchanged.
5. Do not change local token storage, API auth, management DTOs, config, or
   token creation/revocation behavior.
6. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- selected and unselected rows still render the cursor, status, ID, safe label,
  token fragment, created time, and disabled time;
- long safe labels wrap without ellipsis in narrow panes;
- unsafe labels are redacted and do not leak;
- empty labels still leave the row clearly identified by token ID and token
  fragment without malformed blank title spacing;
- wide panes keep lifecycle metadata on the compact row.

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

- Local token rows wrap consistently with other TUI panes.
- Important downstream key identity and lifecycle metadata stays visible.
- No runtime behavior outside TUI rendering changes.

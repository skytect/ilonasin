# 448 TUI Local Token Overview

## Context

The API tab shows local downstream token rows and the API surface inventory. The
local token pane has compact rows, but its top summary is still mostly chips.
For a control-plane view, the user should immediately see the downstream key
inventory shape:

- enabled versus disabled tokens;
- newest token metadata;
- newest disabled-token metadata;
- local API scope and upstream/provider separation.

The architecture requires local API auth to remain separate from upstream
provider credentials and requires the TUI to display metadata only.

## Goal

Add a compact local-token overview above token rows using only existing token
metadata.

## Scope

1. Update `internal/tui/api_local_tokens.go`.
2. Add small helpers to summarize existing `LocalToken` rows:
   - enabled and disabled counts;
   - newest creation timestamp;
   - newest disabled timestamp.
3. Render the overview after the section banner and before token rows when rows
   exist.
4. Keep token row rendering, token creation/disable actions, reveal-once
   behavior, management DTOs, storage, config, provider behavior, routing,
   logging, and mutation behavior unchanged.
5. Do not add permanent tests.

## Verification

Use temporary focused render checks, then remove them before commit:

- overview renders at widths 70, 100, and 140 without line overflow;
- empty token rows keep the existing empty state;
- enabled and disabled counts match token metadata;
- newest created and disabled timestamps are selected correctly;
- token secret fragments are not added to the overview.

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

- Local API token pane has a visual overview before individual token rows.
- Existing token rows and token actions are unchanged.
- No token secret material is newly displayed.
- Existing management behavior and static config boundary are unchanged.
- The TUI still fits narrow and wide terminal smoke runs.
- No permanent tests are added.

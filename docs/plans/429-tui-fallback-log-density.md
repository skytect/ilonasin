# 429 TUI Fallback Log Density

## Context

The logs tab now has denser request metadata rows, but fallback metadata still
uses a table row followed by three expanded detail lines:

- route;
- reason;
- credentials.

This keeps the information visible, but it wastes vertical space and makes the
fallback list feel less table-like than request metadata.

## Goal

Make fallback event rows denser and easier to scan by consolidating detail
metadata under the existing table row.

## Scope

1. Update `internal/tui/log_fallbacks.go`.
2. Keep the existing fallback table header, separator, and first table row.
3. Replace the separate `route`, `reason`, and `credentials` detail lines with
   two compact wrapped rows:
   - route plus reason;
   - source and destination credential identities.
4. Preserve blank-line separation between fallback events.
5. Do not ellipsize provider IDs, model IDs, fallback reasons, or credential
   identities.
6. Do not change management DTOs, storage, request metadata recording, logging
   policy, API behavior, provider behavior, config, or subscription usage
   rendering.
7. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- a fallback row still renders the table row with state, relative time, source
  credential ID, destination credential ID, and route;
- long routes and fallback reasons wrap rather than ellipsize;
- safe credential labels remain visible and unsafe labels fall back to
  credential IDs;
- long safe source and destination credential labels wrap without ellipsis;
- missing source and destination credential labels fall back to credential IDs;
- route/reason and credential details render as compact rows.

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

- Fallback rows are denser and scan like table entries with concise wrapped
  details.
- Important route, reason, and credential metadata remains visible and wrapped.
- No runtime behavior outside TUI rendering changes.

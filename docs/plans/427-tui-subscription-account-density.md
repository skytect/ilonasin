# 427 TUI Subscription Account Density

## Context

The usage quota pane already groups subscription accounts by model family and
renders gauge bars for 5h and weekly windows. It still spends too many lines per
account:

- state and credential live on one line;
- identity/email lives on a separate line;
- provider and plan each get separate field lines;
- source, reached, type, and observed time use another metadata line.

The user asked for more compact TUI presentation, visible email/account
identity, less text-heavy quota display, and better use of horizontal space.
This slice focuses only on subscription account rows in the usage pane.

## Goal

Make each subscription account row denser and more identity-first while keeping
full wrapped email/account visibility and the existing quota gauge behavior.

## Scope

1. Update `internal/tui/usage_subscription.go`.
2. Replace the current multi-line account header with a compact header that
   combines:
   - fresh/stale/error badge;
   - credential ID;
   - provider instance;
   - plan label when present;
   - source/reached/type chips when present;
   - observed time.
3. Keep the account identity/email visibly rendered and wrapped, not ellipsized.
   - Missing email still renders as `email not captured`.
   - Redacted identity still renders as `identity redacted`.
4. Keep the existing usage gauge rows for account windows.
5. Preserve blank-line separation between account blocks for readability.
6. Do not change management DTOs, storage, subscription usage refresh,
   keepalive, pooling, API behavior, logs, config, or provider behavior.
7. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- a row with an email identity renders the full email text;
- a row with a long email wraps rather than ellipsizes;
- a row with no account label renders `email not captured`;
- a row with a redacted account label renders `identity redacted` and does not
  fall back to `email not captured` or expose the unsafe value;
- a row with a plan/source/reached/type renders those values in the compact
  header;
- quota window gauge text still appears for non-error rows;
- error rows still render the error and do not render window gauges.

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

- Subscription account blocks are visibly more compact.
- Email/account identity remains visible and wrapped.
- Quota gauges remain present and unchanged in meaning.
- No runtime behavior outside TUI rendering changes.

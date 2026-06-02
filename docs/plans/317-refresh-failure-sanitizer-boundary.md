# 317 Refresh Failure Sanitizer Boundary

## Context

`docs/ilonasin-architecture.md` allows OAuth refresh failure descriptions from
token endpoint error objects to be stored and rendered for account visibility,
but forbids persisting or rendering full token endpoint responses, bearer
tokens, OAuth tokens, authorization codes, account IDs, request IDs, raw
payloads, prompts, completions, bodies, raw SSE chunks, tool arguments, or tool
results.

That policy is currently duplicated:

- `credentials.safeRefreshFailureDescription` sanitizes descriptions before
  storage;
- `management.safeRefreshFailureDescription` repeats the same normalization and
  unsafe pattern before management DTO exposure;
- `tui.safeRefreshFailureDescriptionDisplay` uses the broader generic TUI
  display pattern and a different truncation limit for the same already
  sanitized field.

Duplicating this privacy policy across credential storage, management snapshots,
and TUI rendering makes the allowed failure-description surface harder to audit.

## Plan

1. Add a small neutral `internal/privacy` package for refresh-failure
   description sanitization. It must import only the standard library and must
   not depend on credentials, management, TUI, storage, provider, server,
   logging, or config packages. The package owns:
   - whitespace/control-character normalization;
   - the unsafe refresh-failure description pattern;
   - the stable max length used for stored and management-visible descriptions.
2. Update credentials storage sanitization and management DTO sanitization to
   call that shared helper, preserving current behavior for safe descriptions,
   unsafe markers, and max length.
3. Update the TUI refresh-failure description display path to call the same
   shared helper for defensive display sanitization, then apply any
   presentation-only truncation. The TUI must not own a divergent unsafe regex
   for refresh-failure descriptions.
4. Keep refresh failure classes, OAuth refresh behavior, storage schema,
   management API JSON shape, provider adapters, logging, config, and TUI layout
   unchanged.
5. Review the diff for privacy regressions, boundary direction, accidental
   broadened rendering of raw error payloads, and new package placement.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/credentials
go test ./internal/management
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary focused smoke, then remove it before commit. It must prove:

- credentials and management sanitizers produce identical output for safe
  descriptions;
- credentials and management sanitizers redact bearer/API/local/OAuth token
  markers, authorization code markers, account/request ID markers, raw payload
  markers, prompt/completion/body markers, SSE chunk markers, tool
  argument/result markers, and JWT-shaped strings;
- safe long descriptions are truncated consistently at the shared max length;
- TUI refresh-failure display preserves safe sanitized text and does not expose
  unsafe seeded management values because it reuses the shared helper.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME` and isolated config, checking management health
over the Unix socket, running `ilonasin manage` under bounded narrow and wide
terminals, and cleaning up the daemon and temporary directory.

## Acceptance

- Refresh-failure description privacy policy has one shared implementation for
  credential storage and management DTOs.
- TUI display reuses the shared unsafe policy for refresh-failure descriptions
  instead of owning a divergent regex.
- Existing safe descriptions remain visible, unsafe descriptions remain
  redacted, and max-length behavior remains stable.
- No storage schema, provider behavior, management API shape, logging policy,
  config behavior, or TUI layout changes are introduced.

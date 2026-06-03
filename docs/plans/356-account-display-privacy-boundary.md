# 356 Account Display Privacy Boundary

## Context

`docs/ilonasin-architecture.md` allows safe OAuth account display metadata, and
`docs/codex-auth.md` requires management snapshots, CLI/TUI output, request
metadata, and fallback metadata to avoid full account IDs. Safe email-like
labels should remain visible so users can distinguish subscription accounts,
but token-shaped, account-ID-shaped, request-ID-shaped, raw payload, prompt,
completion, stream chunk, and tool payload markers must not render.

The account-display unsafe marker policy is currently duplicated:

- `internal/management/snapshot_sanitize.go` owns
  `unsafeAccountDisplayPattern` for management DTO exposure;
- `internal/tui/display_sanitize.go` owns a near-copy for TUI rendering.

Recent slices centralized refresh-failure description and class policy in
`internal/privacy`. Account-display safety has the same shape: it is a
cross-boundary privacy rule that should be owned by a neutral package, while
management and TUI keep only presentation-specific truncation or wrapping.

## Scope

1. Add a shared `privacy.AccountDisplay` helper.
   - It trims leading/trailing whitespace.
   - It removes control characters.
   - It returns empty for empty input.
   - It returns `[redacted]` for unsafe account-display markers.
   - It preserves safe email-like labels and safe non-email labels such as
     `Codex`.
   - It imports only the standard library.
2. Move the duplicated account-display unsafe regex into `internal/privacy`.
3. Update management account-display sanitization to call the shared helper,
   then keep its existing 128-rune DTO truncation.
4. Update TUI account-display helpers to call the shared helper, then keep their
   existing presentation behavior:
   - `safeAccountDisplay` remains capped for compact fields;
   - `safeWrappedAccountDisplay` keeps its existing compact 64-rune cap;
   - `safeFullWrappedAccountDisplay` continues preserving long safe labels.
5. Preserve the existing generic display sanitizers and generic unsafe display
   regexes. This slice only moves account-display policy.
6. Do not change storage schema, DTO shapes, OAuth metadata extraction,
   subscription usage aggregation, TUI layout, logging, routing, provider
   behavior, config, or request metadata.

## Out Of Scope

- Adding a separate email field.
- Changing how account labels are extracted from OAuth claims.
- Changing account hashes or full account ID handling.
- Changing generic display sanitization.
- Changing any TUI layout or wrapping behavior.

## Implementation Steps

1. Add `privacy.AccountDisplay(value string) string`.
2. Replace management's duplicated account-display sanitizer internals with the
   shared helper plus existing 128-rune truncation.
3. Replace TUI account-display helpers with the shared helper plus existing
   compact or wrapped presentation limits.
4. Remove duplicated account-display regexes from management and TUI if they no
   longer have callers.
5. Review the diff for safe email preservation, unsafe marker redaction, and no
   dependency cycles.

## Verification

Use temporary focused checks, then remove them before commit:

- safe email-like labels remain visible through `privacy`, management, and TUI;
- safe non-email labels remain visible;
- token, account ID, request ID, raw payload/body, prompt/completion, stream
  chunk, tool argument/result, and JWT-shaped strings become `[redacted]`;
- management keeps its 128-rune cap after shared sanitization;
- compact TUI account display keeps its 64-rune cap after shared sanitization;
- wrapped TUI account display keeps its existing 64-rune cap after shared
  sanitization;
- full wrapped TUI account display preserves long safe labels after shared
  sanitization.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/privacy
go test ./internal/management
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage`, and cleaning up all temporary
files and processes.

## Acceptance

- Account-display privacy policy has one shared implementation in
  `internal/privacy`.
- Management and TUI account-display rendering use the same unsafe marker
  policy while preserving their existing presentation limits.
- Safe email/display labels remain visible.
- Unsafe account, token, raw payload, request ID, and JWT-like values are not
  exposed.
- No storage, API shape, OAuth extraction, subscription, TUI layout, logging,
  routing, provider, or config behavior changes are introduced.

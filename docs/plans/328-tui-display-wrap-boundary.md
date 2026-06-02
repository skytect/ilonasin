1. Keep this slice TUI-only.
   - Touch only `internal/tui` display/wrapping helpers and this plan.
   - Do not change daemon, provider, management DTO, SQLite, config, routing,
     request logging, or any currently dirty server/provider files.
   - Check `git diff --name-only` before and after implementation and ensure
     staged files for this slice remain limited to this plan plus `internal/tui`.

2. Tighten the display wrapping boundary.
   - Make label/value display wrapping use ANSI display width consistently.
   - Prefer word-boundary wrapping for human text where possible.
   - Preserve hard wrapping for long machine identifiers, emails, hashes, model
     names, and token-like fragments so panes never overflow horizontally.
   - Keep targeted wrapped display values wrapped, not ellipsized. Do not
     change intentional chrome clipping, compact unsafe-adjacent display, or
     sanitizer behavior that currently uses ellipses.

3. Preserve existing redaction and styling.
   - Keep all values passing through the existing safe display helpers.
   - Do not expose secrets, account IDs, request bodies, response bodies,
     prompts, completions, tool arguments, or raw provider payloads.
   - Do not change chip colors, pane styles, card styles, or section ordering.

4. Keep pane behavior unchanged.
   - Do not change pane IDs, focus, scroll offsets, tab order, action routing,
     keybindings, or layout column rules.
   - Keep pane-local scrolling and clipping behavior.

5. Verify directly.
   - Use a temporary focused TUI check if useful, then remove it before commit,
     covering long prose word wrapping, long machine IDs/emails/model names,
     ANSI-styled chips, wide Unicode width, no literal `...` in targeted wrapped
     values, and unsafe marker redaction for token/raw/prompt/body/account-like
     strings.
   - Run `go test ./internal/tui`, `go test ./...`, `go vet ./...`, and
     `git diff --check`.
   - Build `cmd/ilonasin`.
   - Smoke `ilonasin serve` with a temporary home and `[server] bind =
     "127.0.0.1:0"`, check `/_ilonasin/manage/health` over the management
     socket, and run a short `ilonasin manage` TUI smoke.
   - Keep temporary checks and files out of the final commit.

# 459 Local Token Usage Summary

## Goal

Expose downstream local API-key usage in the management snapshot and API TUI pane.

## Scope

- Add a metadata-level local-token usage summary grouped by `request_metadata.client_token_id`.
- Include all retained request metadata until pruning, not only recent requests.
- Include request count, status mix, token totals, cache and reasoning token counts, average latency, and latest request time.
- Add a snapshot-only management API field for the summary.
- Render the usage summary under the API `downstream keys` pane, aligned with existing token rows.
- Render unattributed or deleted-token usage as a compact separate row rather than dropping it.
- Do not store full local API tokens, token hashes, prompts, completions, request bodies, response bodies, tool arguments, or raw stream chunks.
- Do not change local API auth, request routing, provider behavior, SQLite schema, config, or TUI keybindings.

## Verification

- `gofmt` on touched Go files.
- `git diff --check`.
- `find . -name '*_test.go' -type f -print`.
- `go test ./internal/management`.
- `go test ./internal/storage/sqlite`.
- `go test ./internal/tui`.
- `go test ./...`.
- `go vet ./...`.
- Build a temp `ilonasin` binary, run isolated `serve`, smoke `/health` and `/snapshot`, and open `manage` at 70, 100, and 140 columns with cleanup afterward.

## Risks

- Joining token metadata must not select or expose token hashes or full token values.
- Disabled tokens may still have historical usage and should remain visible.
- Requests with missing or deleted token IDs should not crash snapshot rendering.
- The TUI should remain compact at narrow widths and should not duplicate the recent request log table.

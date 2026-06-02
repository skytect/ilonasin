# Plan 324: TUI Wrap Table Group Polish

## Goal

Tighten the TUI usage and logs rendering so long values wrap instead of being
dropped, logs read as tables, subscription entries have clearer separation, and
Codex subscription limits are grouped with GPT 5.5 usage before GPT 5.4/Spark.

## Scope

- Add a reusable wrapped table row helper for compact metadata tables.
- Keep table wrapping ANSI-width aware, with no ellipsis fallback in the target
  request or fallback table rows.
- Use wrapped table rows for request metadata and fallback metadata.
- Preserve table headers, status, route, token, retry, latency, and model data.
- Keep expanded request and fallback detail lines below each table row.
- Make subscription group headers model-limit first, with provider metadata below.
- Use existing safe display helpers so longer wrapped values do not expose unsafe
  account IDs, tokens, request IDs, raw IO, prompts, completions, tool values, or
  SSE chunks.
- Sort subscription usage from existing limit name and ID fields only: GPT 5.5
  first, GPT 5.4/Spark second, unknown limits after that.
- Add clearer blank-line separation inside multi-line subscription account and
  pool entries.
- Keep all TUI data read-only through the existing management snapshots.

## Out Of Scope

- No management DTO changes.
- No daemon, provider, storage, or config changes.
- No permanent tests.

## Verification

- `gofmt` on touched Go files.
- `git diff --check`.
- `find . -name '*_test.go' -type f -print`.
- Temporary focused TUI render smoke, removed before commit:
  - long safe request and fallback values wrap with no literal `...`;
  - unsafe marker values still redact;
  - request and fallback table headers remain visible;
  - subscription account entries include blank separation between multi-line
    items;
  - GPT 5.5 groups render before GPT 5.4/Spark groups;
  - pooled quota rows remain summative;
  - stripped rendered lines fit the requested width.
- `go test ./internal/tui`.
- `go test ./...`.
- `go vet ./...`.
- Build a temporary `ilonasin` binary.
- Start `ilonasin serve` with a temporary `ILONASIN_HOME`.
- Hit the management health endpoint.
- Run `ilonasin manage` under short PTY timeouts at narrow and wide widths.
- Clean up all temporary files and daemon processes.

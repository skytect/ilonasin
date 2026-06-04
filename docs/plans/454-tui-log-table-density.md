# 454 TUI Log Table Density

## Context

The logs tab already has request and fallback table headers, but each item still
expands into stacked detail prose with large gaps. Earlier TUI feedback asked
for logs to read more like tables and for text-heavy views to become more
visual and compact without losing wrapped detail.

## Goal

Make request and fallback log panes more table-first and denser, while keeping
metadata-only privacy boundaries and preserving wrapped detail for long routes,
credentials, errors, and fallback reasons.

## Scope

1. Update only TUI log rendering and this plan.
2. Render request rows as a compact table row plus one dense visual metrics row.
3. Keep detailed route, credential, error, fallback, tier, and reasoning fields
   available as wrapped detail chips, but reduce unnecessary vertical spacing.
   Preserve the existing metadata-only request detail chips for endpoint,
   fallback reason, error class, message/tool/image counts, upstream latency,
   requested and effective service tier, reasoning effort, and thinking type.
4. Render fallback rows as a compact table row plus one dense detail row rather
   than several stacked prose lines.
5. Preserve table wrapping, never ellipsize values.
6. Preserve metadata-only display. Do not add prompts, completions, request
   bodies, response bodies, raw provider payloads, stream chunks, tool
   arguments, or tool results.
7. Do not change management DTOs, storage, config, provider behavior, routing,
   logging collection, pruning, or management APIs.
8. Do not add permanent tests.

## Verification

Use temporary focused render checks at `70`, `100`, and `140` columns, then
remove them before commit, covering:

- request log output keeps a table header and table rows;
- fallback log output keeps a table header and table rows;
- long route/model, credential labels, error classes, and fallback reasons wrap
  without ellipsis;
- endpoint, fallback reason, error class, message/tool/image counts, upstream
  latency, requested and effective service tier, reasoning effort, and thinking
  type remain visible when present;
- request rows include compact visual token/cache/latency metadata;
- no rendered line overflows the target width;
- no raw IO content fields appear.

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

- Logs read primarily as compact tables.
- Long metadata wraps instead of being ellipsized.
- Request rows retain useful token/cache/latency visuals.
- Fallback rows are denser while preserving route, reason, and credential
  context.
- No runtime behavior outside TUI rendering changes.
- No permanent tests are added.

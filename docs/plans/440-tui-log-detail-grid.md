# 440 TUI Log Detail Grid

## Context

Request and fallback logs already have table headers, but expanded detail rows
still read like wrapped prose. The user asked for logs to be more table-like and
for multi-line items to stay readable without losing wrapped values.

## Goal

Make request and fallback detail rows denser and more table-like while
preserving all existing metadata and wrapping behavior.

## Scope

1. Update `internal/tui/log_requests.go` and `internal/tui/log_fallbacks.go`
   only if needed.
2. Replace `requestDetailLine` output with a compact detail-grid style that:
   - keeps the detail label visible;
   - wraps values instead of ellipsizing them;
   - keeps wrapped continuation lines aligned;
   - preserves existing route, credential, usage, fallback reason, endpoint,
     error, message/tool/image, latency, tier, reasoning, and thinking fields.
3. Keep request and fallback table headers, separators, row columns, row order,
   blank line separation between entries, and per-pane scrolling unchanged.
4. Use existing sanitizers for request/model/provider/credential/error display.
5. Do not change management DTOs, storage, logging policy, IO capture policy,
   provider behavior, routes, config, pane layout, or key handling.
6. Do not add permanent tests.

## Verification

Run:

```sh
gofmt -w internal/tui/log_requests.go internal/tui/log_fallbacks.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary focused render check, then remove it before commit:

- seed request and fallback rows with long safe provider/model/credential values;
- render request and fallback summaries at narrow and wide widths;
- assert table headers remain visible;
- assert route, usage, credential, fallback reason, endpoint, and error metadata
  remain visible;
- assert message/tool/image counts, upstream latency, requested and effective
  service tiers, reasoning effort, and thinking type remain visible;
- assert no literal ellipsis is introduced by the changed detail rows;
- assert rendered lines fit the target widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO
   capture disabled, keepalive disabled, and configured DeepSeek/Codex provider
   instances.
3. Verify management health and snapshot over the management socket.
4. Run bounded `ilonasin manage` at narrow and wide terminal widths.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- Log detail rows look more like structured metadata than prose.
- Existing metadata remains visible and wrapped.
- No runtime behavior outside TUI rendering changes.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

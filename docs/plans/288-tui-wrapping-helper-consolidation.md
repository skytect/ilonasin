# 288 TUI Wrapping Helper Consolidation

## Goal

Remove duplicate TUI wrapping and non-truncating safe-display helper logic left
by the recent provider/log/usage renderer work, without changing the rendered
control-plane behavior.

The architecture requires `ilonasin manage` to be a first-class TUI and the
thread goal requires no residual duplicate code. The current implementation has
near-identical local helpers in `providers_oauth.go`, `log_requests.go`, and
`usage_subscription.go` for:

- wrapping metric rows before pane clipping;
- chunking oversized safe values before pane clipping;
- sanitizing safe display values without pre-wrap truncation.

## Scope

1. Move duplicate wrap-at-boundary logic into shared TUI helper functions.
2. Move duplicate oversized safe-value chunking into one helper.
3. Move duplicate non-truncating safe-display sanitization into one helper that
   still redacts unsafe marker strings.
4. Keep account-specific sanitization separate from generic display
   sanitization through explicit helper names.
5. Update the three current renderer call sites to use the shared helpers.
6. Preserve the behavior introduced in plan 287:
   - targeted safe account labels can wrap without `...`;
   - targeted safe request model labels can wrap without `...`;
   - unsafe markers still render as `[redacted]`;
   - subscription grouping keys do not use display-truncated strings;
   - subscription account group headers remain visually distinct from pooled
     summative rows.

## Boundaries

- No management API, DTO, storage, schema, provider, server route, Anthropic,
  logging policy, subscription keepalive, config, or action behavior changes.
- No pane layout, pane clipping, navigation, tab, scroll, or pane ID changes.
- No new UI feature or visual redesign in this slice.
- No raw prompts, completions, request bodies, response bodies, provider
  payloads, SSE chunks, tool arguments, tool results, raw token values, bearer
  tokens, full account IDs, request IDs, or payload paths rendered.
- No permanent tests.

## Implementation

Touch only:

- `internal/tui/display_sanitize.go`
- `internal/tui/visual_text.go`
- `internal/tui/providers_oauth.go`
- `internal/tui/log_requests.go`
- `internal/tui/usage_subscription.go`

Expected helper shape:

- a non-truncating generic safe display helper for wrap-targeted text;
- a non-truncating account safe display helper for email/account labels;
- a metric-row wrapping helper that wraps between supplied parts;
- a display-value chunking helper that splits oversized single values by
  terminal cell width.

Keep the existing truncating `safeDisplay` and `safeAccountDisplay` behavior for
all existing call sites that are not explicitly wrap-targeted.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused render smoke, then remove it before commit:

- seed OAuth/provider account rows with long safe email-like labels and unsafe
  marker labels;
- seed request rows with long safe model labels and unsafe marker model labels;
- seed request extras with fallback reason, requested/effective service tiers,
  reasoning, thinking, messages, tools, and images so extras still wrap through
  the shared helper;
- seed subscription rows with long safe account labels and long safe limit IDs
  that share a prefix but differ at the end;
- render Providers, Usage, and Logs at 80, 120, 160, and 220 columns;
- assert targeted long safe values wrap without display-truncation ellipses;
- assert unsafe markers redact;
- assert long request extras wrap without pane-clipping ellipses;
- assert long safe subscription limit IDs do not collapse into one group;
- assert actual subscription pool rows still render separately and summative.

Run a cleanup guard after implementation:

```sh
rg -n "func (wrapMetricLine|wrapRequestMetricLine|wrapSubscriptionMetricLine|wrapPlainDisplayChunks|wrapRequestDisplayChunks|wrapSubscriptionDisplayChunks|safeWrappedDisplayWithPattern|safeWrappedRequestDisplayWithPattern|safeSubscriptionGroupKeyWithPattern)" internal/tui/providers_oauth.go internal/tui/log_requests.go internal/tui/usage_subscription.go
```

This command should print no renderer-local duplicate helper definitions.
Small domain wrappers may remain only when they add renderer-specific meaning
without duplicating shared wrapping or sanitization logic.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health and snapshot over the management socket.
4. Run `manage` under short timeouts at narrow and wide terminal sizes.
5. Verify API, providers, usage, and logs chrome renders.
6. Remove all temporary artifacts.

## Acceptance

- Duplicate wrapping/safe-display helper logic from the three renderers is
  consolidated.
- Existing plan 287 behavior is preserved.
- Privacy boundaries remain unchanged.
- Compile, vet, focused render smoke, daemon/manage smoke, senior plan review,
  and senior implementation review pass.

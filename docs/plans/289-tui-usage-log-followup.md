# 289 TUI Usage And Log Follow-Up

## Goal

Tighten the recent TUI usage and log density work against the latest screenshot
feedback:

- safe email/model/provider labels should wrap where possible instead of being
  ellipsized;
- request logs should scan as compact table rows;
- multi-line subscription account items should have blank-line separation;
- subscription account groups should keep all GPT-5.5-style usage above
  GPT-5.4 Spark-style usage instead of interweaving rows.

## Scope

1. Keep the four top-level TUI sections and pane-local scrolling unchanged.
2. Keep all changes inside TUI rendering code.
3. Preserve existing management DTOs, storage, server routes, provider adapters,
   subscription keepalive behavior, and config behavior.
4. Sort subscription account groups with a deterministic display priority:
   - groups remain keyed by safe provider plus safe limit identity;
   - GPT-5.5-style limits render before GPT-5.4 Spark-style limits when both
     are present;
   - unknown limits keep stable lexical order after known Codex limits;
   - rows inside a group keep daemon order.
5. Keep pooled subscription rows separate and summative only.
6. For targeted wrapped TUI rows, avoid `safeDisplay` truncation when the field
   can be safely wrapped before pane clipping.
7. Make request metadata rows more table-like by using stable compact columns
   and continuation lines for model, token, cache, route, latency, and fallback
   metadata.

## Boundaries

- No changes to public management API JSON shape.
- No changes to subscription usage calculations.
- No new account IDs, tokens, request IDs, prompts, completions, request bodies,
  response bodies, raw provider payloads, raw SSE chunks, tool arguments, or
  tool results in TUI output.
- No global dashboard layout or clipping changes.
- No permanent tests.

## Implementation

Touch only:

- `internal/tui/log_requests.go`
- `internal/tui/usage_subscription.go`
- shared TUI visual helpers only if needed for non-truncating safe chips.

Planned changes:

1. Add a small non-truncating wrapped chip helper or local equivalent for the
   targeted rows.
2. Update subscription group ordering after grouping, without changing grouping
   keys or row membership.
3. Keep blank-line account separation in `writeSubscriptionUsage`.
4. Update request rows to render as fixed-order, table-like lines:
   - status, route, model, HTTP status, time, stream;
   - token mix, total, cache visual;
   - credential, tries, auth retries, fallback count, latency, TTFT, TPS;
   - optional extras on continuation lines.
5. Add temporary render smoke coverage, then remove it before commit.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused TUI render smoke, then remove it before commit:

- seed long safe account emails and long safe model/provider labels;
- seed interleaved GPT-5.4 Spark and GPT-5.5 subscription rows;
- seed request log rows with token, cache, latency, and fallback metadata;
- render Usage and Logs at 80, 120, 160, and 220 columns;
- assert stripped rendered lines fit target widths;
- assert safe emails are visible;
- assert safe long labels wrap before pane clipping in targeted rows;
- assert GPT-5.5 groups render before GPT-5.4 Spark groups;
- assert subscription account items have blank-line separation;
- assert pooled rows stay summative and do not render averages.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health and snapshot over the management socket.
4. Run `manage` under short timeouts at narrow and wide terminal sizes.
5. Verify API, providers, usage, and logs chrome renders.
6. Remove all temporary artifacts.

## Acceptance

- Safe account labels wrap visibly in targeted rows.
- Logs read like compact structured rows.
- Subscription account cards have visual separation when they span lines.
- GPT-5.5-style usage groups render above GPT-5.4 Spark-style groups.
- Pooled subscription usage remains summative only.

# 456 TUI Routing Logging Backlog

## Context

Recent operator feedback shows the current product is functionally broad but the
management experience still has too much duplicated visual language, too many
repeated labels, rigid pane layouts, and uneven density. The next work needs to
stay organized across many small slices instead of turning into one large TUI,
routing, and logging rewrite.

This plan records the follow-up backlog that should drive the next UI-heavy
slices, then the Codex account-routing work, then the logging architecture work.

## Goal

Create a reviewed, committed backlog plan for the next major sequence of slices:
polish the TUI into a concise operational dashboard, add better downstream-key
usage visibility, design prefix-cache-aware Codex account routing, and simplify
logging around the binary IO-logging boundary.

## Scope

1. Update only this plan.
2. Do not implement runtime behavior in this slice.
3. Treat the following TUI items as the next implementation priority:
   - deduplicate repeated labels, chips, and status text across all panes;
   - clean up usage and performance views so detailed token/cache/latency data
     remains visible but is grouped into digestible summaries;
   - aggregate health/quota by endpoint/model/credential where useful, and keep
     per-event detail in logs rather than health panes;
   - make logs more digestible, likely full-width or vertical table-style when
     that better fits content than rigid columns;
   - shrink IO policy/pruning display so it does not consume large empty panes;
   - replace rigid equal-column layouts with dynamic pane widths or irregular
     tiles where content density benefits;
   - audit every keybinding and keep a parsimonious, intuitive, fully featured
     set for each screen;
   - make live daemon/SQLite state feel live in the TUI, avoiding manual refresh
     as the normal path;
   - expose downstream local API-key usage and activity, not only upstream
     credential/provider usage;
   - show routing strategy, cache status, and pooling state in the TUI once the
     backend surfaces exist.
4. Treat the following Codex account-routing work as a later sequence after the
   near-term TUI sweep:
   - optimize round-robin/load distribution across Codex OAuth accounts;
   - account for cache locality only through architecture-approved routing
     signals such as safe client-sent `prompt_cache_key`, selected safe
     metadata, selected safe headers, verified local token identity, resolved
     provider/model route, pressure, and cursor state;
   - do not synthesize affinity from prompts, messages, input content, request
     bodies, completions, tool arguments, tool results, or raw IO logs;
   - treat any prompt/body-derived cache-affinity idea as requiring a separate
     architecture change and provider-policy review before implementation;
   - keep any new cache-affinity behavior auditable and disableable in
     `config.toml`;
   - make the TUI show cache-affinity/routing strategy status when implemented;
   - keep provider-policy and quota-evasion risks explicit and auditable.
5. Treat the following logging work as a later architecture sequence:
   - simplify redaction around a binary metadata-only versus IO-logging boundary;
   - when IO logging is disabled, do not store raw prompts, completions, request
     bodies, response bodies, raw provider payloads, SSE chunks, tool arguments,
     or tool results;
   - when IO logging is enabled, keep raw IO in the IO-logging path and make the
     policy explicit rather than scattering defensive redaction through normal
     metadata paths;
   - reassess whether file-based logging remains the right default;
   - if file logging remains, investigate more queryable or robust structures;
   - split oversized logging code into maintainable boundaries.
6. Keep all future slices aligned with `docs/ilonasin-architecture.md`, with
   daemon-owned SQLite reads/writes, TUI mutations through management APIs, and
   no TUI mutation of `config.toml`.
7. Do not add permanent tests in this planning slice.

## Suggested Slice Order

1. TUI full-sweep inventory: identify repeated UI patterns and target removals.
2. TUI pane layout policy: dynamic/irregular pane sizing where content needs it.
3. TUI usage summary redesign: clean token/cache/latency visuals.
4. TUI health aggregation: endpoint/model/credential summaries, events moved to
   logs.
5. TUI logs layout redesign: table-first/full-width where appropriate.
6. TUI IO policy/pruning compaction.
7. TUI downstream API-key usage visibility.
8. TUI keybinding audit and command-bar cleanup.
9. TUI live update strategy through management snapshots/events.
10. TUI routing/cache-status surface, after backend data exists.
11. Codex cache-affinity architecture for approved client-sent signals.
12. Codex cache-affinity implementation.
13. Logging boundary architecture review.
14. Logging storage/queryability design.
15. Logging implementation refactors.

This order is intentionally a backlog, not a promise that each item is one
slice. Each future slice still needs its own numbered plan, three plan reviews,
implementation, checks, direct `serve`/`manage` smoke where relevant, three
implementation reviews, and a commit.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health and
snapshot over the Unix socket, running bounded `ilonasin manage` at narrow and
wide terminal widths, and cleaning up all temporary files and processes.

## Acceptance

- The plan captures the current TUI feedback without changing runtime behavior.
- The plan names downstream local API-key usage, live TUI state, keybindings,
  cache-affinity routing, and logging architecture follow-ups explicitly.
- Future work can use this document as a backlog while still creating separate
  reviewed numbered plans for each slice.
- No unrelated worktree changes are modified or staged.
- No permanent tests are added by this slice.

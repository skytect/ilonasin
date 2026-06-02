# 254 TUI Section Pane Composition

## Goal

Tighten one bounded part of the existing `ilonasin manage` TUI without changing
daemon behavior:

1. Keep the four top-level sections as `api`, `providers`, `usage`, and `logs`.
2. Stabilize pane titles so long account and quota details live inside pane
   bodies, not chrome.
3. Make the Logs section more compact and visual so it reads less like a long
   text report.

This is intentionally not a full TUI redesign. Usage and Providers still need
further row-density passes after this slice.

## Scope

1. Keep the four existing tabs and pane-local scroll state.
2. Keep pane titles short and stable:
   - do not put long pooled quota summaries or selected email addresses in the
     pane title;
   - render those details inside the pane body where there is room to clip and
     scroll.
3. Make Logs denser:
   - render request metadata as compact blocks that emphasize route, status,
     time, model, token mix, latency, retries, and throughput;
   - keep fallback events as compact rows rather than repeated cards;
   - keep IO policy visible as a compact policy strip.
4. Keep account identities visible where already safely exposed by management
   DTOs and TUI sanitizers.
5. Add only small TUI rendering helpers if they reduce duplication or make row
   clipping more reliable.

## Boundaries

- No management API, DTO, storage, schema, provider, server route, Anthropic,
  keepalive, logging policy, or config changes.
- No direct `config.toml` mutation.
- No raw prompt, completion, request body, response body, SSE chunk, tool
  argument, tool result, bearer token, OAuth token, API key, full account ID, or
  request ID rendering.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary daemon and TUI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Run `manage` under 80, 120, 160, and 220 columns.
4. Confirm the API, providers, usage, and logs sections render.
5. Confirm pane-local scroll markers still appear on short terminals.
6. Confirm existing section-scoped actions still route to the intended panes:
   API token actions, provider API-key/OAuth/fallback actions, usage refresh,
   and logs pruning.
7. Remove all temporary artifacts.

Run a temporary focused render smoke, then remove it before commit:

- seed safe email-like OAuth and subscription labels plus unsafe marker strings;
- seed request, fallback, IO policy, safe account identity, unsafe marker, and
  subscription pool rows;
- assert safe email-like labels render;
- assert unsafe marker strings remain redacted;
- assert pane titles are stable labels and do not contain seeded email or quota
  text;
- assert pane IDs and pane order remain stable for API, providers, usage, and
  logs;
- assert Logs request and fallback rows render as compact row blocks, not
  repeated cards;
- assert output lines fit 80, 120, 160, and 220 column views after ANSI
  stripping.

## Acceptance

- The TUI still has only `api`, `providers`, `usage`, and `logs` as top-level
  sections.
- Logs panes use compact visual rows and avoid whole-screen report prose.
- Pane titles are short and stable.
- Pooled usage is summative, not averaged.
- Email-like account labels remain visible where safely exposed.
- Compile, vet, daemon smoke, manage smoke, focused render smoke, senior plan
  review, and senior implementation review complete.

# 189 TUI Temporal Density

## Context

The TUI already has subscription and observability cards with gauges, but many
time fields still use absolute `Jan 02 15:04` text and repeated labels such as
`at`, `reset`, and `retry`. The architecture calls the TUI a first-class
management interface, and the user specifically asked for more compact,
human-readable times in the system time zone and less text where visuals can
carry the meaning.

The worktree currently has concurrent logging/provider edits. This slice must
stay in the TUI render layer and avoid those files.

## Goal

Make time and status metadata in the TUI more compact and scan-friendly without
changing management DTOs, storage, provider behavior, routes, config, or update
flow.

After this slice:

- recent request, health, quota, fallback, OAuth, and subscription reset times
  render as local-time relative chips such as `now`, `4m ago`, `in 5h`, with a
  compact local clock suffix when useful;
- reset and retry timestamps are visually distinct chips rather than plain text;
- cards use shorter labels and avoid redundant prose on narrow terminals;
- existing gauges/cards remain width-bounded and ANSI-safe;
- no sensitive identifiers or raw payload data are newly rendered.

## Scope

1. Update TUI display helpers.
   - Add compact local-time helpers for past and future timestamps.
   - Use the model's injected `nowTime()` clock, not raw `time.Now()`, so PTY
     captures and future focused checks are deterministic.
   - Keep `time.Local` behavior and avoid UTC-only display.
   - Keep zero timestamps rendering empty.
   - Use concise thresholds: `now` below one minute, `Xm ago` or `in Xm` below
     one hour, `Xh ago HH:MM` or `in Xh HH:MM` below two days, `Xd ago` or
     `in Xd` below seven days, and `Jan 02 15:04` outside that range.
2. Update subscription usage rendering.
   - Replace `reset Jan 02 15:04` text with compact reset chips.
   - Keep existing usage and pool gauges unchanged except for label density.
3. Update observability renderers.
   - Replace `at`, `retry`, and `reset` absolute time chips with compact
     relative/local-time chips.
   - Keep status badges, metric chips, and existing bars.
4. Update OAuth account rendering only where it already shows expiry times.
5. Preserve privacy boundaries.
   - Render only existing sanitized display labels, provider IDs, credential
     IDs, plan labels, limit labels, percentages, status classes, counts, and
     timestamps.
   - Do not render bearer tokens, local API tokens, full upstream account IDs,
     prompts, completions, request bodies, response bodies, raw SSE chunks, tool
     data, raw provider payloads, or provider request IDs.
6. Do not add dependencies or permanent tests.

## Out of Scope

- New management DTO fields, routes, migrations, or provider adapters.
- New Bubble Tea messages, key bindings, tabs, animations, or storage writes.
- Changing subscription pooling math, usage fetching, keepalive, or logging.
- Touching concurrent logging/provider/server dirty files.

## Implementation Steps

1. Add time chip helpers to the TUI display/visual layer.
2. Replace timestamp call sites in subscription and observability renderers.
3. Review the diff for width handling and privacy.
4. Run `gofmt`.
5. Run compile, vet, diff whitespace, and direct serve/manage smokes.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
git diff --check
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
```

Then start `ilonasin serve` with a temporary `ILONASIN_HOME` and explicit
config, wait for the management socket health route, seed SQLite rows for recent
requests, health, quota, fallback, OAuth expiry, provider account, and
subscription reset windows, and capture `ilonasin manage` in both 120-column and
60-column PTYs. Use a short timeout or explicit process cleanup and remove
temporary directories afterward.

The seeded captures must assert that:

- cards contain relative time words like `ago` or `in`;
- subscription reset and quota retry/reset times are still visible;
- narrow subscription reset text preserves `ago`, `in`, or `now` rather than
  truncating to only a clock;
- targeted changed cards do not use the old absolute-only `Jan 02 15:04`
  pattern for fresh seeded rows;
- no capture contains secret-shaped values such as `bearer`, `sk-`, `iln_`,
  `access_token`, `refresh_token`, `id_token`, `raw`, `payload`, `prompt body`,
  `completion body`, `request_id`, `tool argument`, or `tool result`.

## Acceptance

- Timestamps in existing TUI cards are compact, local-time, and readable.
- Existing TUI cards and gauges remain bounded on narrow and wide terminals.
- No non-TUI runtime behavior changes.
- Compile, vet, whitespace, and direct serve/manage smokes pass.

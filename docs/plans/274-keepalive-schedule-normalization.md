# 274 Keepalive Schedule Normalization

## Goal

Make subscription keepalive schedule times reliable and human-readable without
changing quota policy or provider behavior.

The user-facing feature is configured as four local schedule slots. The current
default config uses `HH:MM`, but requested/operator shorthand such as `0700`
should not silently fail. The TUI should also render these slots as readable
local times rather than raw config strings.

## Current Evidence

- `internal/config/config.go` defaults subscription keepalive schedule times to
  `07:00`, `12:00`, `17:00`, and `22:00`.
- `internal/app/keepalive.go` only matches schedule entries exactly against
  `now.Format("15:04")`, so a valid-looking operator value like `0700` never
  runs.
- `internal/management/subscription_usage_keepalive.go` only accepts `HH:MM`,
  blanks invalid entries, and falls back only when the schedule is empty before
  sanitization.
- `internal/tui/usage_subscription.go` joins `ScheduleTimes` directly, so the
  usage pane shows raw strings rather than human-readable local schedule labels.

## Scope

1. Add a small shared normalization helper in the config package for
   subscription keepalive times:
   - accept `HH:MM` and `HHMM`;
   - reject non-zero-padded forms such as `7:00` and `700`;
   - validate hour `00` through `23` and minute `00` through `59`;
   - normalize accepted values to canonical `HH:MM`;
   - drop invalid values while preserving the order of valid values.
2. Use the helper when config defaults are applied so runtime config has a
   canonical schedule. Normalize non-empty configured values after loading, and
   restore defaults if every configured value is invalid.
3. Use the same helper in `keepaliveSlot` so the runner is robust even if called
   with raw schedule values. Return the canonical `HH:MM` slot so `0700` and
   `07:00` share the same completion key.
4. Use the same helper in management keepalive status so the TUI receives a
   sanitized canonical schedule with invalid entries filtered out.
5. Render keepalive schedule values in the TUI as compact local clock labels,
   for example `7:00 AM`, `12:00 PM`, `5:00 PM`, `10:00 PM`, while keeping the
   strings clipped and sanitized. Treat canonical schedule strings as local
   clock labels without timezone conversion.

## Boundaries

- No change to whether keepalive is enabled by default.
- No implementation of `subscription_keepalive.timezone` semantics in this
  slice; existing local-time matching behavior is preserved.
- No change to the keepalive prompt, model, output-token cap, provider
  selection, subscription usage refresh, OAuth resolution, credential pooling,
  or quota accounting.
- No direct SQLite or `config.toml` mutation from the TUI.
- No storage, schema, server route, Anthropic, OpenAI, or provider adapter
  changes.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run temporary focused checks, then remove them before commit:

- assert `NormalizeSubscriptionKeepaliveTimes` accepts `07:00`, `0700`,
  `00:00`, and `23:59`;
- assert non-zero-padded forms such as `7:00` and `700` are rejected;
- assert invalid entries like `24:00`, `12:60`, `abcd`, and blank strings are
  dropped;
- assert mixed schedules such as `0700`, `bad`, `22:00` normalize to `07:00`,
  `22:00` in order;
- assert duplicate valid values preserve current behavior unless a narrow code
  pass proves deduplication is required;
- assert all-invalid schedules fall back to defaults during config defaulting;
- assert loading an existing config normalizes in memory but does not rewrite
  `config.toml`;
- assert `keepaliveSlot` matches both canonical and shorthand schedule entries;
- assert `keepaliveSlot` returns the same canonical slot for `0700` and
  `07:00`;
- assert TUI usage rendering shows readable local schedule labels and still
  clips cleanly at narrow widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive enabled, and shorthand schedule values such as `0700`.
3. Verify the management health endpoint over the management socket.
4. Verify management snapshot returns canonical keepalive schedule times.
5. Run `manage` under a short timeout and verify the usage section chrome still
   renders.
6. Remove all temporary artifacts.

## Acceptance

- Operator shorthand `HHMM` no longer silently prevents keepalive from running.
- Management status exposes canonical keepalive schedule values.
- The TUI renders schedule times in readable local clock form.
- Compile, vet, focused checks, serve smoke, manage smoke, senior plan review,
  and senior implementation review pass.

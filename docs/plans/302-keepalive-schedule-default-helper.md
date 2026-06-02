# 302 Keepalive Schedule Default Helper

## Context

Plan 298 moved management keepalive status off `internal/config`, but the app
conversion helper still calls `config.Default("")` only to recover the default
subscription keepalive schedule. That couples a narrow runtime status conversion
to the full default config shape.

The config package already owns subscription keepalive time normalization.
Keeping default fallback there makes the boundary cleaner: callers should ask
for normalized keepalive schedule values, not reconstruct fallback behavior by
peeking into full default config.

This slice is behavior-preserving. It must not change keepalive enablement,
output-cap policy, schedule values, timezone semantics, provider behavior,
management JSON, TUI rendering, storage, credentials, or logs.

## Plan

1. Add `config.DefaultSubscriptionKeepaliveScheduleTimes() []string`, returning
   a fresh copy of the canonical default schedule:
   - `07:00`;
   - `12:00`;
   - `17:00`;
   - `22:00`.
2. Add `config.SubscriptionKeepaliveScheduleTimes(values []string) []string`
   that:
   - normalizes with the existing `NormalizeSubscriptionKeepaliveTimes`;
   - falls back to a fresh default schedule copy when normalization yields no
     valid times.
3. Use `DefaultSubscriptionKeepaliveScheduleTimes` in `Default` so the canonical
   default schedule is defined once.
4. Use the new helper in `Config.applyDefaults` instead of duplicating
   normalize-then-fallback logic there.
5. Use the new helper in `app.managementKeepaliveSettings` so app no longer
   calls `config.Default("")` for schedule fallback.
6. Use the new helper in `keepaliveSlot` so direct runner calls preserve the
   same fallback behavior if given raw or all-invalid schedule values.
7. Keep `NormalizeSubscriptionKeepaliveTimes` available for callers that
   deliberately want filter-only behavior.
8. Review the diff before checks for copy semantics, default values, and
   accidental behavior changes.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/config
go test ./internal/app
go test ./...
go vet ./...
```

Run a temporary focused smoke, then remove it before commit:

- assert `DefaultSubscriptionKeepaliveScheduleTimes` returns the four expected
  canonical defaults and returns a fresh slice;
- assert `config.Default("").SubscriptionKeepalive.ScheduleTimes` equals those
  helper defaults;
- assert `SubscriptionKeepaliveScheduleTimes([]string{"0700", "bad", "22:00"})`
  returns `07:00`, `22:00`;
- assert `SubscriptionKeepaliveScheduleTimes([]string{"bad"})` returns the four
  defaults and returns a fresh fallback slice;
- assert `managementKeepaliveSettings` uses the same fallback values;
- assert `keepaliveSlot` matches defaults when passed an invalid-only schedule
  and a matching local time.

Run direct CLI smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive enabled, and invalid-only schedule values.
3. Verify management health over the Unix socket.
4. Verify management snapshot and subscription usage responses expose the four
   canonical default schedule times and current unverified output-cap status.
5. Run `manage` under short PTY timeouts at narrow and wide widths and verify
   API, providers, usage, and logs render.
6. Remove all temporary artifacts and stop the daemon.

## Acceptance

- Keepalive schedule default fallback is centralized in `internal/config`.
- `internal/app` no longer calls `config.Default("")` for keepalive schedule
  fallback.
- Existing schedule normalization and management status behavior remain
  unchanged.
- No provider, storage, public API, TUI mutation, credential, or logging
  behavior changes are introduced.
- Focused compile, full compile, vet, direct serve/manage smoke, and senior
  implementation reviews pass.

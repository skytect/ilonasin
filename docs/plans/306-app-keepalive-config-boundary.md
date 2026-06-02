# 306 App Keepalive Config Boundary

## Context

`docs/ilonasin-architecture.md` treats `config.toml` as static bootstrap input.
Recent slices moved management and provider/logging boundaries away from direct
config-package coupling. The app keepalive runner and management bootstrap still
pass `config.SubscriptionKeepaliveConfig` through runtime helpers.

The app package should translate config into runtime settings once, then pass
those settings to keepalive execution and management status conversion. The
config package can continue to own TOML parsing, defaults, schedule
normalization, and output-cap verification helpers.

This slice is behavior-preserving. It must not change keepalive enablement,
schedule defaults, schedule normalization, output-cap policy, request model
fallback, max output token policy, management status JSON, provider behavior,
storage, TUI, or logging.

## Plan

1. Add an app-owned `subscriptionKeepaliveSettings` type with only runtime
   fields needed by app keepalive:
   - `Enabled`;
   - `ScheduleTimes`;
   - `Model`;
   - `MaxOutputTokens`;
   - `OutputCapVerified`.
2. Add `subscriptionKeepaliveSettingsFromConfig` in app to translate
   `config.SubscriptionKeepaliveConfig` using existing config helpers:
   - `config.SubscriptionKeepaliveScheduleTimes`;
   - `config.SubscriptionKeepaliveOutputCapVerified`.
3. Change `startSubscriptionKeepalive`, `keepaliveRunner`, and
   `managementKeepaliveSettings` to use app-owned settings.
4. Keep `keepaliveSlot` operating on already-normalized schedule values; it may
   preserve defensive normalization only if needed for direct helper calls.
5. Update app callsites in `internal/app/commands.go` to translate once.
6. Review code before checks for behavior drift, especially invalid-only
   schedules, disabled keepalive, output-cap unavailable status, model fallback,
   and management keepalive JSON.

## Verification

Run:

```sh
! rg -n 'config\.SubscriptionKeepaliveConfig' internal/app/keepalive.go internal/app/management.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/app
go test ./...
go vet ./...
```

Run a temporary focused smoke, then remove it before commit:

- translate disabled config and assert keepalive settings preserve disabled
  state and normalized schedule defaults;
- translate enabled config with shorthand, invalid, and duplicate schedules and
  assert normalized schedule values match config helper output;
- translate invalid-only schedule and assert default schedule values;
- assert management keepalive settings preserve enabled status, output-cap
  verification, and schedule values;
- assert command callsite helpers pass the same translated settings into
  management status and keepalive execution paths;
- assert enabled but unverified output cap returns a no-op runner, logs
  `unavailable_output_cap_unverified`, and does not call resolver, adapter, or
  usage clients;
- assert `keepaliveSlot` still matches expected local times;
- assert `keepaliveRequest` still falls back to `gpt-5.5` for empty model.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME` and config, checking management health over the
Unix socket, running `ilonasin manage` under bounded narrow and wide terminals,
and cleaning up the daemon and temp directory.

## Acceptance

- App keepalive execution and management status helpers no longer accept
  `config.SubscriptionKeepaliveConfig` directly.
- Config remains the owner of TOML parsing, defaults, normalization, and
  output-cap verification helpers.
- Runtime behavior and management keepalive status JSON remain unchanged.
- No provider, credential, storage, TUI, logging, public API, or config parser
  behavior changes are introduced.

# 298 Management Keepalive Config Boundary

## Context

`docs/ilonasin-architecture.md` separates static TOML bootstrap config from the
daemon-owned management API. The current management subscription usage status
code imports `internal/config` to normalize keepalive schedules, verify the
output cap, and call `config.Default("")` for fallback schedule slots.

That is a boundary smell. Management should expose daemon status using
management-shaped inputs. Config loading/defaulting should remain in the config
and app layers before the management service is constructed.

This slice is behavior-preserving. It must not change management response JSON,
keepalive execution policy, schedule defaults, SQLite, provider behavior, TUI
mutation behavior, or logging capture policy.

## Plan

1. Add a small management-owned `SubscriptionKeepaliveSettings` struct with only
   fields required to render management status:
   - `Enabled`;
   - `OutputCapVerified`;
   - canonical `ScheduleTimes`.
2. Replace `management.Service.Keepalive config.SubscriptionKeepaliveConfig`
   with `management.Service.Keepalive SubscriptionKeepaliveSettings`.
3. Move `keepaliveStatus` to use only the management settings:
   - disabled reports `status = "disabled"`;
   - enabled with `OutputCapVerified == false` reports
     `status = "unavailable_output_cap_unverified"`;
   - enabled with `OutputCapVerified == true` reports `status = "enabled"`;
   - schedule times are copied from the settings.
4. Add an app-layer conversion helper from
   `config.SubscriptionKeepaliveConfig` to
   `management.SubscriptionKeepaliveSettings`.
   The helper should use existing config helpers/defaults so current behavior is
   preserved:
   - normalize configured schedule times;
   - fall back to default schedule times if normalization yields no values;
   - set `OutputCapVerified` from
     `config.SubscriptionKeepaliveOutputCapVerified`.
5. Keep keepalive runner behavior untouched. The app keepalive runner still uses
   the full config struct because it owns scheduling/provider execution.
6. Review the diff before checks for boundary direction, response shape,
   schedule copying, and accidental behavior changes.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/management
go test ./internal/app
go test ./...
go vet ./...
! rg -n '"ilonasin/internal/config"' internal/management
```

Run a temporary focused check, then remove it before commit:

- construct a management service with disabled settings and assert keepalive
  status is disabled with the provided canonical schedule;
- construct enabled/unverified settings and assert
  `unavailable_output_cap_unverified`;
- construct enabled/verified settings and assert enabled;
- mutate the source schedule after calling status construction and assert the
  response schedule is not aliased to service state;
- assert app conversion normalizes `0700` to `07:00`;
- assert app conversion falls back to `07:00`, `12:00`, `17:00`, `22:00` when
  schedule normalization yields no values.

Run direct CLI smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive enabled, and shorthand/invalid schedule values.
3. Verify management health over the Unix socket.
4. Verify management snapshot and subscription usage responses expose
   canonical keepalive schedule times and current unverified output-cap status.
5. Run `manage` under short PTY timeouts at narrow and wide widths and verify
   API, providers, usage, and logs render.
6. Remove all temporary artifacts and stop the daemon.

## Acceptance

- `internal/management` no longer imports `internal/config`.
- Management keepalive status remains behaviorally equivalent.
- Config defaulting/normalization remains in config/app-owned code.
- No provider, storage, public API, TUI mutation, or keepalive runner behavior
  changes are introduced.
- Focused compile, full compile, vet, direct serve/manage smoke, and senior
  implementation reviews pass.

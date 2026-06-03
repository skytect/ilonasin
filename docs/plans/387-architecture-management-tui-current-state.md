# 387 Architecture Management TUI Current State

## Context

`docs/ilonasin-architecture.md` still contains stale migration language:

- direct TUI SQLite access is described as a legacy implementation detail to be
  removed progressively;
- telemetry pruning controls are described as something the TUI should provide
  later.

Current code shows the migration has moved forward:

- `ilonasin manage` uses `bootstrapClient`, creates a Unix management client,
  loads a management snapshot, and passes management clients into the TUI;
- `internal/tui` imports `internal/management` DTO/client interfaces, not
  `internal/storage/sqlite`;
- pruning is exposed through the management API and TUI:
  `POST /_ilonasin/manage/telemetry/prune`, `TelemetryPruneClient`, and the
  logs pruning pane/action.

## Goal

Update `docs/ilonasin-architecture.md` so the target architecture reflects the
current management/TUI boundary and pruning capability, without changing runtime
code.

## Scope

1. Replace stale direct-TUI-SQLite migration wording with the current target:
   - daemon owns SQLite reads and writes;
   - `ilonasin manage` is a local management API client;
   - the TUI must not add new direct storage or config mutation paths.
2. Update telemetry retention/pruning wording:
   - retention remains keep-forever until pruned;
   - manual pruning is available through the daemon-owned management API and
     TUI;
   - scheduled pruning remains an open policy question if still present.
3. Keep the architecture explicit that the TUI does not edit `config.toml`.
4. Do not change runtime code, storage schema, management routes, TUI layout, or
   pruning behavior.

## Verification

Run:

```sh
rg -n "direct TUI SQLite|pruning controls later|daemon-owned local management API|telemetry retention|manual pruning|scheduled" docs/ilonasin-architecture.md
rg -n "sqlite|storage/sqlite|database|config.toml" internal/tui internal/app/commands.go internal/app/runtime_core.go
rg -n "PathTelemetryPrune|PruneTelemetry|pruningBody|pruneTelemetryAction" internal/management internal/tui
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- The architecture no longer implies direct TUI SQLite access is current or
  tolerated for new work.
- The architecture no longer says TUI pruning controls are merely future work.
- The open question about manual versus scheduled pruning remains accurate.
- No runtime behavior changes.
- Compile/package checks, vet, and direct serve/manage smoke pass.

# 301 Codex Service Tier Helper Boundary

## Context

`docs/ilonasin-architecture.md` says provider adapters own provider-specific
behavior and the router core should not embed provider quirks beyond selecting
adapters and passing typed route options.

Plan 293 moved Codex-compatible model-list shaping behind the provider boundary.
`internal/provider/codex_model_list.go` still exports
`CodexFastServiceTier`, even though it is only used inside that same file as a
fallback for Codex-compatible model-list metadata. Keeping that helper exported
leaks an implementation detail as provider package API.

This slice is behavior-preserving. It must not change model-list JSON, model
discovery, provider HTTP parsing, server routing, management DTOs, TUI behavior,
storage, config, credentials, or logs.

## Plan

1. Rename `CodexFastServiceTier` to `codexFastServiceTier`.
2. Update the single same-file call site in `CodexModelInfoFromMetadata`.
3. Keep the returned `ModelServiceTier` fields unchanged:
   - `id = "priority"`;
   - `name = "Fast"`;
   - `description = "1.5x speed, increased usage"`.
4. Add source checks that prove no exported `CodexFastServiceTier` symbol
   remains and no external call sites exist.
5. Review the diff before checks to confirm this is a pure boundary tightening
   with no data-shape changes.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
! rg -n 'CodexFastServiceTier' internal
rg -n 'codexFastServiceTier' internal/provider/codex_model_list.go
go test ./internal/provider
go test ./...
go vet ./...
```

Run a temporary focused smoke, then remove it before commit:

- construct response-capable `provider.ModelMetadata` with `service_tier`
  capability and no explicit service tiers;
- call `CodexModelInfoFromMetadata`;
- assert the generated fallback service tier is still exactly `priority`,
  `Fast`, and `1.5x speed, increased usage`;
- assert a non-response-capable row is still excluded.

Run direct CLI smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, and at least two provider instances.
3. Verify management health over the Unix socket.
4. Run `manage` under short PTY timeouts at narrow and wide widths and verify
   API, providers, usage, and logs render.
5. Remove all temporary artifacts and stop the daemon.

## Acceptance

- Codex model-list fallback service tier remains provider-internal.
- Codex-compatible model-list data remains unchanged.
- No server, management, TUI, storage, config, credential, or provider runtime
  behavior changes are introduced.
- Focused compile, full compile, vet, direct serve/manage smoke, and senior
  implementation reviews pass.

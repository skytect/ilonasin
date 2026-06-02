# 277 Model Discovery Audit Alignment

## Goal

Align stale compatibility audit language with the current model-discovery
implementation.

Current code now resolves all currently eligible/materialized credentials for
model discovery and tries them in order:

- `internal/server/models.go` calls `resolveModelCredentials`;
- `internal/server/discoverModelsWithCredentials` iterates the returned pool and
  records health for each attempt;
- `internal/storage/sqlite/ResolveOAuthBearerCredentials` returns all currently
  eligible/materialized OAuth bearer credentials;
- `internal/storage/sqlite/ResolveAPIKeyCredentials` returns all enabled API-key
  credentials.

`docs/codex-compatibility-audit.md` and `docs/codex-client-red-team.md` still
describe the old hazard where a primary Codex credential 401 can hide secondary
accounts. That is now stale and can misdirect future architecture work.

## Scope

1. Update `docs/codex-compatibility-audit.md` to distinguish historical live
   audit evidence from current implementation status:
   - current code inspection shows model discovery uses pooled credential
     resolution;
   - current code inspection shows a 401 on one attempted Codex OAuth
     credential should not hide later currently eligible credentials;
   - health rows remain per-attempt metadata;
   - broad switch-gate and OpenRouter/tool-family blockers remain unchanged.
2. Update `docs/codex-client-red-team.md` model discovery row to reflect the
   current code status.
3. Do not change server, storage, provider, management, TUI, config, SQLite, or
   route behavior in this slice.

## Boundaries

- Documentation-only.
- No new behavior claims that are not backed by current code inspection.
- Do not erase historical smoke evidence. Reframe it as historical evidence and
  current resolved status.
- Do not claim the entire switch gate is clear.
- No permanent tests.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
! rg 'non-pooled model discovery still uses the primary credential only|primary credential health can hide valid secondary accounts|Primary Codex credential 401 can hide secondary credentials' docs/codex-compatibility-audit.md docs/codex-client-red-team.md
go test ./...
go vet ./...
```

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- Stale current-tense claims that model discovery is non-pooled are removed or
  marked historical.
- Current-tense variants of “primary credential health can hide valid secondary
  accounts” and “Primary Codex credential 401 can hide secondary credentials”
  are removed or marked historical.
- Remaining compatibility blockers are preserved.
- Docs do not claim live switch-gate readiness unless live credentials are
  rerun.
- The docs point future work at real remaining gaps rather than a fixed model
  discovery pooling issue.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

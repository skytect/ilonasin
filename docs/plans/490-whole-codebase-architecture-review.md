# 490 Whole Codebase Architecture Review

## Context

The active goal requires regular senior-engineer review of the whole codebase
against `docs/ilonasin-architecture.md`. The most recent whole-codebase review
checkpoint was plan 478. Since then, slices changed supporting architecture
docs, TUI density, endpoint and usage presentation, live snapshot refresh,
downstream local-token usage visibility, IO log retention, typed IO metadata,
and routing policy visibility.

Run a fresh whole-codebase review before selecting the next implementation
slice.

## Goal

Obtain three independent senior-engineer reviews of the current codebase and
docs, focused on fidelity to `docs/ilonasin-architecture.md`, and record
concrete findings for future slices.

## Scope

1. Ask three senior subagents to review the entire current codebase and docs.
2. Reviewers should inspect at least:
   - `docs/ilonasin-architecture.md`;
   - all supporting markdown under `docs/**`;
   - server routing, provider adapters, credentials, storage, management, TUI,
     logging, metadata, privacy, config, app bootstrap, and CLI packages.
3. Ask reviewers to return only concrete findings, each with:
   - file and line reference;
   - severity;
   - architecture requirement violated or maintainability issue;
   - recommended next slice boundary.
4. Persist the three review outputs and selected follow-up findings in this
   plan before committing the slice.
5. Treat findings as input to future slices. Do not implement runtime fixes in
   this slice.
6. Do not add permanent tests.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary SQLite, IO
   capture disabled, keepalive disabled, and configured provider instances.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at narrow and wide terminal widths.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- Three senior reviewers complete whole-codebase architecture reviews.
- Concrete findings are recorded in this plan.
- Selected follow-up findings are listed for future slices.
- No runtime behavior changes are made.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Review Record

### Reviewer 1

Severity: Medium

- Finding: Several TUI mutations still perform daemon management calls
  synchronously in the Bubble Tea update path, which can freeze
  `ilonasin manage` during socket, SQLite, provider refresh, or pruning work.
- References:
  - `internal/tui/api_local_token_actions.go:32`
  - `internal/tui/api_local_token_actions.go:51`
  - `internal/tui/provider_api_key_actions.go:32`
  - `internal/tui/oauth_actions.go:63`
  - `internal/tui/oauth_actions.go:94`
  - `internal/tui/usage_log_actions.go:31`
  - `internal/tui/usage_log_actions.go:56`
  - `docs/ilonasin-architecture.md:42`
  - `docs/ilonasin-architecture.md:570`
- Requirement or issue: the TUI is meant to be a first-class, responsive
  management client. Mutable operations correctly go through the daemon API, but
  blocking them inside update handlers undermines the UI architecture.
- Recommended next slice: TUI async mutation boundary. Move local-token
  create/disable, API-key add, OAuth refresh, and prune calls into `tea.Cmd`
  message flows. Preserve management API routes, storage, DTOs, keybindings,
  and rendered data.

Severity: Low

- Finding: The downstream-token aggregate strip excludes unknown or
  deleted-token usage, so the visible downstream usage total is
  current-token-only while `management.LocalTokenUsage` contains all retained
  downstream usage. This is documented in plan 487, but the TUI label itself
  does not make the narrower scope obvious.
- References:
  - `internal/tui/api_local_tokens.go:90`
  - `internal/tui/api_local_tokens.go:91`
  - `internal/tui/api_local_tokens.go:113`
  - `docs/plans/487-tui-downstream-usage-strip.md:25`
  - `docs/ilonasin-architecture.md:584`
  - `docs/ilonasin-architecture.md:585`
- Requirement or issue: usage views should be operator-legible and
  metadata-only. A current-token-only aggregate next to a separate
  unknown/deleted row can be misread as all downstream usage.
- Recommended next slice: TUI local-token usage label clarity. Rename or chip
  the aggregate as current tokens, and keep unknown/deleted usage in its
  separate row. No storage, management DTO, routing, logging, or keybinding
  changes.

Severity: Low

- Finding: Management snapshots set `PruningAvailable = true`
  unconditionally, even when `Service.Pruner` is nil and the prune route would
  return unavailable.
- References:
  - `internal/management/snapshot.go:70`
  - `internal/management/pruning.go:40`
  - `internal/management/pruning.go:41`
  - `internal/management/http.go:146`
  - `docs/ilonasin-architecture.md:482`
  - `docs/ilonasin-architecture.md:590`
- Requirement or issue: daemon-owned management state should accurately
  describe available operations. The current DTO can advertise pruning in
  service configurations where the operation is unavailable.
- Recommended next slice: management pruning availability truth source. Set
  `PruningAvailable` from `s.Pruner != nil` and update TUI display only if
  needed. No storage schema or pruning behavior changes.

### Reviewer 2

Severity: Medium

- Finding: A permanent Go test file remains in the codebase, while repository
  instructions say not to keep permanent test files and to use direct compile,
  vet, and CLI smoke checks instead.
- References:
  - `internal/openai/responses_test.go:8`
- Requirement or issue: permanent tests conflict with the repository workflow
  for this project and create stale-maintenance surface outside the intended
  smoke-check strategy.
- Recommended next slice: remove the permanent Responses tests and replace
  their coverage with documented temporary focused smoke commands or
  CLI/provider harness checks that are deleted before commit.

### Reviewer 3

Severity: High

- Finding: Provider `base_url` accepts HTTPS URLs with userinfo, query, or
  fragment, unlike `auth_issuer`. This risks embedding secret-bearing URL
  components into outbound provider construction and config-derived runtime
  state.
- References:
  - `internal/provider/provider.go:172`
- Requirement or issue: config/provider boundaries must keep provider
  credentials separate from config and avoid secret-bearing values in logs and
  snapshots.
- Recommended next slice: provider base URL sanitizer. Make
  `validateHTTPSBaseURL` reject userinfo, query, and fragment, then smoke config
  load failure.

Severity: Medium

- Finding: `ProviderAccount` exposes both local account row ID and credential
  ID in the management snapshot. The architecture says full account IDs must not
  be exposed and observable account references should use local credential IDs,
  safe display labels, or one-way account hashes. The extra account row ID is
  not needed by current TUI rendering and creates another stable identity
  surface.
- References:
  - `internal/management/snapshot_dto.go:103`
- Requirement or issue: management snapshots should expose the minimum safe
  account identity needed for current management operations and rendering.
- Recommended next slice: management provider account ID boundary. Remove
  `ProviderAccount.ID` from DTO/conversion unless a current management
  operation requires it.

Severity: Medium

- Finding: Downstream aggregate usage excludes unknown/deleted token usage,
  while the empty state totals all usage. This makes API usage totals
  inconsistent across states and undermines downstream API-key usage monitoring.
- References:
  - `internal/tui/api_local_tokens.go:90`
  - `internal/tui/api_local_tokens.go:229`
- Requirement or issue: TUI downstream usage aggregates should have one clear
  semantic across normal and empty states.
- Recommended next slice: TUI downstream usage total boundary. Define one
  aggregate semantic, likely all downstream usage plus a separate
  unknown/deleted row, and update labels.

Severity: Low

- Finding: Runtime version output shells out to `git` from the process current
  working directory. A built binary outside the repo can lose commit subject
  even when build info has a revision, and CLI identity depends on ambient
  filesystem state.
- References:
  - `internal/cli/version.go:99`
- Requirement or issue: CLI version identity should be stable from build
  metadata where possible, with any local git fallback isolated as development
  behavior.
- Recommended next slice: CLI version build-info boundary. Prefer ldflags or
  build info only, keep local git fallback strictly development-only and clearly
  isolated.

## Selected Follow-Up Findings

1. Provider base URL sanitizer. This is the highest-severity finding because it
   concerns config-derived provider boundaries and potential secret-bearing URL
   components.
2. TUI async mutation boundary. Multiple mutation handlers still block inside
   the update path and should move to `tea.Cmd` message flows.
3. Management provider account ID boundary. Remove unneeded stable account row
   IDs from snapshots if current management operations do not require them.
4. Downstream local-token usage aggregate clarity. Define and render one
   consistent aggregate semantic while keeping unknown/deleted usage visible.
5. Management pruning availability truth source. Report pruning availability
   from the actual service dependency.
6. Permanent Responses test cleanup. Remove `internal/openai/responses_test.go`
   or replace coverage with temporary smoke commands per repository workflow.
7. CLI version build-info boundary. Avoid depending on ambient repository state
   for normal version output.

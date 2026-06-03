# 442 Whole Codebase Architecture Review

## Context

The active goal requires regular senior-engineer review of the whole codebase
against `docs/ilonasin-architecture.md`, not only narrow slice reviews. Since
plan 426, follow-up slices addressed the recorded review findings and several
TUI polish slices changed management presentation.

Run a fresh whole-codebase review before selecting more implementation work.

## Goal

Obtain three independent senior-engineer reviews of the current codebase and
docs, focused on fidelity to `docs/ilonasin-architecture.md`, and record
concrete findings for future slices.

## Scope

1. Ask three senior subagents to review the entire current codebase and docs.
2. Reviewers should inspect at least:
   - `docs/ilonasin-architecture.md`;
   - `docs/codex-auth.md`;
   - `docs/codex-endpoints.md`;
   - `docs/deepseek-api.md`;
   - `docs/openrouter-api.md`;
   - `docs/deepseek-openrouter-comparison.md`;
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
   capture disabled, keepalive disabled, and configured DeepSeek/Codex provider
   instances.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at narrow and wide terminal widths.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- Three senior reviewers complete whole-codebase architecture reviews.
- Concrete findings are recorded in this plan.
- No runtime behavior changes are made.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Review Record

### Reviewer 1

Severity: Low

- Finding: The API pane labels the inventory as `Local API surfaces` but only
  renders `/v1/models`, `/v1/chat/completions`, `/v1/responses`, and
  `/v1/messages`. It omits live documented routes `/models`, `/responses`, and
  the concrete `/v1/messages/count_tokens` path, treating count-tokens as only
  a capability chip.
- References:
  - `internal/tui/control_sections.go:42`
  - `internal/tui/control_sections.go:48`
  - `internal/tui/control_sections.go:55`
  - `internal/tui/control_sections.go:59`
  - `internal/server/handler.go:7`
  - `internal/server/handler.go:9`
  - `internal/server/handler.go:13`
  - `docs/ilonasin-architecture.md:180`
  - `docs/ilonasin-architecture.md:195`
- Requirement or issue: the TUI should be a first-class management surface and
  should accurately show the implemented local compatibility API surface.
- Recommended next slice: TUI-only API surface inventory alignment in
  `internal/tui/control_sections.go`, preserving route behavior, DTOs, auth,
  storage, provider behavior, and config.

### Reviewer 2

Severity: Medium

- Finding: The API pane under-represents the implemented and documented local
  API surface. It reports `surfaces 3` and shows only the grouped API families,
  while concrete routes `/models`, `/responses`, and
  `/v1/messages/count_tokens` are live.
- References:
  - `internal/tui/control_sections.go:42`
  - `internal/tui/control_sections.go:48`
  - `internal/tui/control_sections.go:55`
  - `docs/ilonasin-architecture.md:182`
  - `docs/ilonasin-architecture.md:636`
- Requirement or issue: management/TUI views should accurately represent the
  local API compatibility surfaces listed by the active architecture.
- Recommended next slice: TUI-only API surface inventory alignment in
  `internal/tui/control_sections.go`, preserving routes, DTOs, auth, storage,
  and provider behavior.

Severity: Medium

- Finding: Server core still owns provider-specific policy decisions for
  Codex/OpenRouter Responses conversion, Codex Anthropic generation-option
  omission, and Codex stream error exposure.
- References:
  - `internal/server/provider_policy.go:12`
  - `internal/server/provider_policy.go:29`
  - `internal/server/provider_policy.go:39`
  - `docs/ilonasin-architecture.md:529`
  - `docs/ilonasin-architecture.md:543`
- Requirement or issue: provider-specific behavior should live at provider
  adapter or dependency-neutral provider policy boundaries. Router core should
  not embed provider-specific quirks beyond adapter selection and typed route
  options.
- Recommended next slice: move remaining provider policy selection out of
  `internal/server/provider_policy.go` into provider-owned or
  dependency-neutral policy helpers, preserving exact behavior.

### Reviewer 3

PASS: no findings.

### Selected Follow-Up Findings

1. TUI API surface inventory alignment. Two reviewers independently found this
   drift, and it is a narrow TUI-only slice.
2. Server provider policy boundary cleanup. This is a deeper architecture slice
   and should follow the route-inventory cleanup.

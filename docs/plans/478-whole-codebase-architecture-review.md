# 478 Whole Codebase Architecture Review

## Context

The active goal requires regular senior-engineer review of the whole codebase
against `docs/ilonasin-architecture.md`. The most recent whole-codebase review
checkpoint was plan 450. Since then, many slices changed TUI presentation,
logging presentation helpers, provider-policy boundaries, affinity wording, and
architecture inventory.

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
   capture disabled, keepalive disabled, and configured DeepSeek/Codex provider
   instances.
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

Severity: Low

- Finding: Supporting provider docs still describe colon-prefixed model
  namespaces, while the active architecture and live router use slash model
  addresses.
- References:
  - `docs/deepseek-openrouter-comparison.md:30`
  - `docs/deepseek-openrouter-comparison.md:55`
  - `docs/ilonasin-architecture.md:218`
  - `internal/routing/model.go:13`
- Requirement or issue: supporting docs should not contradict the current model
  addressing contract: `<provider_instance_id>/<provider_model_id>`.
- Recommended next slice: docs-only cleanup of
  `docs/deepseek-openrouter-comparison.md` model namespace wording. Preserve
  runtime behavior and historical plan files.

### Reviewer 2

Severity: Medium

- Finding: Anthropic routes accept `X-Api-Key` as a local client token, while
  the active architecture requires local API requests to use
  `Authorization: Bearer <ilonasin_token>`. This is likely intentional
  compatibility, but it is architecture drift without an explicit exception.
- References:
  - `internal/server/auth.go:46`
  - `docs/ilonasin-architecture.md:157`
- Requirement or issue: local API auth requirements and compatibility aliases
  must be explicit and auditable.
- Recommended next slice: local auth compatibility policy, either document
  `X-Api-Key` as an Anthropic-only alias with privacy constraints, or remove it.

Severity: Medium

- Finding: Several TUI action handlers still perform management API calls
  synchronously before returning a Bubble Tea command. This preserves the daemon
  boundary, but it can freeze the TUI on socket, SQLite, prune, or OAuth refresh
  work and undercuts the async live-refresh direction already used for
  snapshots.
- References:
  - `internal/tui/api_local_token_actions.go:32`
  - `internal/tui/provider_api_key_actions.go:32`
  - `internal/tui/oauth_actions.go:94`
  - `internal/tui/usage_log_actions.go:56`
- Requirement or issue: the TUI should remain a polished, responsive management
  client while mutations go through the daemon-owned management API.
- Recommended next slice: TUI async management mutations, move
  create/disable/refresh/prune calls into `tea.Cmd` message flows with
  in-flight state and foreground refresh chaining.

### Reviewer 3

Severity: High

- Finding: `AGENTS.md` says "The TUI may mutate SQLite", which conflicts with
  the active architecture requirement that `ilonasin manage` must be a client of
  the daemon-owned management API and must not read or write SQLite directly.
- References:
  - `AGENTS.md:30`
  - `docs/ilonasin-architecture.md:128`
- Requirement or issue: repository guidance should not preserve stale direct TUI
  SQLite mutation policy.
- Recommended next slice: docs-only constraint cleanup, update `AGENTS.md` and
  stale plan wording that still permits direct TUI SQLite mutation.

Severity: Medium

- Finding: The DeepSeek/OpenRouter comparison doc recommends storing
  `provider_usage` JSON. That risks contradicting the metadata-only architecture
  and the ban on raw provider payload storage.
- References:
  - `docs/deepseek-openrouter-comparison.md:40`
  - `docs/ilonasin-architecture.md:390`
- Requirement or issue: only normalized metadata fields may be stored unless IO
  logging is enabled.
- Recommended next slice: provider research-doc reconciliation, clarify that
  only normalized metadata fields may be stored unless IO logging is enabled.

Severity: Medium

- Finding: The Codex auth doc calls "credits, and request IDs" first-class
  rate-limit telemetry. The architecture forbids full provider request IDs and
  treats billing, credits, balances, and plan-limit querying as provider-policy
  risk unless separately approved.
- References:
  - `docs/codex-auth.md:206`
  - `docs/ilonasin-architecture.md:390`
  - `docs/ilonasin-architecture.md:335`
- Requirement or issue: supporting docs should align with metadata-only
  observability and provider-policy constraints.
- Recommended next slice: Codex research-doc alignment, revise to say redacted
  or normalized quota metadata only, with no full request IDs or credit/billing
  persistence by default.

## Selected Follow-Up Findings

1. AGENTS direct-TUI-SQLite wording. This is the highest-severity finding
   because active repository guidance conflicts with the architecture's
   daemon-owned SQLite boundary.
2. Local auth compatibility policy. Decide and document whether Anthropic
   `X-Api-Key` is an explicit compatibility alias for local ilonasin client
   tokens, or remove the alias.
3. Supporting provider docs metadata cleanup. Align stale
   `docs/deepseek-openrouter-comparison.md` and `docs/codex-auth.md` wording so
   slash model addressing, normalized metadata-only storage, quota policy, and
   request-ID privacy match the active architecture.
4. TUI async management mutations. Convert synchronous management mutation
   calls in action handlers into Bubble Tea command flows with in-flight state
   and refresh chaining.

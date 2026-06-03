# 450 Whole Codebase Architecture Review

## Context

The active goal requires regular senior-engineer review of the whole codebase
against `docs/ilonasin-architecture.md`. Plan 442 found two concrete follow-up
items:

- TUI API surface inventory drift;
- server-owned provider route policy.

Plans 443 and 449 addressed those selected findings, and plans 444 through 448
changed TUI presentation. Run a fresh whole-codebase review before selecting
the next implementation slice.

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
- Selected follow-up findings are listed for future slices.
- No runtime behavior changes are made.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Review Record

### Reviewer 1

Severity: Low

- Finding: Chat option metadata policy selection is still a separate
  provider-type policy path from the newer provider route policy boundary.
  Server request metadata calls
  `provider.ChatOptionMetadataPolicyForProviderType(instance.Type)` directly,
  while route conversion/exposure policy now goes through
  `provider.RoutePolicyForInstance(instance)`. This leaves provider-specific
  Chat metadata policy split across two selector surfaces and keeps raw provider
  type selection visible at the server request metadata boundary.
- References:
  - `internal/server/request_metadata_chat.go:29`
  - `internal/provider/chat_option_metadata.go:27`
  - `internal/provider/chat_option_metadata.go:29`
  - `internal/provider/chat_option_metadata.go:31`
  - `internal/provider/chat_option_metadata.go:33`
  - `internal/provider/route_policy.go:24`
  - `docs/ilonasin-architecture.md:529`
  - `docs/ilonasin-architecture.md:543`
- Requirement or issue: provider-specific behavior should live behind
  provider-owned policy boundaries, and router core should avoid embedding
  provider-specific quirks beyond adapter selection and typed route options.
- Recommended next slice: fold Chat option metadata policy into the
  provider-owned route policy, or add an instance-based provider policy helper,
  then have server request metadata consume the neutral policy without calling a
  raw provider-type selector. Preserve exact metadata values and early
  zero-policy behavior.

### Reviewer 2

Severity: Medium

- Finding: The architecture's SQLite table boundary omits two live durable
  metadata tables: `quota_events` and `subscription_usage_snapshots`. This
  conflicts with the same document's quota/subscription architecture,
  especially metadata-only quota rows and subscription usage views.
- References:
  - `docs/ilonasin-architecture.md:586`
  - `docs/ilonasin-architecture.md:602`
  - `internal/storage/sqlite/migrations.go:284`
  - `internal/storage/sqlite/migrations.go:326`
  - `internal/storage/sqlite/events.go:72`
  - `internal/storage/sqlite/subscription_usage.go:12`
- Requirement or issue: active architecture should accurately describe durable
  SQLite state boundaries for quota and subscription usage metadata.
- Recommended next slice: docs-only architecture alignment for SQLite
  durable-state boundaries, adding `quota_events` and
  `subscription_usage_snapshots` while preserving migrations and runtime
  behavior.

### Reviewer 3

Severity: Medium

- Finding: Server still derives Chat option metadata policy from raw provider
  type via `provider.ChatOptionMetadataPolicyForProviderType(instance.Type)`.
  This preserves behavior, but leaves a provider-policy selector shape outside
  the provider-owned `RoutePolicyForInstance` consolidation and keeps future
  provider additions split across policy factories.
- References:
  - `internal/server/request_metadata_chat.go:29`
  - `docs/ilonasin-architecture.md:543`
- Requirement or issue: router core should not embed provider-specific quirks
  beyond adapter selection and typed options.
- Recommended next slice: add a provider-owned instance-based Chat metadata
  policy helper, or fold it into provider route policy, while preserving early
  zero-policy behavior.

Severity: Low

- Finding: Supporting docs still recommend internal model prefixes like
  `deepseek:<model>` and `openrouter:<slug>`, but the active architecture and
  implementation use slash model addresses.
- References:
  - `docs/deepseek-openrouter-comparison.md:55`
  - `docs/ilonasin-architecture.md:217`
  - `internal/routing/model.go:13`
- Requirement or issue: supporting docs should not contradict the current model
  addressing scheme.
- Recommended next slice: docs-only alignment of supporting model-namespace
  wording to current `<provider_instance_id>/<provider_model_id>` addressing.

### Selected Follow-Up Findings

1. Chat option metadata provider-policy boundary. Two reviewers independently
   found the same provider-specific policy split at
   `internal/server/request_metadata_chat.go:29`; this should be the next code
   slice.
2. SQLite durable-state architecture inventory. Record `quota_events` and
   `subscription_usage_snapshots` in the active architecture in a docs-only
   slice.
3. Supporting model-addressing wording. Update
   `docs/deepseek-openrouter-comparison.md` to use slash provider/model
   addressing instead of stale colon prefixes.

# 426 Whole Codebase Architecture Review

## Context

The active goal requires regular senior-engineer review of the whole codebase,
not only narrow slice reviews. Recent slices tightened provider request policy,
credential pooling helpers, affinity hashing, and stream quota normalization.

Before choosing the next implementation slice, run a fresh whole-codebase
architecture review against `docs/ilonasin-architecture.md` and the supporting
docs under `docs/**`. The purpose is to find concrete remaining architecture
drift, dead/stale code, duplication, or maintainability risks that should drive
future slices.

## Goal

Obtain three independent senior-engineer codebase reviews focused on whether
the current codebase is faithful to `docs/ilonasin-architecture.md`, and record
the review findings in this plan for follow-up work.

## Scope

1. Ask three senior subagents to review the entire current codebase and docs.
2. Reviewers should inspect at least:
   - `docs/ilonasin-architecture.md`;
   - provider behavior docs: `docs/codex-auth.md`, `docs/codex-endpoints.md`,
     `docs/deepseek-api.md`, `docs/openrouter-api.md`, and
     `docs/deepseek-openrouter-comparison.md`;
   - server routing, provider adapters, credentials, storage, management, TUI,
     logging, metadata, and config packages.
3. Ask reviewers to return only concrete findings, each with:
   - file and line reference;
   - severity;
   - architecture requirement violated or maintainability issue;
   - recommended next slice boundary.
4. Persist the three review outputs and selected follow-up findings in this plan
   before committing the slice.
5. Treat disagreements or non-blocking observations as input to future slices,
   not as automatic code changes in this slice.
6. Do not change runtime behavior or production code in this slice. Only edit
   this review plan, and only for the review record or narrow wording fixes.
7. Do not add permanent tests.

## Verification

Run:

```sh
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

- Three senior reviewers complete whole-codebase architecture reviews.
- Findings are summarized in this plan before commit.
- No runtime behavior changes are made.
- The repository remains clean after committing the review-plan slice.

## Review Record

### Reviewer 1

Severity: Medium

- Finding: Codex OAuth eligibility is duplicated across package-local
  boundaries, so the same `type == "codex" && OAuth` rule can drift
  independently.
- References:
  - `internal/credentials/provider_boundary.go:21`
  - `internal/management/snapshot_dto.go:43`
  - `internal/app/keepalive_provider_adapters.go:27`
  - `internal/server/provider_policy.go:44`
- Requirement or issue: `docs/ilonasin-architecture.md` keeps OAuth-capable
  provider behavior as an explicit provider/account boundary, and future
  provider behavior should be adapter/policy driven. The duplication is
  behavior-preserving today, but makes Codex OAuth refresh, management
  visibility, TUI login selection, and keepalive eligibility easy to change
  inconsistently.
- Recommended next slice: add a dependency-neutral provider capability helper
  over provider type and OAuth booleans, then replace duplicated Codex OAuth
  predicates while preserving DTO isolation and exact behavior.

### Reviewer 2

Severity: Medium

- Finding: local token labels are passed through raw in the management
  local-token DTO path, while snapshot surfaces sanitize labels.
- References:
  - `internal/management/tokens.go:129`
  - `internal/management/snapshot_sanitize.go:18`
- Requirement or issue: management surfaces must expose safe token metadata and
  fragments only, and observable labels should be sanitized before management
  or TUI rendering.
- Recommended next slice: sanitize local token DTO conversion at the management
  boundary, preserving the intentionally full token only in
  `CreateLocalTokenResponse.Token`.

Severity: Low

- Finding: `normalizeModels` has a `case "codex"` fallback capability branch
  that is unreachable because `instance.Type == "codex"` returns through
  `normalizeCodexModels`.
- Reference: `internal/provider/http_models.go:203`
- Requirement or issue: provider adapters should keep provider-specific model
  discovery behavior auditable and avoid dead fallback policy.
- Recommended next slice: remove the unreachable `case "codex"` branch or make
  the fallback path explicit if it is genuinely needed.

Severity: Low

- Finding: supporting docs recommend encrypted local credential storage, while
  the architecture explicitly says initial SQLite is plaintext with file
  permissions and redaction.
- References:
  - `docs/codex-auth.md:222`
  - `docs/ilonasin-architecture.md:122`
- Requirement or issue: supporting docs should not contradict the target
  architecture.
- Recommended next slice: update `docs/codex-auth.md` wording to distinguish
  generic router advice from Ilonasin's current plaintext SQLite decision.

### Reviewer 3

Severity: Low

- Finding: the architecture source labels itself as a draft architecture plan
  and frames the implemented route/provider/storage/TUI surface as an MVP
  target.
- References:
  - `docs/ilonasin-architecture.md:3`
  - `docs/ilonasin-architecture.md:630`
- Requirement or issue: current code and recent slices treat this file as the
  active architecture source of truth, so stale status wording weakens review
  alignment.
- Recommended next slice: docs-only architecture status/current-state wording
  cleanup.

Severity: Low

- Finding: subscription account fallback under provider terms appears both
  under deferred research and as a separate open question.
- References:
  - `docs/ilonasin-architecture.md:621`
  - `docs/ilonasin-architecture.md:625`
- Requirement or issue: one unresolved provider-terms policy appears as two
  different architecture gaps.
- Recommended next slice: docs-only deferred-research/open-question
  consolidation.

Severity: Low

- Finding: the storage section still says exact schema is deferred, while the
  live SQLite migration set is established and surrounding architecture relies
  on concrete metadata-only tables and boundaries.
- References:
  - `docs/ilonasin-architecture.md:586`
  - `internal/storage/sqlite/migrations.go`
- Requirement or issue: stale architecture wording, not a migration bug.
- Recommended next slice: docs-only storage-current-surface wording update,
  keeping historical migrations untouched.

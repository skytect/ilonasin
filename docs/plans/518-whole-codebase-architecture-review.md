# 518 Whole Codebase Architecture Review

## Context

Plan 517 closed the selected follow-up from plan 516 by rejecting unknown
top-level fields on known Responses `input[]` item families before provider
dispatch.

The user-requested architecture-alignment workflow requires regular
whole-codebase senior-engineer review against `docs/ilonasin-architecture.md`
and supporting docs under `docs/**`. Run a fresh checkpoint before selecting
another implementation slice.

## Goal

Obtain three independent senior-engineer reviews of the current codebase and
docs, focused on fidelity to `docs/ilonasin-architecture.md`, and record
concrete findings for future implementation slices.

## Scope

1. Ask three senior subagents to review this plan before execution.
2. Ask three senior subagents to review the entire current codebase and docs.
3. Reviewers should inspect at least:
   - `docs/ilonasin-architecture.md`;
   - supporting markdown under `docs/**`;
   - server routing, provider adapters, credentials, storage, management, TUI,
     logging, metadata, privacy, config, app bootstrap, and CLI packages.
4. Ask reviewers to return only concrete findings, each with:
   - file and line reference;
   - severity;
   - architecture requirement violated or maintainability issue;
   - recommended next slice boundary.
5. Persist the three review outputs and selected follow-up findings in this
   plan before committing the slice.
6. Treat findings as input to future slices. Do not implement runtime fixes in
   this slice.
7. Do not add permanent tests.

## Worktree Isolation

This slice may commit only:

- `docs/plans/518-whole-codebase-architecture-review.md`.

Do not push.

If verification observes failures caused by unrelated dirty runtime files, note
that in the review record instead of folding those changes into this slice.

## Verification

Run:

```sh
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/518-whole-codebase-architecture-review.md
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
5. Confirm TUI output includes ANSI color sequences.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- Three senior reviewers complete whole-codebase architecture reviews.
- Concrete findings are recorded in this plan.
- Selected follow-up findings are listed for future slices.
- No runtime behavior changes are made, staged, or committed by this slice.
- The slice is committed locally only and is not pushed.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Review Record

### Reviewer 1

Severity: Medium

- Finding: Codex Responses tool preservation accepts unknown tool declaration
  families as raw upstream payload. `validateCodexResponsesTool` validates only
  `function` and `namespace` shapes, then the default case accepts all other
  `tools[n].type` values unchanged. `responsesToolsToChatTools` preserves those
  raw tools for Codex routes.
- References:
  - `internal/openai/responses_tools.go:48`
  - `internal/openai/responses_tools.go:135`
  - `internal/openai/responses_tools.go:178`
  - `docs/ilonasin-architecture.md:211`
  - `docs/ilonasin-architecture.md:218`
- Requirement or issue: provider-specific escape hatches must be explicit and
  namespaced, and preserved Codex-native Responses tool declarations must be
  validated. Unknown tool families should not silently pass through as a broad
  raw provider escape hatch.
- Recommended next slice: Codex Responses tool declaration allowlist. Keep
  existing validated `function` and `namespace` handling, add explicit
  per-family validation for any proven Codex-native tool families that remain
  preserved, and reject unknown tool types locally with
  `tools[n].type is unsupported`.

### Reviewer 2

Severity: Medium

- Finding: `validateCodexResponsesTool` validates only partial `function` and
  `namespace` shape, then accepts all other tool types unchanged. This lets
  arbitrary client tool declarations be forwarded on Codex routes as raw
  upstream payload.
- References:
  - `internal/openai/responses_tools.go:135`
  - `internal/openai/responses_tools.go:178`
  - `docs/ilonasin-architecture.md:211`
  - `docs/ilonasin-architecture.md:218`
- Requirement or issue: Codex Responses tool preservation is allowed only for
  validated Codex-native declarations. Unknown tool declarations are not an
  explicit namespaced provider escape hatch.
- Recommended next slice: Codex Responses tool declaration allowlists. Keep
  currently proven Codex-native tool families and require explicit per-type
  validation, or reject unknown tool declaration families before
  `CodexResponsesTools` preservation.

### Reviewer 3

Severity: Medium

- Finding: `IsSensitiveLogKey` redacts credential keys plus `header`, `body`,
  `payload`, `raw`, and `stdout`, but the structured logging plan requires the
  normal logging guard to redact broader sensitive key families such as
  `account`, `request_id`, `generation_id`, `url`, `uri`, `host`, `path`,
  `query`, `prompt`, and `completion`.
- References:
  - `internal/logging/secrets.go:133`
  - `docs/plans/077-structured-application-logging.md:45`
  - `docs/ilonasin-architecture.md:401`
- Requirement or issue: normal logs should defensively redact sensitive
  attribute key families even if a future log call accidentally uses them. The
  current guard is narrower than the documented logging boundary.
- Recommended next slice: normal structured-log redaction alignment. Expand or
  reconcile `IsSensitiveLogKey` with the documented sensitive attribute
  families, with a temporary focused harness covering representative keys and
  no provider, storage, TUI, or route behavior changes.

## Selected Follow-Up Findings

1. Codex Responses tool declaration allowlist. This is selected first because
   two independent reviewers found it, it sits directly on the provider
   dispatch boundary, and it can silently preserve unknown client payload
   families upstream.
2. Normal structured-log redaction alignment. This should follow as a narrow
   privacy-hardening slice.

## Verification Record

- Senior plan review: one reviewer reported no findings; two reviewers found
  that the plan did not explicitly state the workflow's no-push constraint. The
  worktree isolation and acceptance sections were updated before execution.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty" docs/plans/518-whole-codebase-architecture-review.md`:
  passed for the new untracked plan file. Git returned status `1` only because
  the files differ, with no whitespace findings.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a
  pseudo-terminal. Both bounded runs exited by timeout with status `124` as
  expected.
- TUI color capture: passed. The 80-column capture contained 436 SGR sequences
  across 9 unique 256-color foreground/background codes, and the 140-column
  capture contained 578 SGR sequences across 10 unique 256-color
  foreground/background codes.
- Cleanup: temporary home, binary, config, terminal captures, and daemon
  process were removed.
- Senior implementation review: three reviewers reported no findings.

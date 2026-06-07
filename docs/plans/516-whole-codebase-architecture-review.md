# 516 Whole Codebase Architecture Review

## Context

Plans 513 through 515 closed all selected follow-up findings from plan 512:

- Codex Responses normal-log shape privacy;
- Responses input item allowlist before provider dispatch;
- IO logger normal-log error privacy.

The user-requested architecture-alignment workflow requires regular
whole-codebase senior-engineer review against
`docs/ilonasin-architecture.md`, not only local review of individual slices.
Run a fresh checkpoint before selecting another runtime change.

## Goal

Obtain three independent senior-engineer reviews of the current codebase and
docs, focused on fidelity to `docs/ilonasin-architecture.md`, and record
concrete findings for future implementation slices.

## Scope

1. Ask three senior subagents to review the entire current codebase and docs.
2. Reviewers should inspect at least:
   - `docs/ilonasin-architecture.md`;
   - supporting markdown under `docs/**`;
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

## Worktree Isolation

This slice may commit only:

- `docs/plans/516-whole-codebase-architecture-review.md`.

If verification observes failures caused by unrelated dirty runtime files, note
that in the review record instead of folding those changes into this slice.

## Verification

Run:

```sh
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/516-whole-codebase-architecture-review.md
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
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Review Record

### Reviewer 1

Severity: Medium

- Finding: Known Responses `input[]` item parsers validate required fields but
  do not reject extra top-level fields. Codex routes can preserve and forward
  the original raw item for known non-message and non-function-call families.
- References:
  - `internal/openai/responses.go:357`
  - `internal/openai/responses.go:374`
  - `internal/openai/responses.go:424`
  - `internal/openai/responses.go:439`
  - `internal/openai/responses.go:466`
  - `internal/openai/responses.go:494`
  - `internal/openai/responses.go:519`
  - `internal/openai/responses.go:830`
  - `internal/provider/codex_responses_request.go:150`
  - `docs/ilonasin-architecture.md:208`
  - `docs/ilonasin-architecture.md:214`
- Requirement or issue: unsupported or unknown compatibility fields must fail
  locally before provider dispatch. Extra item fields are not an explicit
  namespaced provider escape hatch and should not be silently forwarded.
- Recommended next slice: Responses input item field allowlists. Add per-family
  top-level allowlists for known Responses input item families, preserving
  current accepted fields and rejecting unknown extras with
  `input[n].<field> is unsupported`.

### Reviewer 2

Severity: Medium

- Finding: Responses input item parsers validate required known fields but do
  not reject extra fields, and Codex preservation forwards the original raw
  item for known non-message and non-function-call families.
- References:
  - `internal/openai/responses.go:374`
  - `internal/openai/responses.go:424`
  - `internal/openai/responses.go:439`
  - `internal/openai/responses.go:466`
  - `internal/openai/responses.go:494`
  - `internal/openai/responses.go:519`
  - `internal/openai/responses.go:830`
  - `docs/ilonasin-architecture.md:208`
  - `docs/ilonasin-architecture.md:214`
- Requirement or issue: unsupported fields must fail locally before provider
  dispatch. The current parser permits extra fields on known families, which can
  become upstream payload on Codex paths.
- Recommended next slice: add per-family Responses input item field allowlists
  for Codex-preserved transcript, tool-search, and custom-tool families while
  preserving existing accepted fields.

### Reviewer 3

No findings.

## Selected Follow-Up Findings

1. Responses input item field allowlists. Reject unknown top-level fields on
   known Responses input item families before Codex raw preservation or
   non-Codex conversion can dispatch the request.

## Verification Record

- Senior plan review: two reviewers reported no findings; one reviewer found a
  low-risk wording issue where the plan referred to an active goal rather than
  the user-requested architecture-alignment workflow. The wording was corrected
  before execution.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty" docs/plans/516-whole-codebase-architecture-review.md`:
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
- TUI color capture: passed. The 80-column capture contained 108 256-color SGR
  foreground sequences, and the 140-column capture contained 175.
- Cleanup: temporary home, binary, config, terminal captures, and daemon
  process were removed.
- Worktree isolation: no runtime files were changed for this slice.
- Senior implementation review: three reviewers reported no findings.

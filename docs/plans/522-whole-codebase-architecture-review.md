# 522 Whole-Codebase Architecture Review

## Context

Recent implementation slices addressed:

- Codex Responses tool declaration allowlisting;
- structured normal-log redaction alignment;
- management TUI color saturation.

The active project goal requires regular senior whole-codebase reviews against
`docs/ilonasin-architecture.md` and all markdown files in `docs/**`, then
progressive implementation slices until the codebase is fully aligned.

## Goal

Perform a fresh whole-codebase architecture review and record concrete findings
that remain after plans 519 through 521.

## Scope

1. Review all current documentation in `docs/**`, with
   `docs/ilonasin-architecture.md` as the target architecture.
2. Inspect the current implementation across `cmd/` and `internal/`.
3. Ask three senior engineer subagents to independently review the whole
   codebase for:
   - architecture mismatches;
   - provider-boundary drift;
   - logging or privacy leaks;
   - TUI or management API boundary violations;
   - dead, duplicate, stale, or over-coupled code;
   - missing smoke or verification risks.
4. Record the review process, findings, and selected next implementation slice
   in this plan.
5. Do not edit production code in this review slice.
6. Record each finding with severity, file/line reference, concrete
   architecture or maintainability issue, and recommended next slice boundary.

## Worktree Isolation

- This slice may change only
  `docs/plans/522-whole-codebase-architecture-review.md`.
- Do not stage or commit runtime behavior changes in this slice.
- Do not push.
- If unrelated dirty files appear, record the interference and keep those files
  out of this slice.

## Out Of Scope

- Fixing findings in this slice.
- Changing runtime behavior.
- Adding permanent tests.
- Provider live-network probing.

## Verification

Run:

```sh
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/522-whole-codebase-architecture-review.md
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, keepalive disabled, and configured
   provider instances.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a pseudo-terminal.
5. Confirm TUI output includes ANSI color sequences.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- Three senior reviewers complete a fresh whole-codebase review.
- The plan records each reviewer result.
- The plan identifies the next concrete implementation slice unless all
  reviewers report no findings.
- No production code changes are made.
- Only `docs/plans/522-whole-codebase-architecture-review.md` changes or is
  committed.
- Senior implementation review passes for the doc-only checkpoint.
- Compile, vet, serve smoke, manage smoke, and plan-review checks pass.

## Review Record

- Senior plan review: all three reviewers approved after the plan added
  doc-only worktree isolation, no-push language, actionable finding format, and
  senior implementation review acceptance.
- Whole-codebase reviewer 1 finding:
  - Severity: Medium.
  - File: `internal/app/runtime_core.go:90`.
  - Issue: normal `app_bootstrap` logs include raw local filesystem values via
    `home_dir` and `config_file`, while the architecture and structured-log
    privacy policy treat raw paths as sensitive normal-log data.
  - Recommended next slice boundary: app-bootstrap log privacy only. Replace
    raw path attrs with non-path metadata or explicitly redacted path-class
    fields.
- Whole-codebase reviewer 2 result: no actionable findings.
- Whole-codebase reviewer 3 finding:
  - Severity: Medium.
  - File: `internal/openai/responses.go:377`.
  - Issue: `function_call.namespace` is accepted during Responses input parsing
    and preserved into Codex raw upstream input, while the architecture requires
    unsupported hosted, namespaced, MCP, shell, tool-search, and other
    unproven tool families to fail locally before provider dispatch. The
    non-Codex chat conversion path rejects `namespace`, but the Codex
    preservation path does not.
  - Recommended next slice boundary: Responses namespaced function-call
    rejection. Reject non-empty `input[n].namespace` before Codex preservation,
    while keeping existing non-Codex behavior.

## Selected Next Slice

Next implementation slice: Responses namespaced function-call rejection.

Rationale: this is a request validation and provider-boundary issue on the
compatibility surface. It can silently preserve an unsupported tool-family
signal into Codex provider dispatch, so it should be addressed before the
bootstrap log privacy cleanup.

The app-bootstrap log privacy finding remains queued for the following slice.

## Verification Record

- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty"
  docs/plans/522-whole-codebase-architecture-review.md`: passed for the new
  untracked plan file. Git returned status `1` only because the files differ,
  with no whitespace findings.
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
  capture contained 658 SGR sequences across 10 unique 256-color
  foreground/background codes.
- Senior implementation review: three reviewers reported no findings.
- Cleanup: temporary home, binary, config, terminal captures, and daemon
  process were removed.

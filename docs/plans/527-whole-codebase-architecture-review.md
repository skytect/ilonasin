# 527 Whole-Codebase Architecture Review

## Context

Plans 525 and 526 addressed the remaining provider-boundary finding from the
previous whole-codebase review:

- Codex non-stream response parsing now rejects namespaced `function_call`
  output before local Responses emission;
- the stale helper that partially relayed namespaced Codex function calls was
  removed;
- the management TUI palette was strengthened after the implementation slice.

The active project goal requires regular senior whole-codebase reviews against
`docs/ilonasin-architecture.md` and all markdown files in `docs/**`, then
progressive implementation slices until the codebase is fully aligned.

## Goal

Perform a fresh whole-codebase architecture review and record any concrete
findings that remain after plans 525 and 526.

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
   - missing smoke or verification risks;
   - color or TUI rendering regressions introduced by the latest palette
     adjustment.
4. Record the review process, findings, and selected next implementation slice
   in this plan.
5. Do not edit production code in this review slice.
6. Record each finding with severity, file/line reference, concrete
   architecture or maintainability issue, and recommended next slice boundary.

## Worktree Isolation

- This slice may change only
  `docs/plans/527-whole-codebase-architecture-review.md`.
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
git diff --no-index --check "$tmpempty" docs/plans/527-whole-codebase-architecture-review.md
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
5. Confirm TUI output includes ANSI color sequences and multiple 256-color
   codes.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- Three senior reviewers complete a fresh whole-codebase review.
- The plan records each reviewer result.
- The plan identifies the next concrete implementation slice unless all
  reviewers report no findings.
- No production code changes are made.
- Only `docs/plans/527-whole-codebase-architecture-review.md` changes or is
  committed.
- Senior implementation review passes for the doc-only checkpoint.
- Compile, vet, serve smoke, manage smoke, and plan-review checks pass.

## Review Record

- Senior plan review: Euclid, Avicenna, and Ampere approved the plan as-is.
- Euclid finding: no actionable findings. Euclid also noted that the base URL
  and local file permission checks line up with the active architecture.
- Avicenna finding: no actionable findings.
- Ampere finding: no actionable findings.

## Selected Next Slice

No concrete implementation slice was selected from this checkpoint because all
three senior reviewers reported no actionable findings.

The active long-term goal still requires continued fresh whole-codebase reviews
until three senior reviewers unanimously agree that the entire codebase has zero
flaws, no remaining tech debt, no dead or stale code, no duplicate code, and is
faithful to `docs/ilonasin-architecture.md`.

## Verification Record

- `date`: confirmed the local date as Sun Jun 7 11:57:39 +08 2026 before the
  checkpoint.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty"
  docs/plans/527-whole-codebase-architecture-review.md`: passed for the new
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
  pseudo-terminal. Both bounded runs exited by timeout with accepted status.
- TUI color capture: passed. The 80-column capture contained 436 SGR sequences
  across 12 unique 256-color foreground/background codes, and the 140-column
  capture contained 658 SGR sequences across 13 unique 256-color
  foreground/background codes.
- Senior implementation review: Euclid, Avicenna, and Ampere reported no
  findings.
- Cleanup: temporary home, binary, config, terminal captures, and daemon
  process were removed.

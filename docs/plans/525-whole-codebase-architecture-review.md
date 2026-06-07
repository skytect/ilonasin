# 525 Whole-Codebase Architecture Review

## Context

Plans 523 and 524 implemented both actionable findings from plan 522:

- non-empty Responses `function_call.namespace` now fails locally before Codex
  raw input preservation;
- normal `app_bootstrap` logs no longer emit raw home or config file paths.

The active project goal requires regular senior whole-codebase reviews against
`docs/ilonasin-architecture.md` and all markdown files in `docs/**`, then
progressive implementation slices until the codebase is fully aligned.

## Goal

Perform a fresh whole-codebase architecture review and record any concrete
findings that remain after plans 523 and 524.

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
  `docs/plans/525-whole-codebase-architecture-review.md`.
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
git diff --no-index --check "$tmpempty" docs/plans/525-whole-codebase-architecture-review.md
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
- Only `docs/plans/525-whole-codebase-architecture-review.md` changes or is
  committed.
- Senior implementation review passes for the doc-only checkpoint.
- Compile, vet, serve smoke, manage smoke, and plan-review checks pass.

## Review Record

- Senior plan review: Euclid, Avicenna, and Ampere approved the plan as-is.
- Euclid finding: Medium severity at
  `internal/provider/codex_responses_parse.go:614`. Codex non-stream parsing
  still preserves upstream namespaced `function_call` output items instead of
  failing locally. This conflicts with the architecture requirement to reject
  unsupported namespaced tool families, and it diverges from the Codex streaming
  parser.
- Ampere finding: Medium severity at
  `internal/provider/codex_responses_parse.go:614` and
  `internal/server/responses_sse.go:127`. Non-stream Codex can emit a
  namespaced `function_call` back to Responses clients, while follow-up input
  parsing now rejects non-empty `function_call.namespace`. This creates a
  partially relayed unsupported transcript family.
- Avicenna finding: no actionable findings.

## Selected Next Slice

Plan 526 should implement Codex non-stream namespaced function-call output
rejection.

The slice boundary is:

- update `internal/provider/codex_responses_parse.go` so non-stream Codex
  response parsing rejects any non-empty `function_call.namespace` with
  `upstream_invalid_response`;
- preserve supported non-namespaced function calls;
- leave streaming behavior, request conversion, server SSE shaping, TUI,
  storage, routing, logging, and config unchanged except as directly required
  by the rejection;
- use a temporary focused harness, compile/vet, and direct serve/manage smoke;
- do not add permanent tests.

## Verification Record

- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty"
  docs/plans/525-whole-codebase-architecture-review.md`: passed for the new
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
- TUI color capture: passed. Both captures were non-empty and contained ANSI
  SGR sequences with at least three unique 256-color codes.
- Cleanup: temporary home, binary, config, terminal captures, and daemon
  process were removed.

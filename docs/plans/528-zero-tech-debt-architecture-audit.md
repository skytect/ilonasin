# 528 Zero-Tech-Debt Architecture Audit

## Context

Plan 527 recorded a fresh whole-codebase review after the Codex namespace
cleanup and management TUI palette pass. All three senior reviewers reported no
actionable findings.

The active project goal requires a stricter completion gate than ordinary
checkpoint review: three senior engineers must freshly review the entire
codebase and unanimously agree there are no flaws, no remaining tech debt, no
dead or stale code, no duplicate code, no legacy-oriented residual
architecture, and that the implementation is faithful to
`docs/ilonasin-architecture.md`.

## Goal

Run a strict whole-codebase zero-tech-debt audit and record whether the project
meets the active completion gate or has any remaining concrete flaws to address.

## Scope

1. Review all current documentation in `docs/**`, with
   `docs/ilonasin-architecture.md` as the target architecture.
2. Inspect the current implementation across `cmd/` and `internal/`.
3. Ask three senior engineer subagents to independently perform a fresh,
   skeptical zero-tech-debt review of the whole codebase.
4. Require each reviewer to explicitly answer whether the codebase has:
   - zero architecture mismatches;
   - zero provider-boundary drift;
   - zero logging or privacy leaks;
   - zero TUI or management API boundary violations;
   - zero dead, duplicate, stale, legacy-oriented, or over-coupled code;
   - zero known verification or smoke-test gaps;
   - faithful implementation of `docs/ilonasin-architecture.md`;
   - optimally modular and maintainable design for the current target
     architecture.
5. Record any finding with severity, file/line reference, concrete issue, and
   recommended next slice boundary.
6. If all three reviewers explicitly certify the zero-tech-debt gate, record
   that evidence without changing production code.
7. Do not edit production code in this audit slice.

## Worktree Isolation

- This slice may change only
  `docs/plans/528-zero-tech-debt-architecture-audit.md`.
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
git diff --no-index --check "$tmpempty" docs/plans/528-zero-tech-debt-architecture-audit.md
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

- Three senior reviewers approve this audit plan.
- Three senior reviewers complete a fresh whole-codebase zero-tech-debt audit.
- Each reviewer explicitly answers the zero-tech-debt completion-gate question.
- The plan records each reviewer result.
- The plan identifies the next concrete implementation slice if any reviewer
  reports a flaw, debt, mismatch, or gap.
- No production code changes are made.
- Only `docs/plans/528-zero-tech-debt-architecture-audit.md` changes or is
  committed.
- Senior implementation review passes for the doc-only audit checkpoint.
- Compile, vet, serve smoke, manage smoke, and plan-review checks pass.

## Review Record

- Senior plan review: Euclid, Avicenna, and Ampere approved the plan as-is.
- Euclid result: cannot certify zero-tech-debt.
  - High severity at `docs/codex-compatibility-audit.md:42`: the supporting
    docs still warn not to switch normal Codex use to `ilonasin` except for
    covered Codex provider paths. OpenRouter Codex CLI behavior and broader
    tool-family parity remain blockers.
  - High severity at `docs/codex-compatibility-audit.md:154`: model discovery
    is still marked as needing a live rerun after the historical
    primary-credential discovery hazard.
  - Medium severity at `docs/codex-compatibility-audit.md:163`: hosted,
    deferred, namespace, freeform, MCP, shell, and tool-search families remain
    limited or unproven.
- Avicenna result: cannot certify zero-tech-debt.
  - Medium severity at `docs/ilonasin-architecture.md:639`: the target
    architecture still has deferred research, including provider adapter
    strategy, OpenRouter Codex behavior, and provider-term policy.
  - High severity at `docs/codex-compatibility-audit.md:23`: broad switching is
    blocked by OpenRouter compatibility and unproven tool-family parity, with
    switch-gate requirements still listed later in the same document.
  - Low severity at `internal/app/keepalive_provider_adapters.go:15`: app-local
    keepalive DTOs mirror provider DTO fields and convert them back at the
    boundary. This was intentional boundary work from plan 383, but remains a
    duplicate mapping surface under a strict zero-tech-debt gate.
- Ampere result: cannot certify zero-tech-debt.
  - High severity at `docs/codex-compatibility-audit.md:179`: full Codex tool
    parity is explicitly not proven for hosted, deferred, namespaced, MCP,
    tool-search, shell, and other custom tool families.
  - High severity at `docs/codex-compatibility-audit.md:187`: Codex CLI through
    OpenRouter remains partial, with the tested model failing at the
    provider-response layer.
  - High severity at `docs/codex-compatibility-audit.md:194`: model discovery
    still needs refreshed live switch-gate evidence for multi-credential Codex
    behavior.
  - Medium severity at `docs/codex-compatibility-audit.md:201`: health
    semantics remain upstream-centered and do not distinguish upstream health
    from local route compatibility for unsupported Codex tool families.

## Selected Next Slice

Plan 529 should refresh Codex switch-gate evidence.

The slice boundary is:

- run an isolated switch-gate smoke against the current code to refresh the
  stale Codex compatibility evidence in `docs/codex-compatibility-audit.md`;
- cover root and `/v1` model discovery, root and `/v1` text turns,
  multi-credential model discovery behavior, the historical primary-credential
  regression, per-attempt health, Codex CLI routing through DeepSeek and
  OpenRouter where credentials are available, upstream error paths, and privacy
  scans;
- update the compatibility audit with current evidence and any remaining
  blockers;
- do not implement broad tool-family parity in that slice unless the evidence
  refresh reveals a narrowly bounded local bug.

Follow-up slices should separately handle:

- Codex hosted, deferred, namespaced, MCP, shell, tool-search, and other
  tool-family parity or explicit local rejection documentation;
- local route-compatibility health classification distinct from upstream
  provider health;
- keepalive boundary simplification or explicit anti-corruption-layer hardening;
- deferred architecture research decisions that remain in
  `docs/ilonasin-architecture.md`.

## Verification Record

- `date`: confirmed the local date as Sun Jun 7 12:08:00 +08 2026 before the
  checkpoint.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty"
  docs/plans/528-zero-tech-debt-architecture-audit.md`: passed for the new
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

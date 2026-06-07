# 512 Whole Codebase Architecture Review

## Context

Plans 510 and 511 closed the previous whole-codebase review finding: OAuth
device-login logs no longer persist upstream response-body free text in normal
provider HTTP logs. The active goal requires regular fresh senior-engineer
reviews of the entire codebase against `docs/ilonasin-architecture.md`, not
only review of local implementation slices.

Run a new checkpoint before selecting another runtime change.

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

- `docs/plans/512-whole-codebase-architecture-review.md`.

If verification observes failures caused by unrelated dirty runtime files, note
that in the review record instead of folding those changes into this slice.

## Verification

Run:

```sh
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/512-whole-codebase-architecture-review.md
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

- Finding: Codex Responses upstream 400 logging includes raw client-supplied
  input shape strings such as item `type`, item `role`, and content type names
  in normal `provider_http` logs. `secretGuardHandler` truncates these keys but
  does not redact them.
- References:
  - `internal/provider/codex_responses_request.go:241`
  - `internal/provider/codex_responses.go:132`
  - `internal/provider/codex_responses_stream.go:102`
  - `internal/logging/logging.go:194`
  - `docs/ilonasin-architecture.md:401`
  - `docs/ilonasin-architecture.md:488`
- Requirement or issue: normal telemetry must remain metadata-only. Request
  bodies, raw payloads, and client-provided free-text shape details belong only
  in IO logging when IO logging is enabled.
- Recommended next slice: Codex Responses normal-log shape privacy. Replace
  free-text shape attributes with fixed allowlisted buckets, counts, or
  `other`, preserving IO-log-only access to raw shape details.

### Reviewer 2

Severity: Medium

- Finding: Unknown Responses input item types are accepted as
  `ResponseInputItem{Type: typ}` and, on Codex routes, the original raw item is
  preserved in `CodexResponsesInput` and forwarded upstream.
- References:
  - `internal/openai/responses.go:319`
  - `internal/openai/responses.go:320`
  - `internal/openai/responses.go:830`
  - `internal/provider/codex_responses_request.go:150`
  - `docs/ilonasin-architecture.md:208`
  - `docs/ilonasin-architecture.md:214`
- Requirement or issue: unsupported fields and unimplemented transcript or
  output families should fail locally unless they are an explicit implemented
  relay path. Unknown input item families must not be silently forwarded.
- Recommended next slice: Responses input allowlist for Codex preservation.
  Reject unknown `input[n].type` before provider dispatch while preserving the
  currently implemented validated item families.

### Reviewer 3

Severity: Medium

- Finding: IO logger failure reporting logs `err.Error()` into normal logs for
  IO-log encode, open, rotate, write, and rollback failures.
- References:
  - `internal/logging/io.go:305`
  - `internal/logging/io.go:312`
  - `docs/plans/077-structured-application-logging.md:126`
- Requirement or issue: structured normal logging should not persist raw
  `err.Error()` strings except for local static marker-free errors. IO logging
  failure paths can include filesystem or runtime text that should be reduced
  to safe diagnostics.
- Recommended next slice: IO logger normal-log privacy. Replace the raw
  `error` attribute with fixed stage plus normalized error class or safe reason
  tokens while preserving IO logging behavior.

## Selected Follow-Up Findings

1. Codex Responses normal-log shape privacy. Remove client-supplied free-text
   shape strings from normal `provider_http` logs and keep only counts or fixed
   buckets.
2. Responses input allowlist for Codex preservation. Reject unknown Responses
   input item types before they can be forwarded upstream.
3. IO logger normal-log privacy. Replace raw IO logger `err.Error()` normal-log
   attributes with normalized safe diagnostics.

## Verification Record

- Senior plan review: three reviewers reported no findings.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty" docs/plans/512-whole-codebase-architecture-review.md`:
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

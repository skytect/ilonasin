# 510 Whole Codebase Architecture Review

## Context

Plans 499 through 509 closed the latest selected architecture follow-ups across
Codex error logging privacy, Responses tool rejection, Responses
`parallel_tool_calls` rejection, Codex prompt-cache-key propagation, provider
quota policy, request throughput persistence, and TUI color polish.

The active goal requires regular whole-codebase senior-engineer review against
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

- `docs/plans/510-whole-codebase-architecture-review.md`.

If verification observes failures caused by unrelated dirty runtime files, note
that in the review record instead of folding those changes into this slice.

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
   capture disabled, keepalive disabled, and configured provider instances.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at narrow and wide terminal widths.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- Three senior reviewers complete whole-codebase architecture reviews.
- Concrete findings are recorded in this plan.
- Selected follow-up findings are listed for future slices.
- No runtime behavior changes are made, staged, or committed by this slice.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Verification Record

- Senior plan review: three reviewers reported no findings.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty" docs/plans/510-whole-codebase-architecture-review.md`:
  passed for the new untracked plan file before staging.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal.
- Cleanup: temporary home, binary, config, terminal captures, marker files, and
  daemon process were removed.
- Worktree isolation: no runtime files were changed for this slice.
- Senior implementation review: initial review found that plain
  `git diff --check` did not cover the untracked plan file; an explicit
  no-index check was run and recorded. The other two reviewers reported no
  findings.

## Review Record

### Reviewer 1

Severity: High

- Finding: OAuth device-login HTTP error logging can persist upstream OAuth
  device error `message`, `detail`, or `error_description` strings into normal
  `provider_http` logs through `upstream_error_summary`.
- References:
  - `internal/provider/oauth_device.go:425`
  - `internal/provider/oauth_device.go:438`
  - `internal/provider/oauth_device.go:555`
  - `internal/provider/oauth_device.go:562`
  - `docs/ilonasin-architecture.md:403`
  - `docs/ilonasin-architecture.md:419`
- Requirement or issue: normal observability must not persist raw provider
  payloads outside IO logging. The architecture only carves out OAuth refresh
  failure descriptions, not OAuth device-login upstream response summaries.
- Recommended next slice: OAuth device-login log privacy. Keep fixed structured
  status, class, byte count, endpoint label, provider metadata, and event ID;
  remove upstream free-text summaries from normal logs or gate them behind the
  IO log scrubber.

### Reviewer 2

Severity: Medium

- Finding: `oauthDeviceHTTPErrorAttrs` persists provider-supplied `error`,
  `message`, `detail`, and `error_description` fields into normal
  `provider_http` logs.
- References:
  - `internal/provider/oauth_device.go:438`
  - `internal/provider/oauth_device.go:539`
  - `internal/provider/oauth_device.go:550`
  - `internal/provider/oauth_device.go:556`
  - `docs/ilonasin-architecture.md:401`
  - `docs/plans/077-structured-application-logging.md:117`
- Requirement or issue: the metadata-only logging boundary and structured
  logging policy limit provider HTTP logs to safe metadata and classes, not raw
  upstream provider payload fields.
- Recommended next slice: OAuth device-login log privacy only. Replace raw
  upstream message or summary attrs with fixed normalized classes or tightly
  whitelisted code/type tokens, preserving status, byte count, event ID, and
  diagnostics correlation.

### Reviewer 3

No findings.

## Selected Follow-Up Findings

1. OAuth device-login log privacy boundary. Remove or constrain normal-log
   upstream OAuth device error details so provider free-text payload content is
   not persisted outside IO logging.
